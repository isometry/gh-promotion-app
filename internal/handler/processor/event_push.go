package processor

import (
	"log/slog"
	"net/http"

	"github.com/google/go-github/v68/github"
	"github.com/isometry/gh-promotion-app/internal/config"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/event"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type pushEventProcessor struct {
	logger           *slog.Logger
	githubController *internalGitHub.Controller
}

// NewPushEventProcessor creates a push event processor with an optional configuration and attaches a Controller controller to it.
func NewPushEventProcessor(githubController *internalGitHub.Controller, opts ...Option) Processor {
	_inst := &pushEventProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *pushEventProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:push")
}

func (p *pushEventProcessor) Process(req any) (bus *promotion.Bus, err error) {
	p.logger.Debug("processing push event...")

	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	evt := parsedBus.Event

	if !event.IsEnabled(event.Push) {
		p.logger.Debug("push event is not enabled. skipping...")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	e, ok := evt.(*github.PushEvent)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *github.PushEvent got %T", evt)
	}

	bus.Context.HeadRef = e.Ref
	bus.Context.HeadSHA = e.After

	if nextStage, isPromotable := bus.Context.Promoter.IsPromotableRef(*e.Ref); isPromotable {
		bus.Context.BaseRef = helpers.NormaliseFullRefPtr(nextStage)
	} else {
		p.logger.Info("ignoring push event on non-promotion branch", slog.String("headRef", *bus.Context.HeadRef))
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	// Create missing target ref if the feature is enabled and the target ref does not exist
	if config.Promotion.Push.CreateTargetRef && !p.githubController.PromotionTargetRefExists(bus.Context) {
		if _, err = p.githubController.CreatePromotionTargetRef(bus.Context); err != nil {
			p.logger.Error("failed to create target ref", slog.Any("error", err))
			bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
			return bus, err
		}
	}

	if bus.Context.PullRequest, _ = p.githubController.FindPullRequest(bus.Context); bus.Context.PullRequest != nil {
		// PR already exists covering this push event
		p.logger.Info("skipping recreation of existing promotion request...", slog.String("url", *bus.Context.PullRequest.URL))
		// send feedback commit status: pending
		bus.EventStatus = promotion.Pending
		return bus, nil
	}

	p.logger.Debug("creating promotion PR...")
	bus.Context.PullRequest, err = p.githubController.CreatePullRequest(bus.Context)
	if err != nil {
		p.logger.Error("failed to create promotion PR", slog.Any("error", err))
		bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
		return bus, err
	}
	p.logger.Info("created promotion PR", slog.String("url", *bus.Context.PullRequest.URL))
	// send feedback commit status: pending
	bus.EventStatus = promotion.Pending
	return bus, nil
}
