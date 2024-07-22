package processor

import (
	"log/slog"
	"net/http"

	"github.com/isometry/gh-promotion-app/internal/capabilities"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type rateLimitsPostProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

func NewRateLimitsPostProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
	_inst := &rateLimitsPostProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *rateLimitsPostProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("pre-processor:validator")
}

func (p *rateLimitsPostProcessor) Process(req any) (bus *promotion.Bus, err error) {
	parsedBus, ok := req.(*promotion.Bus)
	if !ok {
		return nil, promotion.NewInternalError("invalid event type. expected *promotion.Bus got %T", req)
	}
	bus = parsedBus
	if !capabilities.Global.FetchRateLimits {
		p.logger.Debug("rate limits fetching disabled")
		return nil, nil
	}

	// Ignore if the response is not 204 (set by the processor.fastForwarderPostProcessor)
	if bus.Response.StatusCode != http.StatusNoContent {
		p.logger.Debug("ignoring rate limits fetching for non-204 response", slog.Int("statusCode", bus.Response.StatusCode))
		return bus, nil
	}

	// Fetch rate limits once a minute
	helpers.OnceAMinute.Do(func() {
		if rateLimits, rErr := p.githubController.RateLimits(bus.Context.ClientV3); rErr != nil {
			p.logger.Warn("failed to fetch rate limits", slog.Any("error", rErr))
			err = rErr
			return
		} else {
			p.logger.Info("rate limits fetched", slog.Any("rateLimits", rateLimits))
		}
	})
	return
}
