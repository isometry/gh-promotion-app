package controllers

import (
	"context"
	"log/slog"
)

//func WithConfig(cfg *aws.Config) Option {
//	return func(c *AWS) {
//		c.config = cfg
//	}
//}

func WithAWSLogger(logger *slog.Logger) Option {
	return func(a *AWS) {
		a.logger = logger
	}
}

func WithAWSContext(ctx context.Context) Option {
	return func(a *AWS) {
		a.ctx = ctx
	}
}
