package processor

import (
	"log/slog"
	"net/http"

	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type fastForwarderPostProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

func NewFastForwarderPostProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
	_inst := &authValidatorProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *fastForwarderPostProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("pre-processor:validator")
}

func (p *fastForwarderPostProcessor) Process(req any) (bus *promotion.Bus, err error) {
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus

	// ignore events without an open promotion req
	if bus.Context.BaseRef == nil || bus.Context.HeadRef == nil {
		// find matching promotion request by head SHA and populate missing refs
		if bus.Context.PullRequest, err = p.githubController.FindPullRequest(bus.Context); err != nil {
			p.logger.Error("failed to find promotion request", slog.Any("error", err), slog.String("headRef", *bus.Context.HeadRef), slog.String("headSHA", *bus.Context.HeadSHA))
			return bus, nil
		}
	}

	// ignore events with refs that are not promotable
	_, isPromotable := bus.Context.Promoter.IsPromotableRef(*bus.Context.HeadRef)
	if !isPromotable {
		p.logger.Info("ignoring event on a non-promotion branch",
			slog.String("headRef", *bus.Context.HeadRef))
		return bus, nil
	}

	if err = p.githubController.FastForwardRefToSha(bus.Context); err != nil {
		p.logger.Error("failed to fast forward ref", slog.Any("error", err))
		bus.Response = models.Response{Body: err.Error(), StatusCode: http.StatusInternalServerError}
		return bus, nil
	}

	p.logger.Info("fast forward complete")
	bus.Response = models.Response{Body: "Promotion complete", StatusCode: http.StatusNoContent}
	return bus, nil
}
