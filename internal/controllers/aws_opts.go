package controllers

import "context"

//func WithConfig(cfg *aws.Config) Option {
//	return func(c *AWS) {
//		c.config = cfg
//	}
//}

func WithContext(ctx context.Context) Option {
	return func(a *AWS) {
		a.ctx = ctx
	}
}
