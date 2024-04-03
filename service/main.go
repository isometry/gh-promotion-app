package main

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/isometry/gh-promotion-app/internal/handler"
)

type Request = events.APIGatewayV2HTTPRequest
type Response = events.APIGatewayV2HTTPResponse

type Runtime struct {
	app handler.App
}

func (r *Runtime) ServeHTTP(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	slog.Info("received request")

	body, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		slog.Error("failed to read request body", slog.Any("error", err))
		http.Error(httpResponse, "failed to read request body", http.StatusInternalServerError)
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

	response, err := r.app.HandleEvent(context.TODO(), handler.Request{
		Body:    request.Body,
		Headers: request.Headers,
	})

	httpResponse.WriteHeader(response.StatusCode)
	httpResponse.Write([]byte(response.Body))
}

func main() {
	slog.Info("spawned service")

	runtime := Runtime{app: handler.App{}}

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

	s.ListenAndServe()
}
