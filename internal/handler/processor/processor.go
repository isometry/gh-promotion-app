package processor

import (
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type Option = func(Processor)

type Processor interface {
	SetLogger(logger *slog.Logger)
	Process(any) (*promotion.Bus, error)
}

type AuthRequest struct {
	Body            []byte
	Headers         map[string]string
	EventProcessors map[string][]Processor
}

func Process(logger *slog.Logger, req any, processors ...Processor) (*promotion.Bus, error) {
	var err error
	for _, p := range processors {
		p.SetLogger(logger)
		req, err = p.Process(req)
		if err != nil {
			return req.(*promotion.Bus), err
		}
	}
	return req.(*promotion.Bus), err
}

func WithLogger(logger *slog.Logger) Option {
	return func(p Processor) {
		p.SetLogger(logger)
	}
}

func applyOpts(m Processor, opts ...Option) {
	for _, opt := range opts {
		opt(m)
	}
}
