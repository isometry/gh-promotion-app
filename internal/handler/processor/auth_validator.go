package processor

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/go-github/v68/github"
	"github.com/isometry/gh-promotion-app/internal/config"
	internalGitHub "github.com/isometry/gh-promotion-app/internal/controllers/github"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/event"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type authValidatorProcessor struct {
	logger           *slog.Logger
	githubController *internalGitHub.Controller
}

// NewAuthValidatorProcessor initializes and returns a new Processor for validating authentication against Controller operations.
// It accepts a Controller and optional configuration options to customize its behavior.
func NewAuthValidatorProcessor(githubController *internalGitHub.Controller, opts ...Option) Processor {
	_inst := &authValidatorProcessor{githubController: githubController, logger: helpers.NewNoopLogger()}
	applyOpts(_inst, opts...)
	return _inst
}

func (p *authValidatorProcessor) SetLogger(logger *slog.Logger) {
	p.logger = logger.WithGroup("pre-processor:validator")
}

func (p *authValidatorProcessor) Process(req any) (bus *promotion.Bus, err error) {
	authRequest, ok := req.(*AuthRequest)
	if !ok {
		return nil, promotion.NewInternalErrorf("invalid event type. expected *AuthRequest got %T", req)
	}
	body, headers := authRequest.Body, authRequest.Headers

	// Default response
	bus = &promotion.Bus{
		Body:     body,
		Headers:  headers,
		Response: models.Response{StatusCode: http.StatusUnprocessableEntity},
	}

	p.logger.Info("processing request...")

	eventType, found := headers[strings.ToLower(github.EventTypeHeader)]
	if !found {
		p.logger.Error("missing event type")
		return &promotion.Bus{
			Response: models.Response{Body: "missing event type", StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("missing event type")
	}
	bus.EventType = event.Type(eventType)

	deliveryID, found := headers[strings.ToLower(github.DeliveryIDHeader)]
	if !found {
		p.logger.Error("missing delivery ID")
		return &promotion.Bus{
			Response: models.Response{Body: "missing delivery ID", StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("missing delivery ID")
	}

	// Validate the request
	resp, err := p.checkEventType(bus.EventType, authRequest.EventProcessors)
	if err != nil {
		p.logger.Error("failed to validate request", slog.Any("error", err))
		return &promotion.Bus{
			Response: *resp,
		}, promotion.NewInternalErrorf("failed to validate request. error: %v", err)
	}

	// Add the event type to the logger now that we know it's valid
	p.logger = p.logger.With(slog.String("event", eventType))

	// Refresh credentials if needed
	if err = p.githubController.RetrieveCredentials(); err != nil {
		p.logger.Error("failed to refresh credentials", slog.Any("error", err))
		return &promotion.Bus{
			Response: models.Response{Body: err.Error(), StatusCode: http.StatusUnauthorized},
		}, promotion.NewInternalErrorf("failed to refresh credentials. error: %v", err)
	}
	if config.Global.Mode != config.ModeLambdaEvent {
		if err = p.githubController.ValidateWebhookSecret(body, headers); err != nil {
			p.logger.Error("failed to validate signature", slog.Any("error", err))
			return &promotion.Bus{
				Response: models.Response{Body: err.Error(), StatusCode: http.StatusForbidden},
			}, promotion.NewInternalErrorf("failed to validate signature. error: %v", err)
		}
		p.logger.Debug("request body is valid")
	} else {
		p.logger.Debug("skipping webhook signature validation in lambda-event mode...")
	}

	// Add the delivery ID to the logger, now that we know the payload is valid
	p.logger = p.logger.With(slog.String("deliveryID", deliveryID))

	repo, err := p.extractRepositoryContext(body)
	if err != nil {
		p.logger.Error("failed to extract repository context", slog.Any("error", err))
		return &promotion.Bus{
			Response: models.Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalErrorf("failed to extract repository context. error: %v", err)
	}

	p.logger = p.logger.With(slog.Any("repo", repo.FullName))
	p.logger.Debug("authenticating...")
	var clients *internalGitHub.Client
	if clients, err = p.githubController.GetGitHubClients(body); err != nil {
		p.logger.Error("failed to authenticate", slog.Any("error", err))
		return &promotion.Bus{
			Response: models.Response{Body: err.Error(), StatusCode: http.StatusUnauthorized},
		}, promotion.NewInternalErrorf("failed to authenticate. error: %v", err)
	}

	evt, err := github.ParseWebHook(eventType, body)
	if err != nil {
		p.logger.Error("failed to parse webhook payload", slog.Any("error", err))
		return &promotion.Bus{
			Response: models.Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalErrorf("failed to parse webhook payload. error: %v", err)
	}

	bus.Event = evt
	bus.EventStatus = promotion.Error
	bus.Repository = repo
	bus.Context = &promotion.Context{
		EventType:  evt,
		Owner:      repo.Owner.Login,
		Repository: repo.Name,
		Logger:     p.logger.WithGroup("runtime:promotion"),
		ClientV3:   clients.V3,
		ClientV4:   clients.V4,
	}

	return bus, nil
}

func (p *authValidatorProcessor) checkEventType(eventType event.Type, definedTypes map[event.Type][]Processor) (*models.Response, error) {
	_, ok := definedTypes[eventType]
	if !ok {
		msg := fmt.Sprintf("unsupported event type: %s", eventType)
		return &models.Response{Body: msg, StatusCode: http.StatusBadRequest}, errors.New(msg)
	}
	return nil, nil
}

func (p *authValidatorProcessor) extractRepositoryContext(body []byte) (*models.RepositoryContext, error) {
	var eventRepository models.EventRepository
	if err := json.Unmarshal(body, &eventRepository); err != nil {
		return nil, fmt.Errorf("event repository not found. error: %w", err)
	}

	return &eventRepository.Repository, nil
}
