package processor

import (
	"log/slog"

	"github.com/google/go-github/v62/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type statusProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

func NewStatusEventProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
	_inst := &statusProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *statusProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:status")
}

func (p *statusProcessor) Process(req any) (bus *promotion.Bus, err error) {
	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	event := parsedBus.Event

	e, ok := event.(*github.StatusEvent)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *github.DeploymentStatusEvent got %T", event)
	}

	p.logger.Debug("processing status event...")

	state := *e.State
	if state != "success" {
		p.logger.Info("ignoring non-success status event with unprocessable status event state...", slog.String("state", state))
		return bus, nil
	}

	bus.Context.HeadSHA = e.SHA

	return bus, nil
}
