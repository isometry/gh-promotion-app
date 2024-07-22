package processor

import (
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/capabilities"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type dynamicPromotionProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

func NewDynamicPromotionPreProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
	_inst := &dynamicPromotionProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *dynamicPromotionProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("pre-processor:validator")
}

func (p *dynamicPromotionProcessor) Process(req any) (bus *promotion.Bus, err error) {
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus

	// If dynamic promotion is enabled use custom properties to set the promoter, else use the default promoter
	if capabilities.Promotion.DynamicPromotion.Enabled {
		p.logger.Debug("assigning promoter...")
		bus.Context.Promoter = promotion.NewDynamicPromoter(p.logger, bus.Repository.CustomProperties, capabilities.Promotion.DynamicPromotion.Key)
	} else {
		p.logger.Info("dynamic promotion is disabled... defaulting to standard promoter")
		bus.Context.Promoter = promotion.NewDefaultPromoter()
	}
	return bus, nil
}
