// Package event provides a type for the event that triggered the webhook.
package event

import (
	"slices"

	"github.com/isometry/gh-promotion-app/internal/config"
)

// Type represents the type of event that triggered the webhook.
type Type string

const (
	// Push represents a push event type.
	Push Type = "push"
	// PullRequest represents a pull request event type.
	PullRequest Type = "pull_request"
	// PullRequestReview represents a pull request review event type.
	PullRequestReview Type = "pull_request_review"
	// CheckSuite represents a check suite event type.
	CheckSuite Type = "check_suite"
	// DeploymentStatus represents a deployment status event type.
	DeploymentStatus Type = "deployment_status"
	// Status represents a status event type.
	Status Type = "status"
	// WorkflowRun represents a workflow run event type.
	WorkflowRun Type = "workflow_run"
)

// IsEnabled returns true if the event type is enabled.
func IsEnabled(eventType Type) bool {
	return slices.Contains(config.Promotion.Events, string(eventType))
}
