package github

import (
	"log/slog"

	"github.com/isometry/gh-promotion-app/internal/controllers/aws"
	"github.com/isometry/gh-promotion-app/internal/validation"
)

// WithToken sets the Controller authentication token for the Controller instance.
func WithToken(token string) GHOption {
	return func(a *Controller) {
		a.Token = token
	}
}

// WithAuthMode sets the authentication mode for a Controller instance using the given mode string.
func WithAuthMode(mode string) GHOption {
	return func(a *Controller) {
		a.authMode = mode
	}
}

// WithAWSController sets the awsController field of a Controller instance with the provided Controller instance.
func WithAWSController(aws *aws.Controller) GHOption {
	return func(a *Controller) {
		a.awsController = aws
	}
}

// WithSSMKey sets the SSM key used for fetching credentials and applies it to the Controller instance.
func WithSSMKey(key string) GHOption {
	return func(a *Controller) {
		a.ssmKey = key
	}
}

// WithLogger sets a custom logger for the Controller instance to use for logging operations.
func WithLogger(logger *slog.Logger) GHOption {
	return func(a *Controller) {
		a.logger = logger
	}
}

// WithWebhookSecret configures a Controller instance to use the provided webhook secret for validating webhook signatures.
func WithWebhookSecret(secret *validation.WebhookSecret) GHOption {
	return func(a *Controller) {
		a.WebhookSecret = secret
	}
}
