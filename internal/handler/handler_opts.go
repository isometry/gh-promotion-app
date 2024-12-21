package handler

import (
	"context"
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/validation"
)

// WithLogger sets the logger instance for the handler.
func WithLogger(logger *slog.Logger) Option {
	return func(h *Handler) {
		h.logger = logger
	}
}

// WithContext sets the context for the handler.
func WithContext(ctx context.Context) Option {
	return func(h *Handler) {
		h.ctx = ctx
	}
}

// WithAuthMode sets the authentication mode for the handler. It is applied as a functional option during initialization.
func WithAuthMode(authMode string) Option {
	return func(h *Handler) {
		h.authMode = authMode
	}
}

// WithLambdaPayloadType sets the lambda payload type for a Handler instance.
func WithLambdaPayloadType(payloadType string) Option {
	return func(h *Handler) {
		h.lambdaPayloadType = payloadType
	}
}

// WithSSMKey sets the SSM key for retrieving credentials and adds it as an option to the handler configuration.
func WithSSMKey(key string) Option {
	return func(h *Handler) {
		h.ssmKey = key
	}
}

// WithToken sets the GitHub token used for authentication in the handler.
func WithToken(token string) Option {
	return func(h *Handler) {
		h.ghToken = token
	}
}

// WithWebhookSecret configures the handler with a webhook secret for request validation.
func WithWebhookSecret(secret string) Option {
	return func(h *Handler) {
		h.webhookSecret = validation.NewWebhookSecret(secret)
	}
}
