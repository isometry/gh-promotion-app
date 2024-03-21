package runtime

import (
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"io"
	"log/slog"
	"net/http"
	"strings"
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

func (r *Runtime) HandleEvent(req helpers.Request) (response helpers.Response, err error) {
	r.logger.Info("received API Gateway V2 request")

	hResponse, err := r.Handler.Process([]byte(req.Body), req.Headers)
	r.logger.Info("handled event", slog.Any("response", hResponse), slog.Any("error", err))
	return helpers.Response{
		Body:       hResponse.Body,
		StatusCode: hResponse.StatusCode,
	}, err
}

func (r *Runtime) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		break
	default:
		r.logger.Debug("Rejecting HTTP request...", slog.Any("requestor", req.RemoteAddr), "reason", "method not allowed", slog.Any("method", req.Method))
		resp.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.logger.Debug("Received HTTP request...", slog.Any("requestor", req.RemoteAddr), slog.Any("method", req.Method), slog.Any("path", req.URL.Path), slog.Any("headers", req.Header))
	r.logger.Debug("Normalising headers...")
	headers := make(map[string]string)
	for k, v := range req.Header {
		// XXX: we're losing duplicated headers here
		headers[strings.ToLower(k)] = v[0]
	}

	r.logger.Debug("Processing request...")
	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.logger.Error("Failed to read request body", slog.Any("error", err))
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	response, err := r.Handler.Process(body, headers)
	helpers.NewHttpResponse(response, err, resp)
}
