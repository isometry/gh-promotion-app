package processor

import (
	"log/slog"

	"github.com/google/go-github/v62/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type pullRequestEventProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

func (p *pullRequestEventProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:pull_request")
}

func NewPullRequestEventProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
	_inst := &pullRequestEventProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *pullRequestEventProcessor) Process(req any) (bus *promotion.Bus, err error) {
	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	event := parsedBus.Event

	e, ok := event.(*github.PullRequestEvent)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *github.PullRequestEvent got %T", event)
	}

	p.logger.Debug("processing pull request event...")
	bus.Context.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
	bus.Context.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
	bus.Context.HeadSHA = e.PullRequest.Head.SHA
	bus.Context.PullRequest = e.PullRequest

	switch *e.Action {
	case "closed":
		if e.PullRequest.GetMerged() {
			p.logger.Debug("processing pull request closed and merged...")
			// send feedback commit status: success
			if statusErr := p.githubController.SendPromotionFeedbackCommitStatus(bus, nil, controllers.CommitStatusSuccess); statusErr != nil {
				p.logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
			}
		}
		return bus, nil
	case "opened":
		// send feedback commit status: pending
		if statusErr := p.githubController.SendPromotionFeedbackCommitStatus(bus, nil, controllers.CommitStatusPending); statusErr != nil {
			p.logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
		}
		fallthrough
	case "edited", "ready_for_review", "reopened", "unlocked":
		// pass
		p.logger.Info("ignoring pull request event...")
		return bus, nil
	default:
		p.logger.Info("ignoring pull request with unprocessable event...", slog.String("action", *e.Action))
		return bus, nil
	}
}
