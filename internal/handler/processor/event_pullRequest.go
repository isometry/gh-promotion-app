package processor

import (
	"log/slog"

	"github.com/google/go-github/v68/github"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/event"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type pullRequestEventProcessor struct {
	logger           *slog.Logger
	githubController *internalGitHub.Controller
}

func (p *pullRequestEventProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:pull-request")
}

// NewPullRequestEventProcessor creates and returns a Processor to handle pull request events, initialized with given Controller controller and options.
func NewPullRequestEventProcessor(githubController *internalGitHub.Controller, opts ...Option) Processor {
	_inst := &pullRequestEventProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *pullRequestEventProcessor) Process(req any) (bus *promotion.Bus, err error) {
	p.logger.Debug("processing pull request event...")

	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	evt := parsedBus.Event

	if !event.IsEnabled(event.PullRequest) {
		p.logger.Debug("pull_request event is not enabled. skipping...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	e, ok := evt.(*github.PullRequestEvent)
	if !ok {
		return nil, promotion.NewInternalErrorf("invalid event type. expected *github.PullRequestEvent got %T", evt)
	}

	if *e.PullRequest.Draft {
		p.logger.Info("ignoring draft pull request...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	bus.Context.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
	bus.Context.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
	bus.Context.HeadSHA = e.PullRequest.Head.SHA
	bus.Context.PullRequest = e.PullRequest

	switch *e.Action {
	case "closed":
		if e.PullRequest.GetMerged() {
			p.logger.Debug("processing pull request closed and merged...")
			// send feedback commit status: success
			bus.EventStatus = promotion.Success
		}
		bus.EventStatus = promotion.Skipped
		return bus, nil
	case "opened":
		bus.EventStatus = promotion.Pending
		return bus, nil
	case "edited", "ready_for_review", "reopened", "unlocked":
		p.logger.Info("ignoring pull request event...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	default:
		p.logger.Info("ignoring pull request with unprocessable event...", slog.String("action", *e.Action))
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}
}
