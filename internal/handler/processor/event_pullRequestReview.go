package processor

import (
	"log/slog"

	"github.com/google/go-github/v68/github"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/event"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type pullRequestReviewEventProcessor struct {
	logger           *slog.Logger
	githubController *internalGitHub.Controller
}

// NewPullRequestReviewEventProcessor initializes a Processor for handling pull request review events with optional configurations.
func NewPullRequestReviewEventProcessor(githubController *internalGitHub.Controller, opts ...Option) Processor {
	_inst := &pullRequestReviewEventProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *pullRequestReviewEventProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:pull-request-review")
}

func (p *pullRequestReviewEventProcessor) Process(req any) (bus *promotion.Bus, err error) {
	p.logger.Debug("processing pull request review event...")

	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	evt := parsedBus.Event

	if !event.IsEnabled(event.PullRequestReview) {
		p.logger.Debug("pull_request_review event is not enabled. skipping...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	e, ok := evt.(*github.PullRequestReviewEvent)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *github.PullRequestReviewEvent got %T", evt)
	}

	p.logger.Debug("processing pull request review event...")

	if *e.Review.State != "approved" {
		p.logger.Info("ignoring non-approved pull request review event with unprocessable review state...", slog.String("state", *e.Review.State))
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	bus.Context.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
	bus.Context.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
	bus.Context.HeadSHA = e.PullRequest.Head.SHA
	bus.Context.PullRequest = e.PullRequest
	return bus, nil
}
