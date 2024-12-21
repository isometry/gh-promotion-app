package controllers

import (
	"context"
	"log/slog"
)

// WithAWSLogger sets a custom slog.Logger instance for the AWS struct to use for logging operations.
func WithAWSLogger(logger *slog.Logger) Option {
	return func(a *AWS) {
		a.logger = logger
	}
}

// WithAWSContext sets a custom context to be used by the AWS instance for request operations.
func WithAWSContext(ctx context.Context) Option {
	return func(a *AWS) {
		a.ctx = ctx
	}
}
