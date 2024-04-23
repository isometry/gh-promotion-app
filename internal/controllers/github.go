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
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v60/github"
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

type GitHub struct {
	Credentials

	authMode      string
	ssmKey        string
	ctx           context.Context
	logger        *slog.Logger
	clientV3      *github.Client
	clientV4      *githubv4.Client
	awsController *AWS

	tokenTTL time.Time
}

type Credentials struct {
	AppId         int64                     `json:"app_id,omitempty"`
	PrivateKey    string                    `json:"private_key,omitempty"`
	WebhookSecret *validation.WebhookSecret `json:"webhook_secret"`
	Token         string                    `json:"token,omitempty"`
}

// @TODO -> Separate into two functions initialise (Authorize) && authenticate (GetClients)

func (g *GitHub) Authenticate(body []byte) error {
	roundTripper := &loggingRoundTripper{logger: g.logger}
	g.logger.Debug("initialising clients...", slog.String("authMode", g.authMode))
	switch strings.TrimSpace(strings.ToLower(g.authMode)) {
	case "token":
		if g.clientV3 != nil && g.clientV4 != nil {
			g.logger.Debug("clients already initialised. Skipping...")
			return nil
		}

		if g.Token == "" {
			g.logger.Debug("[GITHUB_TOKEN] not found. Falling back to SSM...")
			g.authMode = "ssm"
			return g.Authenticate(body)
		}

		g.logger.Debug("[GITHUB_TOKEN] detected. Spawning clients using PAT...")
		g.clientV3 = github.NewClient(&http.Client{Transport: roundTripper}).WithAuthToken(g.Token)
		src := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: g.Token},
		)
		httpClient := oauth2.NewClient(g.ctx, src)
		g.clientV4 = githubv4.NewClient(httpClient)
		g.logger.Debug("successfully spawned clients using PAT...")
		return nil
	case "ssm", "":
		g.logger.Debug("fetching credentials using GitHub App credentials from SSM...")
		if !g.tokenTTL.After(time.Now()) && g.clientV3 != nil && g.clientV4 != nil {
			g.logger.Debug("existing valid token found. Skipping...")
			return nil
		}

		g.logger.Debug("spawning clients using GitHub application credentials from SSM...")
		secret, err := g.awsController.GetSecret(g.ssmKey, true)
		if err != nil {
			return errors.Wrap(err, "failed to fetch GitHub App credentials from SSM")
		}
		if err = json.Unmarshal([]byte(*secret), &g.Credentials); err != nil {
			return errors.Wrap(err, "failed to unmarshal GitHub App credentials")
		}
		var eventInstallationId EventInstallationId
		if err = json.Unmarshal(body, &eventInstallationId); err != nil {
			return fmt.Errorf("no installation ID found. error: %v", err)
		}

		installationId := eventInstallationId.Installation.ID
		g.logger.Debug("using installation ID from event...", slog.Int64("installationId", *installationId))
		transport, err := ghinstallation.New(roundTripper, g.AppId, *installationId, []byte(g.PrivateKey))
		if err != nil {
			return errors.Wrap(err, "failed to create installation transport")
		}

		authTransport := &http.Client{Transport: transport}
		g.clientV3 = github.NewClient(authTransport)
		g.clientV4 = githubv4.NewClient(authTransport)
		// set token TTL to 30 seconds before expiry to allow for some leeway
		g.tokenTTL = time.Now().Add(30 * time.Second)
	case "vault":
		panic("vault auth mode not implemented")
	default:
		return fmt.Errorf("unsupported auth mode: %s", g.authMode)
	}

	return nil
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

	prs, _, err := g.clientV3.PullRequests.List(g.ctx, *pCtx.Owner, *pCtx.Repository, prListOptions)
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
	pr, _, err := g.clientV3.PullRequests.Create(g.ctx, *pCtx.Owner, *pCtx.Repository, &github.NewPullRequest{
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
	_, _, err := g.clientV3.Git.UpdateRef(g.ctx, *pCtx.Owner, *pCtx.Repository, &reference, false)
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

func (g *GitHub) EmptyCommitOnBranch(ctx context.Context, createCommitOnBranchInput githubv4.CreateCommitOnBranchInput) (string, error) {
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

	if err := g.clientV4.Query(ctx, &query, variables); err != nil {
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

	if err := g.clientV4.Mutate(ctx, &mutation, createCommitOnBranchInput, nil); err != nil {
		return "", errors.Wrap(err, "failed to create commit on branch")
	}
	return string(mutation.CreateCommitOnBranch.Commit.Oid), nil
}

func GitHubEmptyCommitOnBranchWithDefaultClient(ctx context.Context, req githubv4.CreateCommitOnBranchInput, opts ...GHOption) (string, error) {
	ctl, err := NewGitHubController(opts...)
	if err != nil {
		return "", err
	}
	if err = ctl.Authenticate(nil); err != nil {
		return "", err
	}
	return ctl.EmptyCommitOnBranch(ctx, req)
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
