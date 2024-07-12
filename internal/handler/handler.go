package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/google/go-github/v62/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/isometry/gh-promotion-app/internal/validation"
	"github.com/pkg/errors"
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
	ctx                 context.Context
	logger              *slog.Logger
	githubController    *controllers.GitHub
	awsController       *controllers.AWS
	authMode            string
	ssmKey              string
	ghToken             string
	webhookSecret       *validation.WebhookSecret
	dynamicPromotion    bool
	dynamicPromotionKey string
	lambdaPayloadType   string
	createTargetRef     bool
}

type CommonRepository struct {
	Name     *string `json:"name,omitempty"`
	FullName *string `json:"full_name,omitempty"`
	Owner    *struct {
		Login *string `json:"login,omitempty"`
	} `json:"owner,omitempty"`
	CustomProperties map[string]string `json:"custom_properties,omitempty"`
}

type EventRepository struct {
	Repository CommonRepository `json:"repository"`
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

	awsCtl, err := controllers.NewAWSController(
		controllers.WithAWSLogger(_inst.logger.With("component", "aws-controller")),
		controllers.WithAWSContext(_inst.ctx))

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
		return nil, errors.Wrap(err, "failed to create the GitHubController instance")
	}
	_inst.awsController = awsCtl
	_inst.githubController = authenticator

	return _inst, err
}

func (h *Handler) ValidateRequest(eventType string) (*helpers.Response, error) {
	if slices.Index(HandledEventTypes, eventType) == -1 {
		return &helpers.Response{StatusCode: http.StatusBadRequest}, fmt.Errorf("unhandled event type: %s", eventType)
	}
	return nil, nil
}

func (h *Handler) ExtractCommonRepository(body []byte) (*CommonRepository, error) {
	var eventRepository EventRepository
	if err := json.Unmarshal(body, &eventRepository); err != nil {
		return nil, fmt.Errorf("event repository not found. error: %v", err)
	}

	return &eventRepository.Repository, nil
}

func (h *Handler) Process(body []byte, headers map[string]string) (response helpers.Response, err error) {
	logger := h.logger
	logger.Info("processing request...")

	eventType, found := headers[strings.ToLower(github.EventTypeHeader)]
	if !found {
		logger.Warn("missing event type")
		return helpers.Response{Body: "missing event type", StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("missing event type")
	}

	deliveryId, found := headers[strings.ToLower(github.DeliveryIDHeader)]
	if !found {
		logger.Warn("missing delivery ID")
		return helpers.Response{Body: "missing delivery ID", StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("missing delivery ID")
	}

	// Validate the request
	resp, err := h.ValidateRequest(eventType)
	if err != nil {
		logger.Warn("validating request", slog.Any("error", err))
		return *resp, err
	}

	// Add the event type to the logger now that we know it's valid
	logger = logger.With(slog.String("event", eventType))

	// Refresh credentials if needed
	if err = h.githubController.RetrieveCredentials(); err != nil {
		logger.Warn("failed to refresh credentials", slog.Any("error", err))
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, err
	}
	if err = h.githubController.ValidateWebhookSecret(body, headers); err != nil {
		logger.Warn("validating signature", slog.Any("error", err))
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusForbidden}, err
	}
	logger.Debug("request body is valid")

	// Add the delivery ID to the logger, now that we know the payload is valid
	logger = logger.With(slog.String("deliveryId", deliveryId))

	repo, err := h.ExtractCommonRepository(body)
	if err != nil {
		logger.Warn("failed to extract repository context", slog.Any("error", err))
		return helpers.Response{Body: "missing repository field", StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("missing repository field")
	}

	logger = logger.With(slog.Any("repo", repo.FullName))

	// GetGitHubClients the request
	logger.Debug("authenticating...")
	var clients *controllers.Client
	if clients, err = h.githubController.GetGitHubClients(body); err != nil {
		h.logger.Warn("failed to authenticate", slog.Any("error", err))
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, err
	}

	event, err := github.ParseWebHook(eventType, body)
	if err != nil {
		logger.Warn("parsing webhook payload", slog.Any("error", err))
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("invalid payload")
	}

	if err = h.awsController.PutS3Object(eventType, os.Getenv("S3_BUCKET_NAME"), body); err != nil {
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, err
	}

	pCtx := &promotion.Context{
		EventType:  &eventType,
		Owner:      repo.Owner.Login,
		Repository: repo.Name,
		Logger:     logger.With(slog.String("routine", "promotion.Context")),
		ClientV3:   clients.V3,
		ClientV4:   clients.V4,
	}

	// If dynamic promotion is enabled use custom properties to set the promoter, else use the default promoter
	if h.dynamicPromotion {
		logger.Debug("assigning promoter...")
		pCtx.Promoter = promotion.NewDynamicPromoter(logger, repo.CustomProperties, h.dynamicPromotionKey)
	} else {
		logger.Info("dynamic promotion is disabled... defaulting to standard promoter")
		pCtx.Promoter = promotion.NewDefaultPromoter()
	}

	logger = logger.With(slog.Any("context", pCtx))
	switch e := event.(type) {
	case *github.PushEvent:
		logger.Debug("processing push_event...")

		pCtx.HeadRef = e.Ref
		pCtx.HeadSHA = e.After

		if nextStage, isPromotable := pCtx.Promoter.IsPromotableRef(*e.Ref); isPromotable {
			pCtx.BaseRef = helpers.NormaliseFullRefPtr(nextStage)
		} else {
			logger.Info("ignoring push event on non-promotion branch", slog.String("headRef", *pCtx.HeadRef))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		// Create missing target ref if the feature is enabled and the target ref does not exist
		if h.createTargetRef && !h.githubController.PromotionTargetRefExists(pCtx) {
			if _, err = h.githubController.CreatePromotionTargetRef(pCtx); err != nil {
				return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
			}
		}

		var pr *github.PullRequest
		if pr, _ = h.githubController.FindPullRequest(pCtx); pr != nil {
			// PR already exists covering this push event
			logger.Info("skipping recreation of existing promotion request...", slog.String("url", *pr.URL))
		}

		if pr == nil {
			logger.Debug("no existing PR found. Creating...")
			pr, err = h.githubController.CreatePullRequest(pCtx)
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

		switch *e.Action {
		case "opened", "edited", "ready_for_review", "reopened", "unlocked":
			// pass
		default:
			logger.Info("ignoring pull request with unprocessable event...", slog.String("action", *e.Action))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
		pCtx.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

		logger.Info("parsed pull request event")

	case *github.PullRequestReviewEvent:
		logger.Debug("processing pull request review event...")

		if *e.Review.State != "approved" {
			logger.Info("ignoring non-approved pull request review event with unprocessable review state...", slog.String("state", *e.Review.State))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
		pCtx.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

		logger.Info("parsed pull request review event")

	case *github.CheckSuiteEvent:
		logger.Debug("processing check suite event...")

		if *e.CheckSuite.Status != "completed" || slices.Contains([]string{"neutral", "skipped", "success"}, *e.CheckSuite.Conclusion) {
			logger.Info("ignoring incomplete check suite event with unprocessable check-suite status...", slog.String("status", *e.CheckSuite.Status))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

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

		state := *e.DeploymentStatus.State
		if state != "success" {
			logger.Info("ignoring non-success deployment status event with unprocessable deployment status state...", slog.String("state", state))
			return helpers.Response{StatusCode: http.StatusFailedDependency}, nil
		}

		pCtx.HeadRef = helpers.NormaliseFullRefPtr(*e.Deployment.Ref)
		pCtx.HeadSHA = e.Deployment.SHA

		logger.Info("parsed deployment status event")

	case *github.StatusEvent:
		logger.Debug("processing status event...")

		state := *e.State
		if state != "success" {
			logger.Info("ignoring non-success status event with unprocessable status event state...", slog.String("state", state))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.HeadSHA = e.SHA

		logger.Info("parsed status event")

	case *github.WorkflowRunEvent:
		logger.Debug("processing workflow run event...")

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
		logger.Warn("rejecting unprocessable event type...")
		return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
	}

	// ignore events without an open promotion req
	if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
		// find matching promotion request by head SHA and populate missing refs
		if _, err = h.githubController.FindPullRequest(pCtx); err != nil {
			logger.Error("failed to find promotion request", slog.Any("error", err), slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA))
			return helpers.Response{StatusCode: http.StatusInternalServerError}, nil
		}
	}

	// ignore events with refs that are not promotable
	_, isPromotable := pCtx.Promoter.IsPromotableRef(*pCtx.HeadRef)
	if !isPromotable {
		logger.Info("ignoring event on a non-promotion branch",
			slog.String("headRef", *pCtx.HeadRef))
		return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
	}

	if err = h.githubController.FastForwardRefToSha(pCtx); err != nil {
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
	}
	logger.Info("fast forward complete")
	return helpers.Response{Body: "Promotion complete", StatusCode: http.StatusNoContent}, nil
}

func (h *Handler) GetLambdaPayloadType() string {
	return h.lambdaPayloadType
}
