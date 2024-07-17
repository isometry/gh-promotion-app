package runtime

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/isometry/gh-promotion-app/internal/controllers"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
)

type Option func(*Runtime)

func WithLogger(logger *slog.Logger) Option {
	return func(r *Runtime) {
		r.logger = logger
	}
}

type Runtime struct {
	*handler.Handler
	logger *slog.Logger
}

// NewRuntime creates a new runtime instance
func NewRuntime(handler *handler.Handler, opts ...Option) *Runtime {
	_inst := &Runtime{Handler: handler}
	for _, opt := range opts {
		opt(_inst)
	}
	if _inst.logger == nil {
		_inst.logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return _inst
}

// HandleEvent is the Lambda handler for the runtime
func (r *Runtime) HandleEvent(req helpers.Request) (response any, err error) {
	r.logger.Info("received API Gateway request")

	// Lower-case incoming headers for compatibility purposes
	lch := make(map[string]string)
	for k, v := range req.Headers {
		lch[k] = strings.ToLower(v)
	}

	result, err := r.Handler.Process([]byte(req.Body), lch)

	// Extensions
	r.extensions(result, err)

	payloadType := r.Handler.GetLambdaPayloadType()
	switch payloadType {
	case "api-gateway-v1":
		return events.APIGatewayProxyResponse{
			Body:       result.Response.Body,
			StatusCode: result.Response.StatusCode,
		}, err
	case "api-gateway-v2":
		return events.APIGatewayV2HTTPResponse{
			Body:       result.Response.Body,
			StatusCode: result.Response.StatusCode,
		}, err
	case "lambda-url":
		return events.LambdaFunctionURLResponse{
			Body:       result.Response.Body,
			StatusCode: result.Response.StatusCode,
		}, err
	default:
		return nil, fmt.Errorf("unsupported lambda payload type: %s", payloadType)
	}
}

// ServeHTTP is the HTTP handler for the runtime
func (r *Runtime) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		break
	default:
		r.logger.Debug("rejecting HTTP request...", slog.Any("requestor", req.RemoteAddr), "reason", "method not allowed", slog.Any("method", req.Method))
		helpers.RespondHTTP(helpers.Response{StatusCode: http.StatusMethodNotAllowed}, nil, resp)
		return
	}

	r.logger.Debug("received HTTP request...", slog.Any("requestor", req.RemoteAddr), slog.Any("method", req.Method), slog.Any("path", req.URL.Path), slog.Any("headers", req.Header))
	r.logger.Debug("normalising headers...")
	headers := make(map[string]string)
	for k, v := range req.Header {
		headers[strings.ToLower(k)] = v[0]
	}

	r.logger.Debug("processing request...")
	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.logger.Error("failed to read request body", slog.Any("error", err))
		helpers.RespondHTTP(helpers.Response{StatusCode: http.StatusInternalServerError}, err, resp)
		return
	}
	result, err := r.Handler.Process(body, headers)
	// Extensions
	r.extensions(result, err)
	helpers.RespondHTTP(result.Response, err, resp)
}

// extensions is a helper function to execute additional runtime extensions
func (r *Runtime) extensions(promotionResult *promotion.Result, err error) {
	// send feedback commit status: error
	if err != nil {
		status := controllers.CommitStatusFailure
		var promotionErr *promotion.InternalError
		if errors.As(err, &promotionErr) {
			status = controllers.CommitStatusError
		}
		if statusErr := r.Handler.SendFeedbackCommitStatus(promotionResult, err, status); statusErr != nil {
			r.logger.Error("failed to send feedback commit status", slog.Any("error", statusErr))
		}
	}

	// Rate limits: if fetchRateLimits is enabled, fetch rate limits
	if rateLimits, err := r.Handler.RateLimits(promotionResult); err != nil {
		r.logger.Warn("failed to fetch rate limits", slog.Any("error", err))
	} else {
		r.logger.Info("rate limits fetched", slog.Any("rateLimits", rateLimits))
	}
}
