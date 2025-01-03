package controllers

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v67/github"
	"github.com/isometry/gh-promotion-app/internal/capabilities"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/isometry/gh-promotion-app/internal/validation"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// EventInstallationID represents an event payload containing an installation ID.
// It embeds the installation details with its respective ID.
type EventInstallationID struct {
	Installation struct {
		ID *int64 `json:"id"`
	} `json:"installation"`
}

// GHOption is a functional option used to configure or modify the properties of a GitHub instance.
type GHOption func(*GitHub)

// NewGitHubController initializes a new GitHub controller with the provided options, setting defaults where necessary.
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
	installationID int64
	V3             *github.Client
	V4             *githubv4.Client
}

// _clientCache - cache of GitHub clients
var _clientCache = make(map[int64]*Client)

// GitHub encapsulates GitHub-related operations and credentials management for various authentication modes.
type GitHub struct {
	Credentials

	authMode      string
	ssmKey        string
	ctx           context.Context
	logger        *slog.Logger
	awsController *AWS
}

// Credentials is a helper struct to hold the GitHub credentials
type Credentials struct {
	AppID         int64                     `json:"app_id,omitempty"`
	PrivateKey    string                    `json:"private_key,omitempty"`
	WebhookSecret *validation.WebhookSecret `json:"webhook_secret"`
	Token         string                    `json:"token,omitempty"`
}

// RetrieveCredentials fetches the GitHub credentials from the environment or SSM
func (g *GitHub) RetrieveCredentials() error {
	switch strings.TrimSpace(strings.ToLower(g.authMode)) {
	case "token":
		if g.Token == "" {
			return fmt.Errorf("missing [GITHUB_TOKEN]]")
		}
		return nil
	case "ssm":
		if g.WebhookSecret != nil && g.AppID != 0 && g.PrivateKey != "" {
			g.logger.Debug("using cached GitHub App credentials...")
			return nil
		}
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

// GetGitHubClients returns a GitHub client for the given installation ID or token
func (g *GitHub) GetGitHubClients(body []byte) (*Client, error) {
	var eventInstallationID EventInstallationID
	if err := json.Unmarshal(body, &eventInstallationID); err != nil {
		return nil, fmt.Errorf("no installation ID found. error: %w", err)
	}

	// Cache hit
	installationID := eventInstallationID.Installation.ID
	if installationID == nil {
		return nil, fmt.Errorf("no installation ID found")
	}
	if client, ok := _clientCache[*installationID]; ok {
		g.logger.Debug("cache hit. using cached client...", slog.Int64("installationId", *installationID))
		return client, nil
	}

	// Cache miss
	g.logger.Debug("cache miss. spawning clients...", slog.Int64("installationId", *installationID))
	var (
		clientV3 *github.Client
		clientV4 *githubv4.Client
	)
	switch strings.TrimSpace(strings.ToLower(g.authMode)) {
	case "token":
		g.logger.Debug("[GITHUB_TOKEN] detected. Spawning clients using PAT...")
		roundTripper := &loggingRoundTripper{logger: g.logger}
		src := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: g.Token},
		)
		httpClient := oauth2.NewClient(g.ctx, src)
		v3rateLimiter, err := github_ratelimit.NewRateLimitWaiterClient(roundTripper)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create rate limiter GitHub client")
		}
		v4rateLimiter, _ := github_ratelimit.NewRateLimitWaiterClient(httpClient.Transport)

		clientV3 = github.NewClient(v3rateLimiter).WithAuthToken(g.Token)
		clientV4 = githubv4.NewClient(v4rateLimiter)
	case "ssm":
		g.logger.Debug("Spawning credentials using GitHub App credentials from SSM...")
		roundTripper := &loggingRoundTripper{logger: g.logger}
		transport, err := ghinstallation.New(roundTripper, g.AppID, *installationID, []byte(g.PrivateKey))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create installation transport")
		}

		rateLimiter, err := github_ratelimit.NewRateLimitWaiterClient(transport)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create rate limiter GitHub client")
		}
		clientV3 = github.NewClient(rateLimiter)
		clientV4 = githubv4.NewClient(rateLimiter)
	default:
		return nil, fmt.Errorf("no valid credentials found")
	}
	// Persist cache entry
	_clientCache[*installationID] = &Client{
		installationID: *installationID,
		V3:             clientV3,
		V4:             clientV4,
	}
	g.logger.Debug("successfully cached spawned clients...", slog.Int64("installationID", *installationID))
	return _clientCache[*installationID], nil
}

// ValidateWebhookSecret verifies the webhook secret against the signature in the provided headers for security validation.
func (g *GitHub) ValidateWebhookSecret(secret []byte, headers map[string]string) error {
	return g.WebhookSecret.ValidateSignature(secret, headers)
}

// PromotionTargetRefExists checks if a ref exists in the repository
func (g *GitHub) PromotionTargetRefExists(ctx *promotion.Context) bool {
	_, _, err := ctx.ClientV3.Git.GetRef(g.ctx, *ctx.Owner, *ctx.Repository, helpers.NormaliseFullRef(ctx.BaseRef))
	return err == nil
}

// CreatePromotionTargetRef creates a new ref in the repository
func (g *GitHub) CreatePromotionTargetRef(pCtx *promotion.Context) (*github.Reference, error) {
	// Fetch the first commit on the head ref
	rootCommit, err := g.GetPromotionSourceRootRef(pCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch root commit")
	}
	ref := &github.Reference{
		Ref: helpers.NormaliseFullRefPtr(pCtx.BaseRef),
		Object: &github.GitObject{
			SHA: rootCommit,
		},
	}
	ref, _, err = pCtx.ClientV3.Git.CreateRef(g.ctx, *pCtx.Owner, *pCtx.Repository, ref)
	return ref, errors.Wrap(err, "failed to create ref")
}

// GetPromotionSourceRootRef fetches the root commit present on the head ref
func (g *GitHub) GetPromotionSourceRootRef(pCtx *promotion.Context) (*string, error) {
	var allCommits []*github.RepositoryCommit
	opts := &github.CommitsListOptions{
		SHA: *pCtx.HeadRef,
		ListOptions: github.ListOptions{
			PerPage: 100, // max allowed value
		},
	}

	for {
		commits, resp, err := pCtx.ClientV3.Repositories.ListCommits(context.Background(), *pCtx.Owner, *pCtx.Repository, opts)
		if err != nil {
			return nil, err
		}
		allCommits = append(allCommits, commits...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	slices.SortFunc(allCommits, func(i, j *github.RepositoryCommit) int {
		di := i.GetCommit().Committer.GetDate()
		dj := j.GetCommit().Committer.GetDate()
		return cmp.Compare(di.Unix(), dj.Unix())
	})
	return allCommits[0].SHA, nil
}

// FindPullRequest searches for an open pull request that matches the promotion request
func (g *GitHub) FindPullRequest(pCtx *promotion.Context) (*github.PullRequest, error) {
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

	g.logger.Debug("Attempting to find matching promotion request...")
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

// CreatePullRequest creates a new pull request in the repository
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

// CommitStatus is a type to represent the commit status
type CommitStatus = string

const (
	// CommitStatusSuccess represents a successful state for a commit status, often used to indicate successful operations.
	CommitStatusSuccess CommitStatus = "success"
	// CommitStatusFailure represents a failed state for a commit status, typically used to indicate unsuccessful operations.
	CommitStatusFailure CommitStatus = "failure"
	// CommitStatusError represents an error state for a commit status, used to indicate critical issues in operations.
	CommitStatusError CommitStatus = "error"
	// CommitStatusPending represents a pending state for a commit status, typically used to indicate ongoing operations.
	CommitStatusPending CommitStatus = "pending"
)

// SendPromotionFeedbackCommitStatus sends a commit status to the head commit of the promotion request
func (g *GitHub) SendPromotionFeedbackCommitStatus(bus *promotion.Bus, promotionError error, commitStatus CommitStatus) error {
	if !capabilities.Promotion.Feedback.Enabled {
		g.logger.Debug("promotion feedback is disabled")
		return nil
	}

	// Validate required fields
	if bus == nil {
		return fmt.Errorf("promotion bus is nil")
	}

	pCtx := bus.Context
	feedbackLogger := pCtx.Logger.WithGroup("feedback:commit-status")
	// Skip if the context is missing required fields
	if pCtx.Owner == nil || pCtx.Repository == nil || pCtx.HeadSHA == nil {
		feedbackLogger.Debug("ignoring promotion feedback commit status due to missing context fields",
			slog.Any("context", pCtx))
		return nil
	}

	// Ignore if the context is missing the pull request reference
	if pCtx.PullRequest == nil {
		feedbackLogger.Debug("ignoring promotion feedback commit status due to missing PullRequest reference",
			slog.Any("context", pCtx))
		return nil
	}

	// Infer the commit status from the provided promotion.Bus and error
	stages, index := pCtx.Promoter.Stages, pCtx.Promoter.StageIndex(*pCtx.HeadRef)
	progress := fmt.Sprintf("%d/%d", index+1, len(stages))

	// Local placeholders
	placeholders := map[string]string{
		"{progress}":  progress,
		"{timestamp}": time.Now().Format(time.RFC3339),
	}
	contextValue := capabilities.Promotion.Feedback.Context
	if pCtx.HeadRef != nil && pCtx.BaseRef != nil {
		placeholders["{source}"] = helpers.ExtractRefFromFullRef(*pCtx.HeadRef)
		placeholders["{target}"] = helpers.ExtractRefFromFullRef(*pCtx.BaseRef)
	}

	// Truncate error (140 max length) -40 lines to allow decoration
	promotionErrorTruncated := helpers.Truncate(fmt.Sprintf("%v", promotionError), 100)

	// Set the commit status message accordingly
	var msg string
	switch commitStatus {
	case CommitStatusSuccess:
		msg = "✅ {progress} @ {timestamp}"
	case CommitStatusFailure:
		msg = fmt.Sprintf("❌ %v @ {timestamp}", promotionErrorTruncated)
	case CommitStatusError:
		msg = fmt.Sprintf("💥 %v @ {timestamp}", promotionErrorTruncated)
	case CommitStatusPending:
		msg = "⏳ {progress} @ {timestamp}"
	default:
		return fmt.Errorf("unknown commit status: %s", commitStatus)
	}

	// Replace placeholders
	for k, v := range placeholders {
		msg = strings.ReplaceAll(msg, k, v)
		contextValue = strings.ReplaceAll(contextValue, k, v)
	}

	// Truncate (140 max length)
	msg = helpers.Truncate(msg, 140)

	status := &github.RepoStatus{
		Description: github.String(msg),
		Context:     github.String(contextValue),
		State:       github.String(commitStatus),
		TargetURL:   pCtx.PullRequest.HTMLURL,
	}

	feedbackLogger.Debug("sending commit status",
		slog.String("status", commitStatus), slog.String("context", contextValue), slog.String("msg", msg),
		slog.String("eventType", fmt.Sprintf("%T", pCtx.EventType)), slog.Any("promotionError", promotionError))
	_, resp, err := pCtx.ClientV3.Repositories.CreateStatus(g.ctx, *pCtx.Owner, *pCtx.Repository, *pCtx.HeadSHA, status)

	if err != nil {
		var body []byte
		if resp != nil && resp.Body != nil {
			body, _ = io.ReadAll(resp.Body)
		}
		feedbackLogger.Error("failed to send commit status", slog.Any("error", err), slog.String("body", string(body)))
		return errors.Wrapf(err, "failed to create commit status. status: %s, body: %s", status, body)
	}
	feedbackLogger.Debug("successfully sent commit status", slog.Any("status", status.String()), slog.Any("sha", *pCtx.HeadSHA))

	return nil
}

// CommitOnBranchRequest is a request to create a commit on a branch
type CommitOnBranchRequest struct {
	Owner, Repository, Branch, Message string
}

// EmptyCommitOnBranch creates an empty commit on a specific branch in a GitHub repository using GraphQL mutation.
// It validates the current branch head and updates the expected head OID in the mutation input for consistency.
// Returns the OID of the newly created commit on success or an error on failure.
func (g *GitHub) EmptyCommitOnBranch(ctx context.Context, clients *Client, createCommitOnBranchInput githubv4.CreateCommitOnBranchInput) (string, error) {
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
		return "", errors.Wrap(err, "EmptyCommitOnBranch: failed create empty commit on branch")
	}

	createCommitOnBranchInput.ExpectedHeadOid = query.Repository.Ref.Target.Oid
	var mutation struct {
		CreateCommitOnBranch struct {
			Commit struct {
				Oid githubv4.GitObjectID
				URL githubv4.String
			}
		} `graphql:"createCommitOnBranch(input: $input)"`
	}

	if err := clients.V4.Mutate(ctx, &mutation, createCommitOnBranchInput, nil); err != nil {
		return "", errors.Wrap(err, "failed to create commit on branch")
	}
	return string(mutation.CreateCommitOnBranch.Commit.Oid), nil
}

// GitHubEmptyCommitOnBranchWithDefaultClient creates an empty commit on a branch using the default client
func GitHubEmptyCommitOnBranchWithDefaultClient(ctx context.Context, req githubv4.CreateCommitOnBranchInput, opts ...GHOption) (string, error) {
	ctl, err := NewGitHubController(opts...)
	if err != nil {
		return "", err
	}
	var clients *Client
	if clients, err = ctl.GetGitHubClients(nil); err != nil {
		return "", err
	}
	return ctl.EmptyCommitOnBranch(ctx, clients, req)
}

// RequestTitle generates a title for a promotion request
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

// RoundTrip logs the request and response
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
	if err != nil {
		l.logger.Log(req.Context(), slog.Level(-8), "failed to send request", slog.Any("error", err))
		return nil, err
	}
	l.logger.Log(req.Context(), slog.Level(-8), "received response", slog.Any("status", resp.Status), slog.Any("error", err))
	return resp, err
}
