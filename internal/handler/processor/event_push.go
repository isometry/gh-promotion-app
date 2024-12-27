package processor

import (
	"log/slog"
	"net/http"

	"github.com/google/go-github/v67/github"
	"github.com/isometry/gh-promotion-app/internal/capabilities"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type pushEventProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

// NewPushEventProcessor creates a push event processor with an optional configuration and attaches a GitHub controller to it.
func NewPushEventProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
	_inst := &pushEventProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *pushEventProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("processor:push")
}

func (p *pushEventProcessor) Process(req any) (bus *promotion.Bus, err error) {
	if p.githubController == nil {
		return nil, promotion.NewInternalError("githubController is nil")
	}
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	event := parsedBus.Event

	e, ok := event.(*github.PushEvent)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *github.PushEvent got %T", event)
	}

	p.logger.Debug("processing push_event...")

	bus.Context.HeadRef = e.Ref
	bus.Context.HeadSHA = e.After

	if nextStage, isPromotable := bus.Context.Promoter.IsPromotableRef(*e.Ref); isPromotable {
		bus.Context.BaseRef = helpers.NormaliseFullRefPtr(nextStage)
	} else {
		p.logger.Info("ignoring push event on non-promotion branch", slog.String("headRef", *bus.Context.HeadRef))
		return bus, nil
	}

	// Create missing target ref if the feature is enabled and the target ref does not exist
	// 	@TODO(paulo) -> connect with cmd args
	if capabilities.Promotion.Push.CreateTargetRef && !p.githubController.PromotionTargetRefExists(bus.Context) {
		if _, err = p.githubController.CreatePromotionTargetRef(bus.Context); err != nil {
			p.logger.Error("failed to create target ref", slog.Any("error", err))
			bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
			return bus, nil
		}
	}

	if bus.Context.PullRequest, _ = p.githubController.FindPullRequest(bus.Context); bus.Context.PullRequest != nil {
		// PR already exists covering this push event
		p.logger.Info("skipping recreation of existing promotion request...", slog.String("url", *bus.Context.PullRequest.URL))
		// send feedback commit status: pending
		if statusErr := p.githubController.SendPromotionFeedbackCommitStatus(bus, nil, controllers.CommitStatusPending); statusErr != nil {
			p.logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
		}
		return bus, nil
	}

	p.logger.Debug("no existing PR found. Creating...")
	bus.Context.PullRequest, err = p.githubController.CreatePullRequest(bus.Context)
	if err != nil {
		p.logger.Error("failed to create promotion request", slog.Any("error", err))
		bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
		return bus, nil
	}
	p.logger.Info("created promotion request", slog.String("url", *bus.Context.PullRequest.URL))
	// send feedback commit status: pending
	if statusErr := p.githubController.SendPromotionFeedbackCommitStatus(bus, nil, controllers.CommitStatusPending); statusErr != nil {
		p.logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
	}
	return bus, nil
}
