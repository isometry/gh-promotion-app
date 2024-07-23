package processor

import (
	"log/slog"

	"github.com/google/go-github/v62/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type deploymentStatusProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

func NewDeploymentStatusEventProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
	_inst := &deploymentStatusProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *deploymentStatusProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:pull_request_review")
}

func (p *deploymentStatusProcessor) Process(req any) (bus *promotion.Bus, err error) {
	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	event := parsedBus.Event

	e, ok := event.(*github.DeploymentStatusEvent)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *github.DeploymentStatusEvent got %T", event)
	}

	p.logger.Info("processing deployment status event...")

	state := *e.DeploymentStatus.State
	if state != "success" {
		p.logger.Info("ignoring non-success deployment status event with unprocessable deployment status state...", slog.String("state", state))
	}

	bus.Context.HeadRef = helpers.NormaliseFullRefPtr(*e.Deployment.Ref)
	bus.Context.HeadSHA = e.Deployment.SHA

	return bus, nil
}
