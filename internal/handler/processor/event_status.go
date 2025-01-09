package processor

import (
	"log/slog"

	"github.com/google/go-github/v68/github"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/event"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type statusProcessor struct {
	logger           *slog.Logger
	githubController *internalGitHub.Controller
}

// NewStatusEventProcessor constructs a Processor instance for handling Controller status events with optional configurations.
func NewStatusEventProcessor(githubController *internalGitHub.Controller, opts ...Option) Processor {
	_inst := &statusProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *statusProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:status")
}

func (p *statusProcessor) Process(req any) (bus *promotion.Bus, err error) {
	p.logger.Debug("processing status event...")

	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	evt := parsedBus.Event

	if !event.IsEnabled(event.Status) {
		p.logger.Debug("status event is not enabled. skipping...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	e, ok := evt.(*github.StatusEvent)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *github.DeploymentStatusEvent got %T", evt)
	}

	p.logger.Debug("processing status event...")
	bus.Context.HeadSHA = e.SHA

	state := *e.State
	if state != "success" {
		p.logger.Info("ignoring non-success status event with unprocessable status event state...", slog.String("state", state))
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	return bus, nil
}
