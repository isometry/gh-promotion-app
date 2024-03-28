package handler

import (
	"context"
	"fmt"
	"github.com/google/go-github/v60/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/validation"
	"github.com/pkg/errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/isometry/gh-promotion-app/internal/promotion"
)

var HandledEventTypes = []string{
	"push", // triggers promotion request creation
	// all other events trigger promotion request fast-forward
	"pull_request",
	"pull_request_review",
	"check_suite",
	"deployment_status",
	"status",
	"workflow_run",
}

type Option func(*Handler)

type Handler struct {
	ctx                context.Context
	logger             *slog.Logger
	promoter           *promotion.Promoter
	githubController   *controllers.GitHub
	awsController      *controllers.AWS
	authMode           string
	ssmKey             string
	ghToken            string
	webhookSecret      *validation.WebhookSecret
	dynamicPromoterKey string
}

func NewPromotionHandler(options ...Option) (*Handler, error) {
	_inst := &Handler{
		logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	for _, opt := range options {
		opt(_inst)
	}

	if _inst.ctx == nil {
		_inst.ctx = context.Background()
	}

	if _inst.promoter == nil {
		// @TODO(Paulo): Extract from request dynamically in the future
		_inst.promoter = promotion.NewDefaultPromoter()
	}

	awsCtl, err := controllers.NewAWSController(
		controllers.WithContext(_inst.ctx))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS controller")
	}
	authenticator, err := controllers.NewGitHubController(
		controllers.WithLogger(_inst.logger.With("component", "github-controller")),
		controllers.WithSSMKey(_inst.ssmKey),
		controllers.WithAuthMode(_inst.authMode),
		controllers.WithToken(_inst.ghToken),
		controllers.WithAWSController(awsCtl),
		controllers.WithWebhookSecret(_inst.webhookSecret))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create GitHub githubController")
	}
	_inst.awsController = awsCtl
	_inst.githubController = authenticator

	return _inst, err
}

func (h *Handler) ValidateRequest(eventType string) (*helpers.Response, error) {
	if slices.Index(HandledEventTypes, eventType) == -1 {
		h.logger.Info("unhandled event")
		return &helpers.Response{StatusCode: http.StatusBadRequest}, fmt.Errorf("unhandled event type: %s", eventType)
	}
	return nil, nil
}

func (h *Handler) Process(body []byte, headers map[string]string) (response helpers.Response, err error) {
	logger := h.logger
	logger.Info("processing request...")

	eventType, found := headers[strings.ToLower(github.EventTypeHeader)]
	if !found {
		logger.Warn("missing event type")
		return helpers.Response{Body: "missing event type", StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("missing event type")
	}

	// Validate the request
	resp, err := h.ValidateRequest(eventType)
	if err != nil {
		return *resp, err
	}

	// Refresh credentials if needed
	h.logger.Debug("refreshing credentials...")
	h.logger.Debug("Controller", slog.Any("controller", h.githubController.Token))
	if err = h.githubController.Authenticate(body); err != nil {
		h.logger.Warn("failed to authenticate", slog.Any("error", err))
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, err
	}

	h.logger.Debug("reading request body...")
	if err = h.githubController.ValidateWebhookSecret(body, headers); err != nil {
		logger.Warn("validating signature", slog.Any("error", err))
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusForbidden}, err
	}
	logger.Debug("request is valid")

	event, err := github.ParseWebHook(eventType, body)
	if err != nil {
		logger.Warn("parsing webhook payload", slog.Any("error", err))
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("invalid payload")
	}

	if err = h.awsController.PutS3Object(eventType, os.Getenv("S3_BUCKET_NAME"), body); err != nil {
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, err
	}

	pCtx := promotion.Context{
		EventType: &eventType,
		Logger:    h.logger.With("routine", "promotion.Context"),
	}

	logger = logger.With(slog.Any("context", pCtx))
	switch e := event.(type) {
	case *github.PushEvent:
		logger.Debug("processing push event...")
		logger.Debug("assigning promoter...")
		pCtx.Promoter = h.promoter
		if pCtx.Promoter == nil {
			pCtx.Promoter = promotion.NewDynamicPromoter(logger, e.Repo.CustomProperties, h.dynamicPromoterKey)
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = e.Ref
		pCtx.HeadSHA = e.After

		if nextStage, isPromotable := pCtx.Promoter.IsPromotableRef(*e.Ref); isPromotable {
			pCtx.BaseRef = helpers.NormaliseFullRefPtr(nextStage)
		} else {
			msg := "ignoring push event on non-promotion branch"
			logger.Info(msg)
			return helpers.Response{Body: strings.ToLower(msg), StatusCode: http.StatusUnprocessableEntity}, nil
		}

		var pr *github.PullRequest
		if pr, _ = h.githubController.FindPullRequest(&pCtx); pr != nil {
			// PR already exists covering this push event
			logger.Info("skipping recreation of existing promotion request...", slog.String("url", *pr.URL))
		}

		if pr == nil {
			logger.Debug("no existing PR found. Creating...")
			pr, err = h.githubController.CreatePullRequest(&pCtx)
			if err != nil {
				logger.Error("failed to create promotion request", slog.Any("error", err))
				return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
			}
			logger.Info("created promotion request", slog.String("url", *pr.URL))
		}

		return helpers.Response{
			Body:       fmt.Sprintf("created promotion req: %s", pr.GetURL()),
			StatusCode: http.StatusCreated,
		}, nil

	case *github.PullRequestEvent:
		logger.Debug("processing pull request event...")
		logger.Debug("assigning promoter...")
		pCtx.Promoter = h.promoter
		if pCtx.Promoter == nil {
			pCtx.Promoter = promotion.NewDynamicPromoter(logger, e.Repo.CustomProperties, h.dynamicPromoterKey)
		}
		switch *e.Action {
		case "opened", "edited", "ready_for_review", "reopened", "unlocked":
			// pass
		default:
			logger.Info("ignoring pull request with unprocessable event...", slog.String("action", *e.Action))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
		pCtx.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

		logger.Info("parsed pull req event")

	case *github.PullRequestReviewEvent:
		logger.Debug("processing pull req review event...")
		logger.Debug("assigning promoter...")
		pCtx.Promoter = h.promoter
		if pCtx.Promoter == nil {
			pCtx.Promoter = promotion.NewDynamicPromoter(logger, e.Repo.CustomProperties, h.dynamicPromoterKey)
		}
		if *e.Review.State != "approved" {
			logger.Info("ignoring non-approved pull request review event with unprocessable review state...", slog.String("state", *e.Review.State))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
		pCtx.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

		logger.Info("parsed pull request review event")

	case *github.CheckSuiteEvent:
		logger.Debug("processing check suite event...")
		logger.Debug("assigning promoter...")
		pCtx.Promoter = h.promoter
		if pCtx.Promoter == nil {
			pCtx.Promoter = promotion.NewDynamicPromoter(logger, e.Repo.CustomProperties, h.dynamicPromoterKey)
		}
		if *e.CheckSuite.Status != "completed" || slices.Contains([]string{"neutral", "skipped", "success"}, *e.CheckSuite.Conclusion) {
			logger.Info("ignoring incomplete check suite event with unprocessable check-suite status...", slog.String("status", *e.CheckSuite.Status))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.CheckSuite.HeadSHA

		for _, pr := range e.CheckSuite.PullRequests {
			if *pr.Head.SHA == *pCtx.HeadSHA && pCtx.Promoter.IsPromotionRequest(pr) {
				pCtx.BaseRef = helpers.NormaliseRefPtr(*pr.Base.Ref)
				pCtx.HeadRef = helpers.NormaliseRefPtr(*pr.Head.Ref)
				break
			}
		}

		if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
			logger.Info("ignoring check suite event without matching promotion request...")
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		logger.Info("Parsed check suite event")

	case *github.DeploymentStatusEvent:
		logger.Info("processing deployment status event...")
		logger.Debug("assigning promoter...")
		pCtx.Promoter = h.promoter
		if pCtx.Promoter == nil {
			pCtx.Promoter = promotion.NewDynamicPromoter(logger, e.Repo.CustomProperties, h.dynamicPromoterKey)
		}
		state := *e.DeploymentStatus.State
		if state != "success" {
			logger.Info("ignoring non-success deployment status event with unprocessable deployment status state...", slog.String("state", state))
			return helpers.Response{StatusCode: http.StatusFailedDependency}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = helpers.NormaliseFullRefPtr(*e.Deployment.Ref)
		pCtx.HeadSHA = e.Deployment.SHA

		logger.Info("parsed deployment status event")

	case *github.StatusEvent:
		logger.Debug("processing status event...")
		logger.Debug("assigning promoter...")
		pCtx.Promoter = h.promoter
		if pCtx.Promoter == nil {
			pCtx.Promoter = promotion.NewDynamicPromoter(logger, e.Repo.CustomProperties, h.dynamicPromoterKey)
		}
		state := *e.State
		if state != "success" {
			logger.Info("ignoring non-success status event with unprocessable status event state...", slog.String("state", state))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.SHA

		logger.Info("parsed status event")

	case *github.WorkflowRunEvent:
		logger.Debug("processing workflow run event...")
		logger.Debug("assigning promoter...")
		pCtx.Promoter = h.promoter
		if pCtx.Promoter == nil {
			pCtx.Promoter = promotion.NewDynamicPromoter(logger, e.Repo.CustomProperties, h.dynamicPromoterKey)
		}
		status := *e.WorkflowRun.Status
		if status != "completed" {
			logger.Info("ignoring incomplete workflow run event with unprocessable workflow run status...", slog.String("status", status))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		conclusion := *e.WorkflowRun.Conclusion
		if conclusion != "success" {
			logger.Info("ignoring unsuccessful workflow run event with unprocessable workflow run conclusion...", slog.String("conclusion", conclusion))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.WorkflowRun.HeadSHA

		for _, pr := range e.WorkflowRun.PullRequests {
			if *pr.Head.SHA == *pCtx.HeadSHA && pCtx.Promoter.IsPromotionRequest(pr) {
				pCtx.BaseRef = helpers.NormaliseRefPtr(*pr.Base.Ref)
				pCtx.HeadRef = helpers.NormaliseRefPtr(*pr.Head.Ref)
				break
			}
		}

		if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
			logger.Info("ignoring check suite event without matching promotion request...")
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		logger.Info("parsed workflow run event")

	default:
		logger.Warn("rejecting unprocessable event type...", slog.String("eventType", eventType))
		return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
	}

	// ignore events without an open promotion req
	if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
		// find matching promotion req by head SHA and populate missing refs
		if _, err = h.githubController.FindPullRequest(&pCtx); err != nil {
			logger.Error("failed to find promotion request", slog.Any("error", err), slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA))
			return helpers.Response{StatusCode: http.StatusInternalServerError}, nil
		}
	}

	if err = h.githubController.FastForwardRefToSha(&pCtx); err != nil {
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
	}
	logger.Info("fast forward complete")
	return helpers.Response{Body: "Promotion complete", StatusCode: http.StatusNoContent}, nil
}
