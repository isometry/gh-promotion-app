package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v60/github"
	"github.com/isometry/gh-promotion-app/internal/validation"
)

type GHOption func(*GitHub)

func WithToken(token string) GHOption {
	return func(a *GitHub) {
		a.Token = token
	}
}

func WithApplication(appId int64, privateKey string) GHOption {
	return func(a *GitHub) {
		a.AppId = appId
		a.PrivateKey = privateKey
	}
}

func NewGitHubController(opts ...GHOption) (*GitHub, error) {
	_inst := new(GitHub)
	for _, opt := range opts {
		opt(_inst)
	}
	if _inst.ctx == nil {
		_inst.ctx = context.Background()
	}
	if _inst.logger == nil {
		_inst.logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})).With("controller", "github")
	}
	if err := _inst.initialiseClients(opts...); err != nil {
		return nil, errors.Wrap(err, "controller.github: failed to initialise clients")
	}
	return _inst, nil
}

type GitHub struct {
	Credentials

	ctx      context.Context
	logger   *slog.Logger
	clientV3 *github.Client
	clientV4 *githubv4.Client
}

type Credentials struct {
	AppId          int64                     `json:"app_id,omitempty"`
	PrivateKey     string                    `json:"private_key,omitempty"`
	WebhookSecret  *validation.WebhookSecret `json:"webhook_secret"`
	Token          string                    `json:"token,omitempty"`
	InstallationId int64                     `json:"installation_id,omitempty"`
	HmacSecret     []byte                    `json:"-"`
}

func (g *GitHub) initialiseClients(options ...GHOption) error {
	for _, opt := range options {
		opt(g)
	}

	roundTripper := &loggingRoundTripper{logger: g.logger}
	if g.Token != "" {
		g.logger.Debug("[GITHUB_TOKEN] detected. Spawning clients using PAT...")
		g.clientV3 = github.NewClient(&http.Client{Transport: roundTripper}).WithAuthToken(g.Token)
		src := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: g.Token},
		)
		httpClient := oauth2.NewClient(g.ctx, src)
		g.clientV4 = githubv4.NewClient(httpClient)
		return nil
	}

	g.logger.Debug("Spawning clients using GitHub application credentials...")
	transport, err := ghinstallation.NewAppsTransport(roundTripper, g.AppId, []byte(g.PrivateKey))
	if err != nil {
		return err
	}

	authTransport := &http.Client{Transport: transport}
	g.clientV3 = github.NewClient(authTransport)
	g.clientV4 = githubv4.NewClient(authTransport)

	return nil
}

func (g *GitHub) ValidateWebhookSecret(secret []byte, headers map[string]string) error {
	return g.WebhookSecret.ValidateSignature(secret, headers)
}

func (g *GitHub) FindPullRequest(pCtx promotion.Context) (*github.PullRequest, error) {
	g.logger.Info("Finding promotion requests...", slog.String("owner", *pCtx.Owner), slog.String("repository", *pCtx.Repository))
	prListOptions := &github.PullRequestListOptions{
		State: "open",
	}

	if pCtx.HeadRef != nil && *pCtx.HeadRef != "" {
		g.logger.Info("Limiting promotion request search to head ref...", slog.String("head", *pCtx.HeadRef))
		prListOptions.Head = *pCtx.HeadRef
	}

	if pCtx.BaseRef != nil && *pCtx.BaseRef != "" {
		// limit scope if we have g base ref
		g.logger.Info("Limiting promotion request search to base ref...", slog.String("base", *pCtx.BaseRef))
		prListOptions.Base = *pCtx.BaseRef
	}

	prs, _, err := g.clientV3.PullRequests.List(g.ctx, *pCtx.Owner, *pCtx.Repository, prListOptions)
	if err != nil {
		g.logger.Error("Failed to list pull requests...", slog.Any("error", err))
		return nil, err
	}

	for _, pr := range prs {
		if *pr.Head.SHA == *pCtx.HeadRef && pCtx.Promoter.IsPromotionRequest(pr) {
			g.logger.Info("Found matching promotion request...", slog.String("pr", *pr.URL))
			return pr, nil
		}
	}

	return nil, fmt.Errorf("no matching promotion request found")
}

func (g *GitHub) CreatePullRequest(pCtx promotion.Context) (*github.PullRequest, error) {
	pr, _, err := g.clientV3.PullRequests.Create(g.ctx, *pCtx.Owner, *pCtx.Repository, &github.NewPullRequest{
		Title:               g.RequestTitle(pCtx),
		Head:                pCtx.HeadRef,
		Base:                pCtx.BaseRef,
		MaintainerCanModify: github.Bool(false),
	})

	if err != nil {
		return nil, err
	}

	return pr, nil
}

// FastForwardRefToSha pushes a commit to a ref, used to merge an open pull request via fast-forward
func (g *GitHub) FastForwardRefToSha(pCtx promotion.Context) error {
	ctxLogger := g.logger.With(slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA), slog.String("owner", *pCtx.Owner), slog.String("repository", *pCtx.Repository))
	ctxLogger.Debug("Attempting fast forward...", slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA))
	reference := github.Reference{
		Ref: helpers.NormaliseRefPtr(*pCtx.BaseRef),
		Object: &github.GitObject{
			SHA: pCtx.HeadSHA,
		},
	}
	_, _, err := g.clientV3.Git.UpdateRef(g.ctx, *pCtx.Owner, *pCtx.Repository, &reference, false)
	if err != nil {
		ctxLogger.Error("failed fast forward", slog.Any("error", err))
		return err
	}

	ctxLogger.Debug("Successful fast forward")
	return nil
}

func (g *GitHub) RequestTitle(pCtx promotion.Context) *string {
	title := fmt.Sprintf(
		"Promote %s to %s",
		strings.TrimPrefix(*pCtx.HeadRef, "refs/heads/"),
		strings.TrimPrefix(*pCtx.BaseRef, "refs/heads/"),
	)
	return &title
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
