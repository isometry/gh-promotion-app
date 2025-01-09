// Package runtime provides the runtime for the application.
package runtime

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/models"
)

// Option defines a function type used to configure a Runtime instance during initialization.
type Option func(*Runtime)

// WithLogger sets the logger for the Runtime instance, enabling structured logging for runtime processes.
func WithLogger(logger *slog.Logger) Option {
	return func(r *Runtime) {
		r.logger = logger
	}
}

// Runtime represents the execution context integrating the handler and logger for processing runtime events.
type Runtime struct {
	*handler.Handler
	logger *slog.Logger
}

// NewRuntime creates a new runtime instance.
func NewRuntime(handler *handler.Handler, opts ...Option) *Runtime {
	_inst := &Runtime{Handler: handler}
	for _, opt := range opts {
		opt(_inst)
	}
	if _inst.logger == nil {
		_inst.logger = helpers.NewNoopLogger()
	}
	return _inst
}

// Lambda is the entrypoint function when the `--mode lambda` flag is set.
func (r *Runtime) Lambda(req models.Request) (response any, err error) {
	r.logger.Info("received request")

	// Lower-case incoming headers for compatibility purposes
	headers := make(map[string]string, len(req.Headers))
	for k, v := range req.Headers {
		headers[k] = strings.ToLower(v)
	}

	bus, err := r.Handler.Process([]byte(req.Body), headers)
	if err != nil {
		return nil, err
	}

	payloadType := r.Handler.GetLambdaPayloadType()
	switch payloadType {
	case "api-gateway-v1":
		return events.APIGatewayProxyResponse{
			Body:       bus.Response.Body,
			StatusCode: bus.Response.StatusCode,
		}, err
	case "api-gateway-v2":
		return events.APIGatewayV2HTTPResponse{
			Body:       bus.Response.Body,
			StatusCode: bus.Response.StatusCode,
		}, err
	case "lambda-url":
		return events.LambdaFunctionURLResponse{
			Body:       bus.Response.Body,
			StatusCode: bus.Response.StatusCode,
		}, err
	default:
		return nil, fmt.Errorf("unsupported lambda payload type: %s", payloadType)
	}
}

// Service is the entrypoint function when the `--mode service` flag is set.
func (r *Runtime) Service(rw http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		break
	default:
		r.logger.Debug("rejecting HTTP request...", slog.Any("requestor", req.RemoteAddr), "reason", "method not allowed", slog.Any("method", req.Method))
		helpers.RespondHTTP(rw, models.Response{StatusCode: http.StatusMethodNotAllowed}, nil)
		return
	}

	r.logger = r.logger.With(slog.Any("requestor", req.RemoteAddr), slog.Any("method", req.Method), slog.Any("path", req.URL.String()), slog.Any("headers", req.Header))
	r.logger.Debug("processing HTTP request...")

	headers := make(map[string]string, len(req.Header))
	for k, v := range req.Header {
		headers[strings.ToLower(k)] = v[0]
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.logger.Error("failed to read request body", slog.Any("error", err))
		helpers.RespondHTTP(rw, models.Response{StatusCode: http.StatusInternalServerError}, err)
		return
	}
	bus, err := r.Handler.Process(body, headers)
	if err != nil {
		r.logger.Error("failed to process request", slog.Any("error", err))
		helpers.RespondHTTP(rw, models.Response{StatusCode: http.StatusInternalServerError}, err)
		return
	}
	helpers.RespondHTTP(rw, bus.Response, err)
}
