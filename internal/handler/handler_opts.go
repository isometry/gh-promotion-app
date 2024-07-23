package handler

import (
	"context"
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/validation"
)

func WithLogger(logger *slog.Logger) Option {
	return func(h *Handler) {
		h.logger = logger
	}
}

func WithContext(ctx context.Context) Option {
	return func(h *Handler) {
		h.ctx = ctx
	}
}

func WithAuthMode(authMode string) Option {
	return func(h *Handler) {
		h.authMode = authMode
	}
}

func WithLambdaPayloadType(payloadType string) Option {
	return func(h *Handler) {
		h.lambdaPayloadType = payloadType
	}
}

func WithSSMKey(key string) Option {
	return func(h *Handler) {
		h.ssmKey = key
	}
}

func WithToken(token string) Option {
	return func(h *Handler) {
		h.ghToken = token
	}
}

func WithWebhookSecret(secret string) Option {
	return func(h *Handler) {
		h.webhookSecret = validation.NewWebhookSecret(secret)
	}
}
