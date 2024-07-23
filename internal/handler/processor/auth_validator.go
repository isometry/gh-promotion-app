package processor

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/go-github/v62/github"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type authValidatorProcessor struct {
	logger           *slog.Logger
	githubController *controllers.GitHub
}

func NewAuthValidatorProcessor(githubController *controllers.GitHub, opts ...Option) Processor {
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
		return nil, promotion.NewInternalError("invalid event type. expected *AuthRequest got %T", req)
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
		p.logger.Warn("missing event type")
		return &promotion.Bus{
			Response: models.Response{Body: "missing event type", StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("missing event type")
	}
	bus.EventType = eventType

	deliveryId, found := headers[strings.ToLower(github.DeliveryIDHeader)]
	if !found {
		p.logger.Warn("missing delivery ID")
		return &promotion.Bus{
			Response: models.Response{Body: "missing delivery ID", StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("missing delivery ID")
	}

	// Validate the request
	resp, err := p.checkEventType(eventType, authRequest.EventProcessors)
	if err != nil {
		p.logger.Warn("validating request", slog.Any("error", err))
		return &promotion.Bus{
			Response: *resp,
		}, promotion.NewInternalError("failed to validate request. error: %v", err)
	}

	// Add the event type to the logger now that we know it's valid
	p.logger = p.logger.With(slog.String("event", eventType))

	// Refresh credentials if needed
	if err = p.githubController.RetrieveCredentials(); err != nil {
		p.logger.Warn("failed to refresh credentials", slog.Any("error", err))
		return &promotion.Bus{
			Response: models.Response{Body: err.Error(), StatusCode: http.StatusUnauthorized},
		}, promotion.NewInternalError("failed to refresh credentials. error: %v", err)
	}
	if err = p.githubController.ValidateWebhookSecret(body, headers); err != nil {
		p.logger.Warn("validating signature", slog.Any("error", err))
		return &promotion.Bus{
			Response: models.Response{Body: err.Error(), StatusCode: http.StatusForbidden},
		}, promotion.NewInternalError("failed to validate signature. error: %v", err)
	}
	p.logger.Debug("request body is valid")

	// Add the delivery ID to the logger, now that we know the payload is valid
	p.logger = p.logger.With(slog.String("deliveryId", deliveryId))

	repo, err := p.ExtractCommonRepository(body)
	if err != nil {
		p.logger.Warn("failed to extract repository context", slog.Any("error", err))
		return &promotion.Bus{
			Response: models.Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("failed to extract repository context. error: %v", err)
	}

	p.logger = p.logger.With(slog.Any("repo", repo.FullName))

	// GetGitHubClients the request
	p.logger.Debug("authenticating...")
	var clients *controllers.Client
	if clients, err = p.githubController.GetGitHubClients(body); err != nil {
		p.logger.Warn("failed to authenticate", slog.Any("error", err))
		return &promotion.Bus{
			Response: models.Response{Body: err.Error(), StatusCode: http.StatusUnauthorized},
		}, promotion.NewInternalError("failed to authenticate. error: %v", err)
	}

	event, err := github.ParseWebHook(eventType, body)
	if err != nil {
		p.logger.Warn("parsing webhook payload", slog.Any("error", err))
		return &promotion.Bus{
			Response: models.Response{Body: err.Error(), StatusCode: http.StatusUnprocessableEntity},
		}, promotion.NewInternalError("failed to parse webhook payload. error: %v", err)
	}

	bus.Event = event
	bus.Repository = repo
	bus.Context = &promotion.Context{
		EventType:  event,
		Owner:      repo.Owner.Login,
		Repository: repo.Name,
		Logger:     p.logger.WithGroup("runtime:promotion"),
		ClientV3:   clients.V3,
		ClientV4:   clients.V4,
	}

	return bus, nil
}

func (p *authValidatorProcessor) checkEventType(eventType string, definedTypes map[string][]Processor) (*models.Response, error) {
	_, ok := definedTypes[eventType]
	if !ok {
		return &models.Response{StatusCode: http.StatusBadRequest}, fmt.Errorf("unhandled event type: %s", eventType)
	}
	return nil, nil
}

func (p *authValidatorProcessor) ExtractCommonRepository(body []byte) (*models.CommonRepository, error) {
	var eventRepository models.EventRepository
	if err := json.Unmarshal(body, &eventRepository); err != nil {
		return nil, fmt.Errorf("event repository not found. error: %v", err)
	}

	return &eventRepository.Repository, nil
}
