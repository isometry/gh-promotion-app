package controllers

import (
	"github.com/isometry/gh-promotion-app/internal/validation"
	"log/slog"
)

func WithToken(token string) GHOption {
	return func(a *GitHub) {
		a.Token = token
	}
}

//func WithApplication(appId int64, privateKey string) GHOption {
//	return func(a *GitHub) {
//		a.AppId = appId
//		a.PrivateKey = privateKey
//	}
//}

func WithAuthMode(mode string) GHOption {
	return func(a *GitHub) {
		a.authMode = mode
	}
}

func WithAWSController(aws *AWS) GHOption {
	return func(a *GitHub) {
		a.awsController = aws
	}
}

func WithSSMKey(key string) GHOption {
	return func(a *GitHub) {
		a.ssmKey = key
	}
}

func WithLogger(logger *slog.Logger) GHOption {
	return func(a *GitHub) {
		a.logger = logger
	}
}

func WithWebhookSecret(secret *validation.WebhookSecret) GHOption {
	return func(a *GitHub) {
		a.WebhookSecret = secret
	}
}
