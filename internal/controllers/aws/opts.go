package aws

import (
	"context"
	"log/slog"
)

// WithLogger sets a custom slog.Logger instance for the Controller struct to use for logging operations.
func WithLogger(logger *slog.Logger) Option {
	return func(a *Controller) {
		a.logger = logger
	}
}

// WithContext sets a custom context to be used by the Controller instance for request operations.
func WithContext(ctx context.Context) Option {
	return func(a *Controller) {
		a.ctx = ctx
	}
}
