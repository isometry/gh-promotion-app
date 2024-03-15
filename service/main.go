package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/isometry/gh-promotion-app/internal/handler"
)

type (
	Request = events.APIGatewayV2HTTPRequest
)

type Runtime struct {
	app *handler.App
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})).With("mode", "service")
	logger.Info("spawning...")
	runtime := Runtime{app: &handler.App{
		Promoter: promotion.NewStagePromoter(promotion.DefaultStages),
		Logger:   logger,
	}}

	h := http.NewServeMux()
	h.HandleFunc("/", runtime.ServeHTTP)

	s := &http.Server{
		Handler:      h,
		Addr:         net.JoinHostPort("127.0.0.1", "8443"),
		WriteTimeout: 1 * time.Second,
		ReadTimeout:  1 * time.Second,
		IdleTimeout:  1 * time.Second,
		TLSConfig:    &tls.Config{MinVersion: tls.VersionTLS13},
	}

	_ = s.ListenAndServe()
}

func (r *Runtime) ServeHTTP(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	r.app.Logger.Debug("received request")

	body, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		r.app.Logger.Error("failed to read request body", slog.Any("error", err))
		http.Error(httpResponse, "failed to read request body", http.StatusInternalServerError)
		return
	}

	var headers = make(map[string]string)
	for k, v := range httpRequest.Header {
		// XXX: we're losing duplicated headers here
		headers[strings.ToLower(k)] = v[0]
	}

	request := Request{
		Body:    string(body),
		Headers: headers,
	}

	response, err := r.app.HandleEvent(context.Background(), handler.Request{
		Body:    request.Body,
		Headers: request.Headers,
	})

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
