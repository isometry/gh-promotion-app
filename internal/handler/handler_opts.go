package handler

import (
	"context"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/isometry/gh-promotion-app/internal/validation"
	"log/slog"
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

func WithPromoter(promoter *promotion.Promoter) Option {
	return func(h *Handler) {
		h.promoter = promoter
	}
}

func WithDynamicPromoterKey(key string) Option {
	return func(h *Handler) {
		h.dynamicPromoterKey = key
	}
}
