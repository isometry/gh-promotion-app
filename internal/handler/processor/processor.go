// Package processor provides a generic interface for processing requests using a list of processors.
package processor

import (
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/promotion"
)

// Option is a function that applies an option to a Processor
type Option = func(Processor)

// Processor is an interface that defines a method to process a request
type Processor interface {
	SetLogger(logger *slog.Logger)
	Process(any) (*promotion.Bus, error)
}

// AuthRequest is a struct that represents an authentication request
type AuthRequest struct {
	Body            []byte
	Headers         map[string]string
	EventProcessors map[string][]Processor
}

// Process is a function that processes a request using a list of processors
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

// WithLogger is a function that sets a logger on a Processor
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
