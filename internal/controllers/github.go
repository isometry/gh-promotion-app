package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v62/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/isometry/gh-promotion-app/internal/validation"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

type EventInstallationId struct {
	Installation struct {
		ID *int64 `json:"id"`
	} `json:"installation"`
}

type GHOption func(*GitHub)

func NewGitHubController(opts ...GHOption) (*GitHub, error) {
	_inst := new(GitHub)
	for _, opt := range opts {
		opt(_inst)
	}
	if _inst.ctx == nil {
		_inst.ctx = context.Background()
	}
	if _inst.logger == nil {
		_inst.logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	_inst.logger.With("authMode", _inst.authMode)
	return _inst, nil
}

// Client - cache struct holding an entry for each installation ID
type Client struct {
	installationId int64
	V3             *github.Client
	V4             *githubv4.Client
}

// _clientCache - cache of GitHub clients
var _clientCache map[int64]*Client = make(map[int64]*Client)

type GitHub struct {
	Credentials

	authMode      string
	ssmKey        string
	ctx           context.Context
	logger        *slog.Logger
	awsController *AWS
}

type Credentials struct {
	AppId         int64                     `json:"app_id,omitempty"`
	PrivateKey    string                    `json:"private_key,omitempty"`
	WebhookSecret *validation.WebhookSecret `json:"webhook_secret"`
	Token         string                    `json:"token,omitempty"`
}

func (g *GitHub) RetrieveCredentials() error {
	g.logger.Debug("retrieving credentials...", slog.String("authMode", g.authMode))
	switch strings.TrimSpace(strings.ToLower(g.authMode)) {
	case "token":
		if g.Token == "" {
			return fmt.Errorf("missing [GITHUB_TOKEN]]")
		}
		return nil
	case "ssm":
		g.logger.Debug("retrieving credentials from SSM...")
		secret, err := g.awsController.GetSecret(g.ssmKey, true)
		if err != nil {
			return errors.Wrap(err, "failed to fetch credentials from SSM")
		}
		if err = json.Unmarshal([]byte(*secret), &g.Credentials); err != nil {
			return errors.Wrap(err, "failed to unmarshal credentials")
		}
	case "vault":
		panic("vault auth mode not implemented")
	default:
		return fmt.Errorf("unsupported auth mode: %s", g.authMode)
	}
	return nil
}

func (g *GitHub) GetGitHubClients(body []byte) (*Client, error) {
	var eventInstallationId EventInstallationId
	if err := json.Unmarshal(body, &eventInstallationId); err != nil {
		return nil, fmt.Errorf("no installation ID found. error: %v", err)
	}

	// Cache hit
	installationId := eventInstallationId.Installation.ID
	if client, ok := _clientCache[*installationId]; ok {
		g.logger.Debug("cache hit. using cached client...", slog.Int64("installationId", *installationId))
		return client, nil
	}

	// Cache miss
	g.logger.Debug("cache miss. spawning clients...", slog.Int64("installationId", *installationId))
	var (
		clientV3 *github.Client
		clientV4 *githubv4.Client
	)
	switch strings.TrimSpace(strings.ToLower(g.authMode)) {
	case "token":
		g.logger.Debug("[GITHUB_TOKEN] detected. Spawning clients using PAT...")
		roundTripper := &loggingRoundTripper{logger: g.logger}
		clientV3 = github.NewClient(&http.Client{Transport: roundTripper}).WithAuthToken(g.Token)
		src := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: g.Token},
		)
		httpClient := oauth2.NewClient(g.ctx, src)
		clientV4 = githubv4.NewClient(httpClient)
	case "ssm":
		g.logger.Debug("Spawning credentials using GitHub App credentials from SSM...")
		roundTripper := &loggingRoundTripper{logger: g.logger}
		transport, err := ghinstallation.New(roundTripper, g.AppId, *installationId, []byte(g.PrivateKey))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create installation transport")
		}

		authTransport := &http.Client{Transport: transport}
		clientV3 = github.NewClient(authTransport)
		clientV4 = githubv4.NewClient(authTransport)
	}
	// Persist cache entry
	_clientCache[*installationId] = &Client{
		installationId: *installationId,
		V3:             clientV3,
		V4:             clientV4,
	}
	g.logger.Debug("successfully cached spawned clients...", slog.Int64("installationId", *installationId))
	return _clientCache[*installationId], nil
}

func (g *GitHub) ValidateWebhookSecret(secret []byte, headers map[string]string) error {
	return g.WebhookSecret.ValidateSignature(secret, headers)
}

func (g *GitHub) FindPullRequest(pCtx *promotion.Context) (*github.PullRequest, error) {
	fmt.Printf("%+v\n", pCtx.Owner)
	fmt.Printf("%+v\n", pCtx.Repository)
	g.logger.Info("finding promotion requests...", slog.String("owner", *pCtx.Owner), slog.String("repository", *pCtx.Repository))
	prListOptions := &github.PullRequestListOptions{
		State: "open",
	}

	if pCtx.HeadRef != nil && *pCtx.HeadRef != "" {
		g.logger.Info("limiting promotion request search to head ref...", slog.String("head", *pCtx.HeadRef))
		prListOptions.Head = *pCtx.HeadRef
	}

	if pCtx.BaseRef != nil && *pCtx.BaseRef != "" {
		// limit scope if we have g base ref
		g.logger.Info("limiting promotion request search to base ref...", slog.String("base", *pCtx.BaseRef))
		prListOptions.Base = *pCtx.BaseRef
	}

	prs, _, err := pCtx.ClientV3.PullRequests.List(g.ctx, *pCtx.Owner, *pCtx.Repository, prListOptions)
	if err != nil {
		g.logger.Error("failed to list pull requests...", slog.Any("error", err))
		return nil, err
	}

	for _, pr := range prs {
		if *pr.Head.SHA == *pCtx.HeadSHA && pCtx.Promoter.IsPromotionRequest(pr) {
			g.logger.Info("found matching promotion request...", slog.String("pr", *pr.URL))
			pCtx.HeadRef = pr.Head.Ref
			pCtx.BaseRef = pr.Base.Ref
			return pr, nil
		}
	}

	return nil, fmt.Errorf("no matching promotion request found")
}

func (g *GitHub) CreatePullRequest(pCtx *promotion.Context) (*github.PullRequest, error) {
	pr, _, err := pCtx.ClientV3.PullRequests.Create(g.ctx, *pCtx.Owner, *pCtx.Repository, &github.NewPullRequest{
		Title:               g.RequestTitle(*pCtx),
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
func (g *GitHub) FastForwardRefToSha(pCtx *promotion.Context) error {
	ctxLogger := g.logger.With(slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA), slog.String("owner", *pCtx.Owner), slog.String("repository", *pCtx.Repository))
	ctxLogger.Debug("attempting fast forward...", slog.String("headRef", *pCtx.HeadRef), slog.String("headSHA", *pCtx.HeadSHA))
	reference := github.Reference{
		Ref: helpers.NormaliseFullRefPtr(*pCtx.BaseRef),
		Object: &github.GitObject{
			SHA: pCtx.HeadSHA,
		},
	}
	_, _, err := pCtx.ClientV3.Git.UpdateRef(g.ctx, *pCtx.Owner, *pCtx.Repository, &reference, false)
	if err != nil {
		ctxLogger.Error("failed fast forward", slog.Any("error", err))
		return err
	}

	ctxLogger.Debug("successful fast forward")
	return nil
}

type CommitOnBranchRequest struct {
	Owner, Repository, Branch, Message string
}

func (g *GitHub) EmptyCommitOnBranch(clients *Client, ctx context.Context, createCommitOnBranchInput githubv4.CreateCommitOnBranchInput) (string, error) {
	// Fetch the current head commit of the branch
	var query struct {
		Repository struct {
			Ref struct {
				Target struct {
					Oid githubv4.GitObjectID
				} `graphql:"target"`
			} `graphql:"ref(qualifiedName: $qualifiedName)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	parts := strings.Split(string(*createCommitOnBranchInput.Branch.RepositoryNameWithOwner), "/")
	variables := map[string]any{
		"owner":         githubv4.String(parts[0]),
		"name":          githubv4.String(parts[1]),
		"qualifiedName": *createCommitOnBranchInput.Branch.BranchName,
	}

	if err := clients.V4.Query(ctx, &query, variables); err != nil {
		return "", errors.Wrap(err, "GetBranchHeadCommit: failed to fetch the current head commit of the branch")
	}

	createCommitOnBranchInput.ExpectedHeadOid = query.Repository.Ref.Target.Oid
	var mutation struct {
		CreateCommitOnBranch struct {
			Commit struct {
				Oid githubv4.GitObjectID
				Url githubv4.String
			}
		} `graphql:"createCommitOnBranch(input: $input)"`
	}

	if err := clients.V4.Mutate(ctx, &mutation, createCommitOnBranchInput, nil); err != nil {
		return "", errors.Wrap(err, "failed to create commit on branch")
	}
	return string(mutation.CreateCommitOnBranch.Commit.Oid), nil
}

func GitHubEmptyCommitOnBranchWithDefaultClient(ctx context.Context, req githubv4.CreateCommitOnBranchInput, opts ...GHOption) (string, error) {
	ctl, err := NewGitHubController(opts...)
	if err != nil {
		return "", err
	}
	var clients *Client
	if clients, err = ctl.GetGitHubClients(nil); err != nil {
		return "", err
	}
	return ctl.EmptyCommitOnBranch(clients, ctx, req)
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
