package processor

import (
	"log/slog"
	"net/http"

	"github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type fastForwarderPostProcessor struct {
	logger           *slog.Logger
	githubController *github.Controller
}

// NewFastForwarderPostProcessor constructs a Processor instance for handling Controller status events with optional configurations.
func NewFastForwarderPostProcessor(githubController *github.Controller, opts ...Option) Processor {
	_inst := &fastForwarderPostProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *fastForwarderPostProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("post-processor:fast-forwarder")
}

func (p *fastForwarderPostProcessor) Process(req any) (bus *promotion.Bus, err error) {
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus

	p.logger.Debug("processing fast-forwarder...")

	if bus.Context.HeadSHA == nil {
		p.logger.Debug("ignoring event without a head SHA")
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	if bus.Context.BaseRef == nil || bus.Context.HeadRef == nil {
		// ignore events without an open promotion PR
		if bus.Context.PullRequest, err = p.githubController.FindPullRequest(bus.Context); err != nil {
			p.logger.Error("failed to find promotion PR", slog.Any("error", err))
			bus.EventStatus = promotion.Skipped
			return bus, err
		}
		p.logger.Debug("found promotion PR", slog.String("headRef", *bus.Context.HeadRef))
	}

	// @Note: deactivated to cope with API limits
	// if bus.Context.Commits == nil {
	//	if bus.Context.Commits, err = p.githubController.ListPullRequestCommits(bus.Context); err != nil {
	//		p.logger.Error("failed to find commits", slog.Any("error", err))
	//		bus.EventStatus = promotion.Skipped
	//		return bus, err
	//	}
	//}

	// ignore events with refs that are not promotable
	_, isPromotable := bus.Context.Promoter.IsPromotableRef(*bus.Context.HeadRef)
	if !isPromotable {
		p.logger.Debug("ignoring event on a non-promotion branch",
			slog.String("headRef", *bus.Context.HeadRef))
		bus.EventStatus = promotion.Skipped
		return bus, nil
	}

	if err = p.githubController.FastForwardRefToSha(bus.Context); err != nil {
		p.logger.Error("failed to fast-forward ref", slog.Any("error", err))
		bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
		bus.Error = err
		return bus, err
	}

	p.logger.Info("fast-forward complete")
	bus.Response = models.Response{Body: "Promotion complete", StatusCode: http.StatusNoContent}
	bus.EventStatus = promotion.Success
	return bus, nil
}
