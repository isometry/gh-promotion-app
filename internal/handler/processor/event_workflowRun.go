package processor

import (
	"log/slog"

	"github.com/google/go-github/v68/github"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/event"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type workflowRunProcessor struct {
	logger           *slog.Logger
	githubController *internalGitHub.Controller
}

// NewWorkflowRunEventProcessor initializes a Processor for handling workflow run events with optional configurations.
func NewWorkflowRunEventProcessor(githubController *internalGitHub.Controller, opts ...Option) Processor {
	_inst := &workflowRunProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *workflowRunProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:workflow-run")
}

func (p *workflowRunProcessor) Process(req any) (bus *promotion.Bus, err error) {
	p.logger.Debug("processing workflow run event...")

	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	evt := parsedBus.Event

	if !event.IsEnabled(event.WorkflowRun) {
		p.logger.Debug("workflow_run event is not enabled. skipping...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	e, ok := evt.(*github.WorkflowRunEvent)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *github.DeploymentStatusEvent got %T", evt)
	}

	p.logger.Debug("processing workflow run event...")

	status := *e.WorkflowRun.Status
	if status != string(internalGitHub.CheckRunStatusCompleted) {
		p.logger.Info("ignoring incomplete workflow run event with unprocessable workflow run status...", slog.String("status", status))
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	conclusion := *e.WorkflowRun.Conclusion
	if conclusion != string(internalGitHub.CheckRunConclusionSuccess) {
		p.logger.Info("ignoring unsuccessful workflow run event with unprocessable workflow run conclusion...", slog.String("conclusion", conclusion))
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	bus.Context.HeadSHA = e.WorkflowRun.HeadSHA

	for _, pr := range e.WorkflowRun.PullRequests {
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
