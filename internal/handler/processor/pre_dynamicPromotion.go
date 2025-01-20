package processor

import (
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type dynamicPromotionProcessor struct {
	logger           *slog.Logger
	githubController *github.Controller
}

// NewDynamicPromotionPreProcessor initializes and returns a Processor for handling dynamic promotion, applying the given options.
func NewDynamicPromotionPreProcessor(githubController *github.Controller, opts ...Option) Processor {
	_inst := &dynamicPromotionProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *dynamicPromotionProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("pre-processor:dynamic-promotion")
}

func (p *dynamicPromotionProcessor) Process(req any) (bus *promotion.Bus, err error) {
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return bus, promotion.NewInternalErrorf("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus

	// If dynamic promotion is enabled use custom properties to set the promoter, else use the default promoter
	if config.Promotion.Dynamic.Enabled {
		p.logger.Debug("processing dynamic promotion, assigned promoter...")
		bus.Context.Promoter = promotion.NewDynamicPromoter(p.logger, bus.Repository.CustomProperties,
			config.Promotion.Dynamic.Key, config.Promotion.Dynamic.Class)
	} else {
		p.logger.Info("dynamic promotion is disabled... defaulting to standard promoter")
		bus.Context.Promoter = promotion.NewDefaultPromoter()
	}
	return bus, nil
}
