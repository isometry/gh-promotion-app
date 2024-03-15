package ghapp

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v59/github"
	"github.com/isometry/gh-promotion-app/internal/validation"
)

type GHAppCredentials struct {
	AppId          int64                     `json:"app_id,omitempty"`
	PrivateKey     string                    `json:"private_key,omitempty"`
	WebhookSecret  *validation.WebhookSecret `json:"webhook_secret"`
	Token          string                    `json:"token,omitempty"`
	InstallationId int64                     `json:"installation_id,omitempty"`
}

type GHAppEvent interface {
	GetInstallation() *github.Installation
}

func (g *GHAppCredentials) GetClient() (*github.Client, *githubv4.Client, error) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	roundTripper := &loggingRoundTripper{logger: logger}

	if g.Token != "" {
		cv3 := github.NewClient(&http.Client{Transport: roundTripper}).WithAuthToken(g.Token)
		src := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: g.Token},
		)
		httpClient := oauth2.NewClient(context.Background(), src)
		cv4 := githubv4.NewClient(httpClient)
		return cv3, cv4, nil
	}

	transport, err := ghinstallation.NewAppsTransport(roundTripper, g.AppId, []byte(g.PrivateKey))
	if err != nil {
		return nil, nil, err
	}

	authTransport := &http.Client{Transport: transport}
	cv3 := github.NewClient(authTransport)
	cv4 := githubv4.NewClient(authTransport)

	return cv3, cv4, nil
}

type loggingRoundTripper struct {
	logger *slog.Logger
}

func (l *loggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var buf bytes.Buffer
	if req.Body != nil {
		_, _ = io.ReadAll(io.TeeReader(req.Body, &buf))
		req.Body = io.NopCloser(&buf)
	}
	var container map[string]any
	_ = json.NewDecoder(&buf).Decode(&container)
	l.logger.Log(req.Context(), slog.Level(-8), "sending request", slog.String("method", req.Method), slog.String("url", req.URL.String()), slog.Any("body", container))
	resp, err := http.DefaultTransport.RoundTrip(req)
	l.logger.Log(req.Context(), slog.Level(-8), "received response", slog.Any("status", resp.Status), slog.Any("error", err))
	return resp, err
}
