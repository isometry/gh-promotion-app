package processor

import (
	"log/slog"

	"github.com/google/go-github/v68/github"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/event"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type checkSuiteEventProcessor struct {
	logger           *slog.Logger
	githubController *internalGitHub.Controller
}

// NewCheckSuiteEventProcessor initializes a Processor for handling check suite events with optional configurations.
func NewCheckSuiteEventProcessor(githubController *internalGitHub.Controller, opts ...Option) Processor {
	_inst := &checkSuiteEventProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *checkSuiteEventProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:check-suite")
}

func (p *checkSuiteEventProcessor) Process(req any) (bus *promotion.Bus, err error) {
	p.logger.Debug("processing check-suite event...")

	if p.githubController == nil {
		return nil, promotion.NewInternalErrorf("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	evt := parsedBus.Event

	if !event.IsEnabled(event.CheckSuite) {
		p.logger.Debug("check_suite event is not enabled. skipping...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	if p.githubController == nil {
		return nil, promotion.NewInternalErrorf("githubController is nil")
	}
	e, ok := evt.(*github.CheckSuiteEvent)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *github.CheckSuiteEvent got %T", evt)
	}

	if e.CheckSuite == nil || e.CheckSuite.Status == nil || e.CheckSuite.Conclusion == nil {
		p.logger.Info("ignoring check suite event without check suite...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	if *e.CheckSuite.Status != "completed" || *e.CheckSuite.Conclusion != "success" {
		p.logger.Info("ignoring incomplete check suite event and/or non-success check-suite status...",
			slog.String("conclusion", *e.CheckSuite.Conclusion),
			slog.String("status", *e.CheckSuite.Status))
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	bus.Context.HeadSHA = e.CheckSuite.HeadSHA

	for _, pr := range e.CheckSuite.PullRequests {
		if *pr.Head.SHA == *bus.Context.HeadSHA && bus.Context.Promoter.IsPromotionRequest(pr) {
			bus.Context.BaseRef = helpers.NormaliseRefPtr(*pr.Base.Ref)
			bus.Context.HeadRef = helpers.NormaliseRefPtr(*pr.Head.Ref)
			break
		}
	}

	if bus.Context.BaseRef == nil || bus.Context.HeadRef == nil {
		p.logger.Info("ignoring check suite event without matching promotion request...")
		return bus, nil
	}
	return bus, nil
}
