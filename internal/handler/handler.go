package handler

import (
	"context"
	"io"
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/handler/processor"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/isometry/gh-promotion-app/internal/validation"
	"github.com/pkg/errors"
)

type Option func(*Handler)

type Handler struct {
	githubController *controllers.GitHub
	awsController    *controllers.AWS

	preProcessors  []processor.Processor
	processors     map[string][]processor.Processor
	postProcessors []processor.Processor

	ctx               context.Context
	logger            *slog.Logger
	authMode          string
	ssmKey            string
	ghToken           string
	lambdaPayloadType string
	webhookSecret     *validation.WebhookSecret
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
	githubController, err := controllers.NewGitHubController(
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
	_inst.githubController = githubController

	// Defined processors
	_inst.preProcessors = []processor.Processor{
		processor.NewDynamicPromotionPreProcessor(_inst.githubController),
	}
	_inst.processors = map[string][]processor.Processor{
		"push":                {processor.NewPushEventProcessor(_inst.githubController)},
		"pull_request":        {processor.NewPullRequestEventProcessor(_inst.githubController)},
		"pull_request_review": {processor.NewPullRequestReviewEventProcessor(_inst.githubController)},
		"check_suite":         {processor.NewCheckSuiteEventProcessor(_inst.githubController)},
		"deployment_status":   {processor.NewDeploymentStatusEventProcessor(_inst.githubController)},
		"status":              {processor.NewStatusEventProcessor(_inst.githubController)},
		"workflow_run":        {processor.NewWorkflowRunEventProcessor(_inst.githubController)},
	}
	_inst.postProcessors = []processor.Processor{
		processor.NewFastForwarderPostProcessor(_inst.githubController),
		processor.NewS3UploaderPostProcessor(_inst.awsController),
		processor.NewRateLimitsPostProcessor(_inst.githubController),
	}

	return _inst, err
}

// Process handles the incoming request
func (h *Handler) Process(body []byte, headers map[string]string) (bus *promotion.Bus, err error) {
	logger := h.logger

	// Authenticate & Validate
	authValidatorProcessor := processor.NewAuthValidatorProcessor(h.githubController)
	bus, err = processor.Process(logger, &processor.AuthRequest{
		Body:    body,
		Headers: headers,
	}, authValidatorProcessor)
	if err != nil {
		logger.Error("failed to authenticate request", slog.Any("error", err))
		return
	}

	// Pre-processors
	bus, err = processor.Process(logger, bus, h.preProcessors...)
	if err != nil {
		logger.Error("failed to pre-process event", slog.Any("error", err))
		return
	}

	logger = logger.With(slog.Any("context", bus.Context))

	// Processors
	eventProcessors := h.processors[bus.EventType]
	if len(eventProcessors) == 0 {
		logger.Error("no processors found for event type")
		return bus, promotion.NewInternalError("no processors found for event type %s", bus.EventType)
	}
	bus, err = processor.Process(logger, bus, eventProcessors...)
	if err != nil {
		logger.Error("failed to post-process event", slog.Any("error", err))
		return
	}

	// Post-processors
	bus, err = processor.Process(logger, bus, h.postProcessors...)
	if err != nil {
		logger.Error("failed to process event", slog.Any("error", err))
		return
	}

	// Deferred
	defer func() {
		// send feedback commit status: error
		if err != nil {
			status := controllers.CommitStatusFailure
			var promotionErr *promotion.InternalError
			if errors.As(err, &promotionErr) {
				status = controllers.CommitStatusError
			}
			if statusErr := h.githubController.SendPromotionFeedbackCommitStatus(bus, err, status); statusErr != nil {
				logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
			}
		}
	}()

	return bus, nil
}

func (h *Handler) GetLambdaPayloadType() string {
	return h.lambdaPayloadType
}
