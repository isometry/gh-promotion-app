package processor

import (
	"log/slog"

	"github.com/google/go-github/v62/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type pullRequestReviewEventProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

func (p *pullRequestReviewEventProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:pull_request_review")
}

func NewPullRequestReviewEventProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
	_inst := &pullRequestReviewEventProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *pullRequestReviewEventProcessor) Process(req any) (bus *promotion.Bus, err error) {
	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	event := parsedBus.Event

	e, ok := event.(*github.PullRequestReviewEvent)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *github.PullRequestReviewEvent got %T", event)
	}

	p.logger.Debug("processing pull request review event...")

	if *e.Review.State != "approved" {
		p.logger.Info("ignoring non-approved pull request review event with unprocessable review state...", slog.String("state", *e.Review.State))
		return bus, nil
	}

	bus.Context.BaseRef = helpers.NormaliseRefPtr(*e.PullRequest.Base.Ref)
	bus.Context.HeadRef = helpers.NormaliseRefPtr(*e.PullRequest.Head.Ref)
	bus.Context.HeadSHA = e.PullRequest.Head.SHA
	bus.Context.PullRequest = e.PullRequest
	return bus, nil
}
