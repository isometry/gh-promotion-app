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
	ctx               context.Context
	logger            *slog.Logger
	githubController  *controllers.GitHub
	awsController     *controllers.AWS
	authMode          string
	ssmKey            string
	ghToken           string
	lambdaPayloadType string
	webhookSecret     *validation.WebhookSecret

	// Extensions >
	createTargetRef     bool
	dynamicPromotion    bool
	dynamicPromotionKey string

	feedbackCommitStatus        bool
	feedbackCommitStatusContext string

	fetchRateLimits bool
	// />
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

func (h *Handler) CheckEventType(eventType string) (*helpers.Response, error) {
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

// Process handles the incoming request
//
//	@TODO(paulo) - refactor this by separating the event handling in dedicated interface/type/impls
func (h *Handler) Process(body []byte, headers map[string]string) (result *promotion.Result, err error) {
	// Default response
	result = &promotion.Result{
		Response: helpers.Response{StatusCode: http.StatusUnprocessableEntity},
	}

	logger := h.logger
	logger.Info("processing request...")

	eventType, found := headers[strings.ToLower(github.EventTypeHeader)]
	if !found {
		logger.Warn("missing event type")
		return &promotion.Result{
			Response: helpers.Response{Body: "missing event type", StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("missing event type")
	}

	deliveryId, found := headers[strings.ToLower(github.DeliveryIDHeader)]
	if !found {
		logger.Warn("missing delivery ID")
		return &promotion.Result{
			Response: helpers.Response{Body: "missing delivery ID", StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("missing delivery ID")
	}

	// Validate the request
	resp, err := h.CheckEventType(eventType)
	if err != nil {
		logger.Warn("validating request", slog.Any("error", err))
		return &promotion.Result{
			Response: *resp,
		}, promotion.NewInternalError("failed to validate request. error: %v", err)
	}

	// Add the event type to the logger now that we know it's valid
	logger = logger.With(slog.String("event", eventType))

	// Refresh credentials if needed
	if err = h.githubController.RetrieveCredentials(); err != nil {
		logger.Warn("failed to refresh credentials", slog.Any("error", err))
		return &promotion.Result{
			Response: helpers.Response{Body: err.Error(), StatusCode: http.StatusUnauthorized},
		}, promotion.NewInternalError("failed to refresh credentials. error: %v", err)
	}
	if err = h.githubController.ValidateWebhookSecret(body, headers); err != nil {
		logger.Warn("validating signature", slog.Any("error", err))
		return &promotion.Result{
			Response: helpers.Response{Body: err.Error(), StatusCode: http.StatusForbidden},
		}, promotion.NewInternalError("failed to validate signature. error: %v", err)
	}
	logger.Debug("request body is valid")

	// Add the delivery ID to the logger, now that we know the payload is valid
	logger = logger.With(slog.String("deliveryId", deliveryId))

	repo, err := h.ExtractCommonRepository(body)
	if err != nil {
		logger.Warn("failed to extract repository context", slog.Any("error", err))
		return &promotion.Result{
			Response: helpers.Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("failed to extract repository context. error: %v", err)
	}

	logger = logger.With(slog.Any("repo", repo.FullName))

	// GetGitHubClients the request
	logger.Debug("authenticating...")
	var clients *controllers.Client
	if clients, err = h.githubController.GetGitHubClients(body); err != nil {
		h.logger.Warn("failed to authenticate", slog.Any("error", err))
		return &promotion.Result{
			Response: helpers.Response{Body: err.Error(), StatusCode: http.StatusUnauthorized},
		}, promotion.NewInternalError("failed to authenticate. error: %v", err)
	}

	event, err := github.ParseWebHook(eventType, body)
	if err != nil {
		logger.Warn("parsing webhook payload", slog.Any("error", err))
		return &promotion.Result{
			Response: helpers.Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("failed to parse webhook payload. error: %v", err)
	}

	if err = h.awsController.PutS3Object(eventType, os.Getenv("S3_BUCKET_NAME"), body); err != nil {
		logger.Warn("failed to store event in S3", slog.Any("error", err))
		return &promotion.Result{
			Response: helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError},
		}, nil
	}

	result.Context = &promotion.Context{
		EventType:  event,
		Owner:      repo.Owner.Login,
		Repository: repo.Name,
		Logger:     logger.WithGroup("runtime:promotion"),
		ClientV3:   clients.V3,
		ClientV4:   clients.V4,
	}

	// If dynamic promotion is enabled use custom properties to set the promoter, else use the default promoter
	if h.dynamicPromotion {
		logger.Debug("assigning promoter...")
		result.Context.Promoter = promotion.NewDynamicPromoter(logger, repo.CustomProperties, h.dynamicPromotionKey)
	} else {
		logger.Info("dynamic promotion is disabled... defaulting to standard promoter")
		result.Context.Promoter = promotion.NewDefaultPromoter()
	}

	logger = logger.With(slog.Any("context", result.Context))
	switch e := event.(type) {
	case *github.PushEvent:
		logger.Debug("processing push_event...")

		result.Context.HeadRef = e.Ref
		result.Context.HeadSHA = e.After

		if nextStage, isPromotable := result.Context.Promoter.IsPromotableRef(*e.Ref); isPromotable {
			result.Context.BaseRef = helpers.NormaliseFullRefPtr(nextStage)
		} else {
			logger.Info("ignoring push event on non-promotion branch", slog.String("headRef", *result.Context.HeadRef))
			return result, nil
		}

		// Create missing target ref if the feature is enabled and the target ref does not exist
		if h.createTargetRef && !h.githubController.PromotionTargetRefExists(result.Context) {
			if _, err = h.githubController.CreatePromotionTargetRef(result.Context); err != nil {
				logger.Error("failed to create target ref", slog.Any("error", err))
				result.Response = helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
				return result, nil
			}
		}

		if result.Context.PullRequest, _ = h.githubController.FindPullRequest(result.Context); result.Context.PullRequest != nil {
			// PR already exists covering this push event
			logger.Info("skipping recreation of existing promotion request...", slog.String("url", *result.Context.PullRequest.URL))
			// send feedback commit status: pending
			if statusErr := h.SendFeedbackCommitStatus(result, nil, controllers.CommitStatusPending); statusErr != nil {
				logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
			}
			break
		}

		logger.Debug("no existing PR found. Creating...")
		result.Context.PullRequest, err = h.githubController.CreatePullRequest(result.Context)
		if err != nil {
			logger.Error("failed to create promotion request", slog.Any("error", err))
			result.Response = helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
			return result, nil
		}
		logger.Info("created promotion request", slog.String("url", *result.Context.PullRequest.URL))
		// send feedback commit status: pending
		if statusErr := h.SendFeedbackCommitStatus(result, nil, controllers.CommitStatusPending); statusErr != nil {
			logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
		}

	case *github.PullRequestEvent:
		logger.Debug("processing pull request event...")
		result.Context.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
		result.Context.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
		result.Context.HeadSHA = e.PullRequest.Head.SHA
		result.Context.PullRequest = e.PullRequest

		switch *e.Action {
		case "closed":
			if e.PullRequest.GetMerged() {
				logger.Debug("processing pull request closed and merged...")
				// send feedback commit status: success
				if statusErr := h.SendFeedbackCommitStatus(result, nil, controllers.CommitStatusSuccess); statusErr != nil {
					logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
				}
			}
			return result, nil
		case "opened":
			// send feedback commit status: pending
			if statusErr := h.SendFeedbackCommitStatus(result, nil, controllers.CommitStatusPending); statusErr != nil {
				logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
			}
			fallthrough
		case "edited", "ready_for_review", "reopened", "unlocked":
			// pass
			logger.Info("ignoring pull request event...")
			return result, nil
		default:
			logger.Info("ignoring pull request with unprocessable event...", slog.String("action", *e.Action))
			return result, nil
		}

	case *github.PullRequestReviewEvent:
		logger.Debug("processing pull request review event...")

		if *e.Review.State != "approved" {
			logger.Info("ignoring non-approved pull request review event with unprocessable review state...", slog.String("state", *e.Review.State))
			return result, nil
		}

		result.Context.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
		result.Context.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
		result.Context.HeadSHA = e.PullRequest.Head.SHA

	case *github.CheckSuiteEvent:
		logger.Debug("processing check suite event...")

		if *e.CheckSuite.Status != "completed" || slices.Contains([]string{"neutral", "skipped", "success"}, *e.CheckSuite.Conclusion) {
			logger.Info("ignoring incomplete check suite event with unprocessable check-suite status...", slog.String("status", *e.CheckSuite.Status))
			return result, nil
		}

		result.Context.HeadSHA = e.CheckSuite.HeadSHA

		for _, pr := range e.CheckSuite.PullRequests {
			if *pr.Head.SHA == *result.Context.HeadSHA && result.Context.Promoter.IsPromotionRequest(pr) {
				result.Context.BaseRef = helpers.NormaliseRefPtr(*pr.Base.Ref)
				result.Context.HeadRef = helpers.NormaliseRefPtr(*pr.Head.Ref)
				break
			}
		}

		if result.Context.BaseRef == nil || result.Context.HeadRef == nil {
			logger.Info("ignoring check suite event without matching promotion request...")
			return result, nil
		}

	case *github.DeploymentStatusEvent:
		logger.Info("processing deployment status event...")

		state := *e.DeploymentStatus.State
		if state != "success" {
			logger.Info("ignoring non-success deployment status event with unprocessable deployment status state...", slog.String("state", state))
		}

		result.Context.HeadRef = helpers.NormaliseFullRefPtr(*e.Deployment.Ref)
		result.Context.HeadSHA = e.Deployment.SHA

	case *github.StatusEvent:
		logger.Debug("processing status event...")

		state := *e.State
		if state != "success" {
			logger.Info("ignoring non-success status event with unprocessable status event state...", slog.String("state", state))
			return result, nil
		}

		result.Context.HeadSHA = e.SHA

	case *github.WorkflowRunEvent:
		logger.Debug("processing workflow run event...")

		status := *e.WorkflowRun.Status
		if status != "completed" {
			logger.Info("ignoring incomplete workflow run event with unprocessable workflow run status...", slog.String("status", status))
			return result, nil
		}

		conclusion := *e.WorkflowRun.Conclusion
		if conclusion != "success" {
			logger.Info("ignoring unsuccessful workflow run event with unprocessable workflow run conclusion...", slog.String("conclusion", conclusion))
			return result, nil
		}

		result.Context.HeadSHA = e.WorkflowRun.HeadSHA

		for _, pr := range e.WorkflowRun.PullRequests {
			if *pr.Head.SHA == *result.Context.HeadSHA && result.Context.Promoter.IsPromotionRequest(pr) {
				result.Context.BaseRef = helpers.NormaliseRefPtr(*pr.Base.Ref)
				result.Context.HeadRef = helpers.NormaliseRefPtr(*pr.Head.Ref)
				break
			}
		}

		if result.Context.BaseRef == nil || result.Context.HeadRef == nil {
			logger.Info("ignoring check suite event without matching promotion request...")
			return result, nil
		}

	default:
		logger.Warn("rejecting unprocessable event type...")
		return result, nil
	}

	// ignore events without an open promotion req
	if result.Context.BaseRef == nil || result.Context.HeadRef == nil {
		// find matching promotion request by head SHA and populate missing refs
		if result.Context.PullRequest, err = h.githubController.FindPullRequest(result.Context); err != nil {
			logger.Error("failed to find promotion request", slog.Any("error", err), slog.String("headRef", *result.Context.HeadRef), slog.String("headSHA", *result.Context.HeadSHA))
			return result, nil
		}
	}

	// ignore events with refs that are not promotable
	_, isPromotable := result.Context.Promoter.IsPromotableRef(*result.Context.HeadRef)
	if !isPromotable {
		logger.Info("ignoring event on a non-promotion branch",
			slog.String("headRef", *result.Context.HeadRef))
		return result, nil
	}

	if err = h.githubController.FastForwardRefToSha(result.Context); err != nil {
		logger.Error("failed to fast forward ref", slog.Any("error", err))
		result.Response = helpers.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
		return result, nil
	}

	logger.Info("fast forward complete")
	result.Response = helpers.Response{Body: "Promotion complete", StatusCode: http.StatusNoContent}

	return result, nil
}

func (h *Handler) SendFeedbackCommitStatus(promotionResult *promotion.Result, err error, status controllers.CommitStatus) error {
	if !h.feedbackCommitStatus {
		h.logger.Debug("feedback commit status sending disabled")
		return nil
	}

	return h.githubController.SendPromotionFeedbackCommitStatus(h.feedbackCommitStatusContext, promotionResult, err, status)
}

func (h *Handler) RateLimits(promotionResult *promotion.Result) (*github.RateLimits, error) {
	if !h.fetchRateLimits {
		h.logger.Debug("rate limits fetching disabled")
		return nil, nil
	}
	return h.githubController.RateLimits(promotionResult.Context.ClientV3)
}

func (h *Handler) GetLambdaPayloadType() string {
	return h.lambdaPayloadType
}
