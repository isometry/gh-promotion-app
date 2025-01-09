// Package handler provides a generic interface for processing requests using a list of processors.
package handler

import (
	"context"
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/controllers/aws"
	"github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/event"
	"github.com/isometry/gh-promotion-app/internal/handler/processor"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/isometry/gh-promotion-app/internal/validation"
	"github.com/pkg/errors"
)

// Option is a functional option for the handler.
type Option func(*Handler)

// Handler is the main handler for the promotion app.
type Handler struct {
	githubController *github.Controller
	awsController    *aws.Controller

	preProcessors      []processor.Processor
	processors         map[event.Type][]processor.Processor
	postProcessors     []processor.Processor
	feedbackProcessors []processor.Processor

	ctx               context.Context
	logger            *slog.Logger
	authMode          string
	ssmKey            string
	ghToken           string
	lambdaPayloadType string
	webhookSecret     *validation.WebhookSecret
}

// NewPromotionHandler creates a new promotion handler instance.
func NewPromotionHandler(options ...Option) (*Handler, error) {
	_inst := &Handler{logger: helpers.NewNoopLogger()}
	for _, opt := range options {
		opt(_inst)
	}

	if _inst.ctx == nil {
		_inst.ctx = context.Background()
	}

	awsCtl, err := aws.NewController(
		aws.WithLogger(_inst.logger.With("component", "aws-controller")),
		aws.WithContext(_inst.ctx))

	if err != nil {
		return nil, errors.Wrap(err, "failed to create Controller controller")
	}
	githubController, err := github.NewController(
		github.WithLogger(_inst.logger.With("component", "github-controller")),
		github.WithSSMKey(_inst.ssmKey),
		github.WithAuthMode(_inst.authMode),
		github.WithToken(_inst.ghToken),
		github.WithAWSController(awsCtl),
		github.WithWebhookSecret(_inst.webhookSecret))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the GitHubController instance")
	}
	_inst.awsController = awsCtl
	_inst.githubController = githubController

	// Defined processors
	_inst.preProcessors = []processor.Processor{
		processor.NewDynamicPromotionPreProcessor(_inst.githubController),
	}
	_inst.processors = map[event.Type][]processor.Processor{
		event.Push:              {processor.NewPushEventProcessor(_inst.githubController)},
		event.PullRequest:       {processor.NewPullRequestEventProcessor(_inst.githubController)},
		event.PullRequestReview: {processor.NewPullRequestReviewEventProcessor(_inst.githubController)},
		event.CheckSuite:        {processor.NewCheckSuiteEventProcessor(_inst.githubController)},
		event.DeploymentStatus:  {processor.NewDeploymentStatusEventProcessor(_inst.githubController)},
		event.Status:            {processor.NewStatusEventProcessor(_inst.githubController)},
		event.WorkflowRun:       {processor.NewWorkflowRunEventProcessor(_inst.githubController)},
	}
	_inst.postProcessors = []processor.Processor{
		processor.NewFastForwarderPostProcessor(_inst.githubController),
		processor.NewS3UploaderPostProcessor(_inst.awsController),
	}
	_inst.feedbackProcessors = []processor.Processor{
		processor.NewCommitStatusFeedbackProcessor(_inst.githubController),
		processor.NewCheckRunFeedbackProcessor(_inst.githubController),
	}

	return _inst, err
}

// Process processes the incoming request.
func (h *Handler) Process(body []byte, headers map[string]string) (*promotion.Bus, error) {
	logger := h.logger

	// Authentication & Validation
	authValidatorProcessor := processor.NewAuthValidatorProcessor(h.githubController)
	bus, err := processor.Process(logger, &processor.AuthRequest{
		Body:            body,
		Headers:         headers,
		EventProcessors: h.processors,
	}, authValidatorProcessor)
	if err != nil {
		logger.Error("failed to authenticate request", slog.Any("error", err))
		return bus, err
	}

	logger = logger.With(slog.Any("context", bus.Context))
	logger.Info("processing event...")

	// Pre-processors
	// @NOTE The processors sections is intentionally not DRY to allow for flexibility in the future
	logger.Debug("launching pre-processors...")
	bus, err = processor.Process(logger, bus, h.preProcessors...)
	if err != nil {
		logger.Error("failed to pre-process event", slog.Any("error", err))
		return bus, err
	}
	if bus.EventStatus == promotion.Skipped {
		logger.Info("skipping event processing")
		return bus, nil
	}

	// Processors
	logger.Debug("launching processors...")
	eventProcessors := h.processors[bus.EventType]
	bus, err = processor.Process(logger, bus, eventProcessors...)
	if err != nil {
		logger.Error("failed to post-process event", slog.Any("error", err))
		return bus, err
	}
	if bus.EventStatus == promotion.Skipped {
		logger.Info("skipping event processing")
		return bus, nil
	}

	// Post-processors
	logger.Debug("launching post-processors...")
	bus, err = processor.Process(logger, bus, h.postProcessors...)
	if err != nil {
		logger.Error("failed to process event", slog.Any("error", err))
	}
	if bus.EventStatus == promotion.Skipped {
		logger.Info("skipping event processing")
		return bus, nil
	}

	// Feedback
	logger.Debug("launching feedback processors...")
	bus, err = processor.Process(logger, bus, h.feedbackProcessors...)
	if err != nil {
		logger.Error("failed to process event", slog.Any("error", err))
		return bus, err
	}

	return bus, nil
}

// GetLambdaPayloadType returns the lambda payload type.
func (h *Handler) GetLambdaPayloadType() string {
	return h.lambdaPayloadType
}
