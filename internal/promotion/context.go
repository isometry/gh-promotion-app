// Package promotion provides the core functionality for handling promotion events and managing related context and response details.
package promotion

import (
	"log/slog"

	"github.com/google/go-github/v67/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/shurcooL/githubv4"
)

// Bus represents the central data structure for processing events and managing related context and response details.
type Bus struct {
	Context  *Context
	Response models.Response

	EventType string
	Event     any

	Body    []byte
	Headers map[string]string

	Repository *models.RepositoryContext
}

// LogValue returns a slog.Value by delegating to the Context's LogValue method, encapsulating structured log attributes.
func (b *Bus) LogValue() slog.Value {
	return b.Context.LogValue()
}

// Context represents the runtime context for handling promotion events and related GitHub interactions.
type Context struct {
	Logger      *slog.Logger
	EventType   any
	Owner       *string
	Repository  *string
	BaseRef     *string
	HeadRef     *string
	HeadSHA     *string
	PullRequest *github.PullRequest

	Promoter *Promoter
	ClientV3 *github.Client
	ClientV4 *githubv4.Client
}

// LogValue generates a structured log value containing context-related attributes like event type, owner, and repository.
// It dynamically includes optional attributes such as head SHA, head reference, and base reference if they are not nil.
func (p *Context) LogValue() slog.Value {
	logAttr := make([]slog.Attr, 1, 6)
	logAttr[0] = slog.Any("eventType", p.EventType)
	if p.Owner != nil {
		logAttr = append(logAttr, slog.String("owner", helpers.String(p.Owner)))
	}
	if p.Repository != nil {
		logAttr = append(logAttr, slog.String("repository", helpers.String(p.Repository)))
	}
	if p.HeadSHA != nil {
		logAttr = append(logAttr, slog.String("headSHA", helpers.String(p.HeadSHA)))
	}
	if p.HeadRef != nil {
		logAttr = append(logAttr, slog.String("headRef", helpers.String(p.HeadRef)))
	}
	if p.BaseRef != nil {
		logAttr = append(logAttr, slog.String("baseRef", helpers.String(p.BaseRef)))
	}
	return slog.GroupValue(logAttr...)
}
