package processor

import (
	"log/slog"

	"github.com/google/go-github/v68/github"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/event"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type deploymentStatusProcessor struct {
	logger           *slog.Logger
	githubController *internalGitHub.Controller
}

// NewDeploymentStatusEventProcessor initializes a Processor for handling deployment status events with optional configurations.
func NewDeploymentStatusEventProcessor(githubController *internalGitHub.Controller, opts ...Option) Processor {
	_inst := &deploymentStatusProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *deploymentStatusProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:deployment-status")
}

func (p *deploymentStatusProcessor) Process(req any) (bus *promotion.Bus, err error) {
	p.logger.Debug("processing deployment-status event...")

	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	evt := parsedBus.Event

	if !event.IsEnabled(event.DeploymentStatus) {
		p.logger.Debug("deployment_status event is not enabled. skipping...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	e, ok := evt.(*github.DeploymentStatusEvent)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *github.DeploymentStatusEvent got %T", evt)
	}

	state := *e.DeploymentStatus.State
	if state != "success" {
		p.logger.Debug("ignoring non-success deployment status event with unprocessable deployment status state...", slog.String("state", state))
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	bus.Context.HeadRef = helpers.NormaliseFullRefPtr(*e.Deployment.Ref)
	bus.Context.HeadSHA = e.Deployment.SHA

	return bus, nil
}
