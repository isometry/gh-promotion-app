package helpers

import (
	"encoding/json"
	"github.com/isometry/gh-promotion-app/internal/handler"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

type Runtime struct {
	*handler.Handler
	logger *slog.Logger
}

func NewRuntime(handler *handler.Handler, logger *slog.Logger) *Runtime {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})).With("component", "runtime")

	}
	return &Runtime{Handler: handler, logger: logger}
}

func (r *Runtime) HandleEvent(request helpers.Request) (response helpers.Response, err error) {
	slog.Info("received API Gateway V2 request")

	hResponse, err := r.Handler.Run(request)
	slog.Info("handled event", slog.Any("response", hResponse), slog.Any("error", err))
	return helpers.Response{
		Body:       hResponse.Body,
		StatusCode: hResponse.StatusCode,
	}, err
}

func (r *Runtime) ServeHTTP(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	r.logger.Debug("received request")

	body, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		r.logger.Error("failed to read request body", slog.Any("error", err))
		http.Error(httpResponse, "failed to read request body", http.StatusInternalServerError)
		return
	}

	var headers = make(map[string]string)
	for k, v := range httpRequest.Header {
		// XXX: we're losing duplicated headers here
		headers[strings.ToLower(k)] = v[0]
	}

	request := helpers.Request{
		Body:    string(body),
		Headers: headers,
	}

	response, err := r.Handler.Run(request)

	var httpResp = struct {
		Message string `json:"message"`
		Error   any    `json:"error"`
	}{
		Message: response.Body,
		Error:   err,
	}
	respBody, _ := json.Marshal(httpResp)
	httpResponse.WriteHeader(response.StatusCode)
	_, _ = httpResponse.Write(respBody)
}
