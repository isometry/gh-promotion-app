package controllers

import (
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/validation"
)

// WithToken sets the GitHub authentication token for the GitHub instance.
func WithToken(token string) GHOption {
	return func(a *GitHub) {
		a.Token = token
	}
}

// WithAuthMode sets the authentication mode for a GitHub instance using the given mode string.
func WithAuthMode(mode string) GHOption {
	return func(a *GitHub) {
		a.authMode = mode
	}
}

// WithAWSController sets the awsController field of a GitHub instance with the provided AWS instance.
func WithAWSController(aws *AWS) GHOption {
	return func(a *GitHub) {
		a.awsController = aws
	}
}

// WithSSMKey sets the SSM key used for fetching credentials and applies it to the GitHub instance.
func WithSSMKey(key string) GHOption {
	return func(a *GitHub) {
		a.ssmKey = key
	}
}

// WithLogger sets a custom logger for the GitHub instance to use for logging operations.
func WithLogger(logger *slog.Logger) GHOption {
	return func(a *GitHub) {
		a.logger = logger
	}
}

// WithWebhookSecret configures a GitHub instance to use the provided webhook secret for validating webhook signatures.
func WithWebhookSecret(secret *validation.WebhookSecret) GHOption {
	return func(a *GitHub) {
		a.WebhookSecret = secret
	}
}
