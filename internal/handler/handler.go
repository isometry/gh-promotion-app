package handler

import (
	"context"
	"fmt"
	"github.com/google/go-github/v60/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
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
	// all other events trigger promotion request fast forward
	"pull_request",
	"pull_request_review",
	"check_suite",
	"deployment_status",
	"status",
	"workflow_run",
}

type Option func(*Handler)

func WithLogger(logger *slog.Logger) Option {
	return func(h *Handler) {
		h.logger = logger
	}
}

func WithContext(ctx context.Context) Option {
	return func(h *Handler) {
		h.ctx = ctx
	}
}

type Handler struct {
	ctx              context.Context
	logger           *slog.Logger
	promoter         *promotion.Promoter
	githubController *controllers.GitHub
	awsController    *controllers.AWS
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
		controllers.WithContext(_inst.ctx))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS controller")
	}
	authenticator, err := controllers.NewGitHubController(
		controllers.WithToken(os.Getenv("GITHUB_TOKEN")))
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

func (h *Handler) Process(req helpers.Request) (response helpers.Response, err error) {
	logger := h.logger
	logger.Info("handling req")

	eventType, found := req.Headers[strings.ToLower(github.EventTypeHeader)]
	if !found {
		logger.Warn("missing event type")
		return helpers.Response{Body: "missing event type", StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("missing event type")
	}

	// Validate the req
	resp, err := h.ValidateRequest(eventType)
	if err != nil {
		return *resp, err
	}

	h.logger.Debug("Reading req body...")
	if err = h.githubController.ValidateWebhookSecret(req.Body, req.Headers); err != nil {
		logger.Error("validating signature", slog.Any("error", err))
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusForbidden}, err
	}
	logger.Debug("req is valid")

	event, err := github.ParseWebHook(eventType, []byte(req.Body))
	if err != nil {
		logger.Warn("parsing webhook payload", slog.Any("error", err))
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity}, fmt.Errorf("invalid payload")
	}

	if err = h.awsController.PutS3Object(eventType, os.Getenv("S3_BUCKET_NAME"), []byte(req.Body)); err != nil {
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, err
	}

	pCtx := promotion.Context{
		EventType: &eventType,
		Logger:    logger.With("routine", "promotion.Context"),
		Promoter:  h.promoter,
	}

	logger = logger.With(slog.Any("context", pCtx))
	switch e := event.(type) {
	case *github.PushEvent:
		logger.Debug("processing push event...")

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = e.Ref // already fully qualified
		pCtx.HeadSHA = e.After

		if nextStage, isPromotable := h.promoter.IsPromotableRef(*e.Ref); isPromotable {
			pCtx.BaseRef = helpers.NormaliseRefPtr(nextStage)
		} else {
			msg := "ignoring push event on non-promotion branch"
			logger.Info(msg)
			return helpers.Response{Body: msg, StatusCode: http.StatusUnprocessableEntity}, nil
		}

		var pr *github.PullRequest
		if pr, _ = h.githubController.FindPullRequest(pCtx); pr != nil {
			// PR already exists covering this push event
			logger.Info("skipping recreation of existing promotion req")
		}

		if pr == nil {
			logger.Info("creating promotion req")
			pr, err = h.githubController.CreatePullRequest(pCtx)
			if err != nil {
				logger.Error("failed to create promotion req", slog.Any("error", err))
				return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
			}
			logger.Info("created promotion req", slog.String("url", *pr.URL))
		}

		return helpers.Response{
			Body:       fmt.Sprintf("Created promotion req: %s", pr.GetURL()),
			StatusCode: http.StatusCreated,
		}, nil

	case *github.PullRequestEvent:
		logger.Debug("processing pull req event...")
		switch *e.Action {
		case "opened", "edited", "ready_for_review", "reopened", "unlocked":
			// pass
		default:
			logger.Info("ignoring pull req event", slog.String("action", *e.Action))
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
		if *e.Review.State != "approved" {
			logger.Info("ignoring non-approved pull req review event", slog.String("state", *e.Review.State))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
		pCtx.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
		pCtx.HeadSHA = e.PullRequest.Head.SHA

		logger.Info("parsed pull req review event")

	case *github.CheckSuiteEvent:
		logger.Debug("processing check suite event...")
		if *e.CheckSuite.Status != "completed" || slices.Contains([]string{"neutral", "skipped", "success"}, *e.CheckSuite.Conclusion) {
			logger.Info("ignoring incomplete check suite event", slog.String("status", *e.CheckSuite.Status))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.CheckSuite.HeadSHA

		for _, pr := range e.CheckSuite.PullRequests {
			if *pr.Head.SHA == *pCtx.HeadSHA && h.promoter.IsPromotionRequest(pr) {
				pCtx.BaseRef = helpers.NormaliseRefPtr(*pr.Base.Ref)
				pCtx.HeadRef = helpers.NormaliseRefPtr(*pr.Head.Ref)
				break
			}
		}

		if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
			logger.Info("ignoring check suite event without matching promotion req")
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		logger.Info("parsed check suite event")

	case *github.DeploymentStatusEvent:
		logger.Info("processing deployment status event...")
		state := *e.DeploymentStatus.State
		if state != "success" {
			logger.Info("Ignoring non-success deployment status event", slog.String("state", state))
			return helpers.Response{StatusCode: http.StatusFailedDependency}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadRef = helpers.NormaliseRefPtr(*e.Deployment.Ref)
		pCtx.HeadSHA = e.Deployment.SHA

		logger.Info("parsed deployment status event")

	case *github.StatusEvent:
		logger.Debug("processing status event...")
		state := *e.State
		if state != "success" {
			logger.Info("Ignoring non-success status event", slog.String("state", state))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.SHA

		logger.Info("parsed status event")

	case *github.WorkflowRunEvent:
		logger.Debug("processing workflow run event...")
		status := *e.WorkflowRun.Status
		if status != "completed" {
			logger.Info("ignoring incomplete workflow run event", slog.String("status", status))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		conclusion := *e.WorkflowRun.Conclusion
		if conclusion != "success" {
			logger.Info("ignoring unsuccessful workflow run event", slog.String("conclusion", conclusion))
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		pCtx.Owner = e.Repo.Owner.Login
		pCtx.Repository = e.Repo.Name
		pCtx.HeadSHA = e.WorkflowRun.HeadSHA

		for _, pr := range e.WorkflowRun.PullRequests {
			if *pr.Head.SHA == *pCtx.HeadSHA && h.promoter.IsPromotionRequest(pr) {
				pCtx.BaseRef = helpers.NormaliseRefPtr(*pr.Base.Ref)
				pCtx.HeadRef = helpers.NormaliseRefPtr(*pr.Head.Ref)
				break
			}
		}

		if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
			logger.Info("Ignoring check suite event without matching promotion req")
			return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
		}

		logger.Info("parsed workflow run event")

	default:
		logger.Warn("unhandled event type", slog.String("eventType", eventType))
		return helpers.Response{StatusCode: http.StatusUnprocessableEntity}, nil
	}

	// ignore events without an open promotion req
	if pCtx.BaseRef == nil || pCtx.HeadRef == nil {
		// find matching promotion req by head SHA and populate missing refs
		if _, err = h.githubController.FindPullRequest(pCtx); err != nil {
			logger.Error("failed to find promotion req", slog.Any("error", err), slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA))
			return helpers.Response{StatusCode: http.StatusInternalServerError}, nil
		}
	}

	if err = h.githubController.FastForwardRefToSha(pCtx); err != nil {
		return helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}, nil
	}
	logger.Info("fast forward complete")
	return helpers.Response{Body: "Promotion complete", StatusCode: http.StatusNoContent}, nil
}

//func (gc *Handler) RetrieveClientCredsFromApp(ctx context.Context, request Request) error {
//	gc.logger.Info("retrieving GitHub Handler credentials")
//
//	if gc.AwsConfig != nil {
//		return gc.RetrieveGhAppCredsFromSSM(ctx)
//	}
//
//	var eventInstallationId EventInstallationId
//	err := json.Unmarshal([]byte(request.Body), &eventInstallationId)
//	if err != nil {
//		return fmt.Errorf("no installation ID found. error: %v", err)
//	}
//	gc.githubController.InstallationId = *eventInstallationId.Installation.ID
//	return fmt.Errorf("no GitHub Handler credentials available")
//}
//
//func (gc *Handler) RetrieveClientCredsFromEnv() error {
//	webhookSecret := validation.WebhookSecret(os.Getenv("GITHUB_WEBHOOK_SECRET"))
//	if webhookSecret == "" {
//		return fmt.Errorf("no GitHub Handler credentials available. [GITHUB_WEBHOOK_SECRET] is not set")
//	}
//	gc.githubController = &gh.Authenticator{
//		Token: os.Getenv("GITHUB_TOKEN"),
//	}
//	gc.githubController.WebhookSecret = &webhookSecret
//	return nil
//}

//func (gc *Handler) RetrieveGhAppCredsFromSSM(ctx context.Context) error {
//	gc.logger.Info("Retrieving GitHub Handler credentials from SSM")
//
//	ssmClient := ssm.NewFromConfig(*gc.AwsConfig)
//
//	ssmResponse, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
//		Name:           aws.String(os.Getenv("GITHUB_APP_SSM_ARN")),
//		WithDecryption: aws.Bool(true),
//	})
//	if err != nil {
//		gc.logger.Warn("failed to load SSM parameters", slog.Any("error", err))
//		return err
//	}
//
//	ghaParams := []byte(*ssmResponse.Parameter.Value)
//
//	if err = json.Unmarshal(ghaParams, &gc.githubController); err != nil {
//		gc.logger.Warn("failed to unmarshal SSM parameters", slog.Any("error", err))
//		return err
//	}
//
//	return nil
//}

//func (gc *Handler) RetrieveGhAppCredsFromVault(ctx context.Context) error {
//	panic("not implemented")
//}
