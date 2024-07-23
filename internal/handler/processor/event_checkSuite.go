package processor

import (
	"log/slog"
	"slices"

	"github.com/google/go-github/v62/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type checkSuiteEventProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

func (p *checkSuiteEventProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:check_suite")
}

func NewCheckSuiteEventProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
	_inst := &checkSuiteEventProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *checkSuiteEventProcessor) Process(req any) (bus *promotion.Bus, err error) {
	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	event := parsedBus.Event

	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	e, ok := event.(*github.CheckSuiteEvent)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *github.CheckSuiteEvent got %T", event)
	}

	p.logger.Debug("processing check suite event...")

	if *e.CheckSuite.Status != "completed" || slices.Contains([]string{"neutral", "skipped", "success"}, *e.CheckSuite.Conclusion) {
		p.logger.Info("ignoring incomplete check suite event with unprocessable check-suite status...", slog.String("status", *e.CheckSuite.Status))
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
