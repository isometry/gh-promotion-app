// Package github provides a Controller for GitHub operations and credentials management.
package github

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
	"text/template"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v68/github"
	"github.com/isometry/gh-promotion-app/internal/config"
	"github.com/isometry/gh-promotion-app/internal/controllers/aws"
	"github.com/isometry/gh-promotion-app/internal/controllers/github/templates"
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/isometry/gh-promotion-app/internal/validation"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"

	_ "embed"
)

// EventInstallationID represents an event payload containing an installation ID.
// It embeds the installation details with its respective ID.
type EventInstallationID struct {
	Installation struct {
		ID *int64 `json:"id"`
	} `json:"installation"`
}

// GHOption is a functional option used to configure or modify the properties of a Controller instance.
type GHOption func(*Controller)

// NewController initializes a new Controller with the provided options, setting defaults where necessary.
func NewController(opts ...GHOption) (*Controller, error) {
	_inst := new(Controller)
	for _, opt := range opts {
		opt(_inst)
	}
	if _inst.ctx == nil {
		_inst.ctx = context.Background()
	}
	if _inst.logger == nil {
		_inst.logger = helpers.NewNoopLogger()
	}
	_inst.logger.With("authMode", _inst.authMode)
	return _inst, nil
}

// Client - cache struct holding an entry for each installation ID.
type Client struct {
	installationID int64
	V3             *github.Client
	V4             *githubv4.Client
}

// _clientCache - cache of Controller clients.
var _clientCache = make(map[int64]*Client)

// Controller encapsulates Controller-related operations and credentials management for various authentication modes.
type Controller struct {
	Credentials

	authMode      string
	ssmKey        string
	ctx           context.Context
	logger        *slog.Logger
	awsController *aws.Controller
}

// Credentials is a helper struct to hold the Controller credentials.
type Credentials struct {
	AppID         int64                     `json:"app_id,omitempty"`
	PrivateKey    string                    `json:"private_key,omitempty"`
	WebhookSecret *validation.WebhookSecret `json:"webhook_secret"`
	Token         string                    `json:"token,omitempty"`
}

// RetrieveCredentials fetches the Controller credentials from the environment or SSM.
func (g *Controller) RetrieveCredentials() error {
	switch strings.TrimSpace(strings.ToLower(g.authMode)) {
	case "token":
		if g.Token == "" {
			return errors.New("missing [GITHUB_TOKEN]]")
		}
		return nil
	case "ssm":
		if g.WebhookSecret != nil && g.AppID != 0 && g.PrivateKey != "" {
			g.logger.Debug("using cached Controller App credentials...")
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

// GetGitHubClients returns a Controller client for the given installation ID or token.
func (g *Controller) GetGitHubClients(body []byte) (*Client, error) {
	var eventInstallationID EventInstallationID
	if err := json.Unmarshal(body, &eventInstallationID); err != nil {
		return nil, fmt.Errorf("no installation ID found. error: %w", err)
	}

	// Cache hit
	installationID := eventInstallationID.Installation.ID
	if installationID == nil {
		return nil, errors.New("no installation ID found")
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
			return nil, errors.Wrap(err, "failed to create rate limiter Controller client")
		}
		v4rateLimiter, _ := github_ratelimit.NewRateLimitWaiterClient(httpClient.Transport)

		clientV3 = github.NewClient(v3rateLimiter).WithAuthToken(g.Token)
		clientV4 = githubv4.NewClient(v4rateLimiter)
	case "ssm":
		g.logger.Debug("Spawning credentials using Controller App credentials from SSM...")
		roundTripper := &loggingRoundTripper{logger: g.logger}
		transport, err := ghinstallation.New(roundTripper, g.AppID, *installationID, []byte(g.PrivateKey))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create installation transport")
		}

		rateLimiter, err := github_ratelimit.NewRateLimitWaiterClient(transport)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create rate limiter Controller client")
		}
		clientV3 = github.NewClient(rateLimiter)
		clientV4 = githubv4.NewClient(rateLimiter)
	default:
		return nil, errors.New("no valid credentials found")
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
func (g *Controller) ValidateWebhookSecret(secret []byte, headers map[string]string) error {
	return g.WebhookSecret.ValidateSignature(secret, headers)
}

// PromotionTargetRefExists checks if a ref exists in the repository.
func (g *Controller) PromotionTargetRefExists(ctx *promotion.Context) bool {
	_, _, err := ctx.ClientV3.Git.GetRef(g.ctx, *ctx.Owner, *ctx.Repository, helpers.NormaliseFullRef(ctx.BaseRef))
	return err == nil
}

// CreatePromotionTargetRef creates a new ref in the repository.
func (g *Controller) CreatePromotionTargetRef(pCtx *promotion.Context) (*github.Reference, error) {
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

// GetPromotionSourceRootRef fetches the root commit present on the head ref.
func (g *Controller) GetPromotionSourceRootRef(pCtx *promotion.Context) (*string, error) {
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

// FindPullRequest searches for an open pull request that matches the promotion request.
func (g *Controller) FindPullRequest(pCtx *promotion.Context) (*github.PullRequest, error) {
	g.logger.Info("finding promotion requests...", slog.String("owner", *pCtx.Owner), slog.String("repository", *pCtx.Repository))
	prListOptions := &github.PullRequestListOptions{
		State: "open",
	}

	if pCtx.HeadSHA == nil {
		return nil, errors.New("head SHA is missing")
	}

	if pCtx.HeadRef != nil && *pCtx.HeadRef != "" {
		g.logger.Info("limiting promotion request search to head ref...", slog.String("head", *pCtx.HeadRef))
		prListOptions.Head = *pCtx.HeadRef
	}

	if pCtx.BaseRef != nil && *pCtx.BaseRef != "" {
		// limit scope if we have base ref
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

	return nil, errors.New("no matching promotion request found")
}

// ListPullRequestCommits fetches all commits present in the pull request.
func (g *Controller) ListPullRequestCommits(pCtx *promotion.Context) ([]*github.RepositoryCommit, error) {
	var allCommits []*github.RepositoryCommit
	options := &github.ListOptions{PerPage: 60}

	for {
		commits, resp, err := pCtx.ClientV3.PullRequests.ListCommits(g.ctx, *pCtx.Owner, *pCtx.Repository, *pCtx.PullRequest.Number, options)
		if err != nil {
			return nil, err
		}
		allCommits = append(allCommits, commits...)
		if resp.NextPage == 0 {
			break
		}
		options.Page = resp.NextPage
	}

	slices.SortFunc(allCommits, func(i, j *github.RepositoryCommit) int {
		di := i.GetCommit().GetCommitter().GetDate()
		dj := j.GetCommit().GetCommitter().GetDate()
		return cmp.Compare(dj.Unix(), di.Unix())
	})
	return allCommits, nil
}

// CreatePullRequest creates a new pull request in the repository.
func (g *Controller) CreatePullRequest(pCtx *promotion.Context) (*github.PullRequest, error) {
	pr, _, err := pCtx.ClientV3.PullRequests.Create(g.ctx, *pCtx.Owner, *pCtx.Repository, &github.NewPullRequest{
		Title:               g.RequestTitle(*pCtx),
		Head:                pCtx.HeadRef,
		Base:                pCtx.BaseRef,
		MaintainerCanModify: github.Ptr(false),
	})

	if err != nil {
		return nil, err
	}

	return pr, nil
}

// FastForwardRefToSha pushes a commit to a ref, used to merge an open pull request via fast-forward.
func (g *Controller) FastForwardRefToSha(pCtx *promotion.Context) error {
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

// CommitStatus is a type to represent the commit status.
type CommitStatus string

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

// CheckRunStatus is a type to represent the check run status.
type CheckRunStatus string

const (
	// CheckRunStatusCompleted represents a completed state for a check run status, often used to indicate successful operations.
	CheckRunStatusCompleted CheckRunStatus = "completed"
	// CheckRunStatusInProgress represents an in-progress state for a check run status, typically used to indicate ongoing operations.
	CheckRunStatusInProgress CheckRunStatus = "in_progress"
	// CheckRunStatusQueued represents a queued state for a check run status, used to indicate pending operations.
	CheckRunStatusQueued CheckRunStatus = "queued"
)

// CheckRunConclusion is a type to represent the check run conclusion.
type CheckRunConclusion string

const (
	// CheckRunConclusionSuccess represents a successful conclusion for a check run status, often used to indicate successful operations.
	CheckRunConclusionSuccess CheckRunConclusion = "success"
	// CheckRunConclusionFailure represents a failed conclusion for a check run status, typically used to indicate unsuccessful operations.
	CheckRunConclusionFailure CheckRunConclusion = "failure"
	// CheckRunConclusionNeutral represents a neutral conclusion for a check run status, used to indicate non-critical operations.
	CheckRunConclusionNeutral CheckRunConclusion = "neutral"
	// CheckRunConclusionCancelled represents a cancelled conclusion for a check run status, used to indicate aborted operations.
	CheckRunConclusionCancelled CheckRunConclusion = "cancelled"
	// CheckRunConclusionSkipped represents a skipped conclusion for a check run status, used to indicate skipped operations.
	CheckRunConclusionSkipped CheckRunConclusion = "skipped"
	// CheckRunConclusionTimedOut represents a timed-out conclusion for a check run status, used to indicate operations that exceeded the time limit.
	CheckRunConclusionTimedOut CheckRunConclusion = "timed_out"
	// CheckRunConclusionActionRequired represents an action-required conclusion for a check run status, used to indicate operations that require manual intervention.
	CheckRunConclusionActionRequired CheckRunConclusion = "action_required"
)

// SendPromotionFeedbackCommitStatus sends a commit status to the head commit of the promotion request.
func (g *Controller) SendPromotionFeedbackCommitStatus(bus *promotion.Bus, commitStatus CommitStatus) error {
	// Validate required fields
	if bus == nil {
		return errors.New("promotion bus is nil")
	}

	pCtx := bus.Context
	feedbackLogger := pCtx.Logger.WithGroup("feedback:commit-status")

	// Process and filter invalid feedback requests
	msg, contextValue := g.processPromotionFeedback(bus, feedbackLogger, config.Promotion.Feedback.CommitStatus.Context)
	if msg == nil || contextValue == nil {
		return promotion.NewInternalError("invalid feedback request. callee: processPromotionFeedback")
	}

	status := &github.RepoStatus{
		Description: msg,
		Context:     contextValue,
		State:       github.Ptr(string(commitStatus)),
		TargetURL:   pCtx.PullRequest.HTMLURL,
	}

	feedbackLogger.Debug("sending commit status",
		slog.String("status", string(commitStatus)), slog.String("context", *contextValue), slog.String("msg", *msg),
		slog.String("eventType", fmt.Sprintf("%T", pCtx.EventType)), slog.Any("error", bus.Error))
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

//go:embed templates/check-run.md.tmpl
var checkRunTemplate string

// SendPromotionFeedbackCheckRun sends a check run to the head commit of the promotion request.
func (g *Controller) SendPromotionFeedbackCheckRun(bus *promotion.Bus, conclusion CheckRunConclusion) error {
	// Validate required fields
	if bus == nil {
		return errors.New("promotion bus is nil")
	}

	pCtx := bus.Context
	feedbackLogger := pCtx.Logger.WithGroup("feedback:check-run")

	// Process and filter invalid feedback requests
	msg, nameValue := g.processPromotionFeedback(bus, feedbackLogger, config.Promotion.Feedback.CheckRun.Name)
	if msg == nil || nameValue == nil {
		feedbackLogger.Error("invalid feedback request", slog.String("callee", "processPromotionFeedback"))
		return promotion.NewInternalError("invalid feedback request")
	}

	var (
		textMessage  *string
		errorMessage string
	)

	if bus.Error != nil {
		errorMessage = strings.Replace(bus.Error.Error(), "[]", "", 1)
	}

	textTmpl, err := template.New("check-run").Funcs(templates.StandardFuncs).Parse(checkRunTemplate)
	if err != nil {
		feedbackLogger.Error("failed to parse check-run template", slog.Any("error", err))
		return promotion.NewInternalErrorf("failed to parse check-run template: %v", err)
	}

	mermaid, err := bus.Context.Promoter.Mermaid(pCtx.PullRequest, pCtx.Commits, bus.Error)
	if err != nil {
		feedbackLogger.Error("failed to generate Mermaid mermaid", slog.Any("error", err))
	}

	metadata := map[string]any{
		"promotion": map[string]any{
			"path": pCtx.Promoter.Stages,
		},
	}

	var textBuffer bytes.Buffer
	if err = textTmpl.Execute(&textBuffer, struct {
		ErrorMessage string
		Mermaid      string
		Commits      []*github.RepositoryCommit
		Metadata     map[string]any
	}{
		ErrorMessage: errorMessage,
		Mermaid:      mermaid,
		Commits:      pCtx.Commits,
		Metadata:     metadata,
	}); err != nil {
		feedbackLogger.Error("failed to execute check-run template", slog.Any("error", err))
		return promotion.NewInternalErrorf("failed to execute check-run template: %v", err)
	}
	textMessage = github.Ptr(textBuffer.String())

	now := time.Now().UTC()
	checkRunOpts := github.CreateCheckRunOptions{
		Name:        *nameValue,
		HeadSHA:     *pCtx.HeadSHA,
		Status:      github.Ptr(string(CheckRunStatusCompleted)),
		Conclusion:  github.Ptr(string(conclusion)),
		CompletedAt: &github.Timestamp{Time: now},
		Output: &github.CheckRunOutput{
			Title:   msg,
			Summary: nameValue,
			Text:    textMessage,
		},
	}

	feedbackLogger.Debug("creating check-run...",
		slog.String("conclusion", string(conclusion)), slog.String("context", *nameValue), slog.String("msg", *msg),
		slog.String("eventType", fmt.Sprintf("%T", pCtx.EventType)), slog.Any("error", bus.Error))

	_, resp, err := pCtx.ClientV3.Checks.CreateCheckRun(g.ctx, *pCtx.Owner, *pCtx.Repository, checkRunOpts)
	if err != nil {
		var body []byte
		if resp != nil && resp.Body != nil {
			body, _ = io.ReadAll(resp.Body)
		}
		feedbackLogger.Error("failed to create check-run", slog.Any("error", err), slog.String("body", string(body)))
		return errors.Wrapf(err, "failed to create check-run. conclusion: %s, body: %s", conclusion, body)
	}
	feedbackLogger.Debug("successfully created check-run", slog.Any("conclusion", conclusion), slog.Any("sha", *pCtx.HeadSHA))
	return nil
}

func (g *Controller) processPromotionFeedback(bus *promotion.Bus, logger *slog.Logger, contextValue string) (*string, *string) {
	pCtx := bus.Context
	logger = logger.With(slog.Any("context", pCtx))
	if pCtx == nil {
		logger.Error(promotion.NewInternalError("promotion context is nil").Error())
		return nil, nil
	}

	if pCtx.EventType == promotion.Skipped {
		logger.Debug("ignoring promotion feedback request due to skipped event")
		return nil, nil
	}

	// Skip if the context is missing required fields
	if pCtx.Owner == nil || pCtx.Repository == nil || pCtx.HeadSHA == nil {
		logger.Debug("ignoring promotion feedback request due to missing context fields")
		return nil, nil
	}

	// Ignore if the context is missing the pull request reference
	if pCtx.PullRequest == nil {
		logger.Debug("ignoring promotion feedback request due to missing PullRequest reference")
		return nil, nil
	}

	// Infer the commit status from the provided promotion.Bus and error
	stages, index := pCtx.Promoter.Stages, pCtx.Promoter.StageIndex(*pCtx.HeadRef)
	progress := fmt.Sprintf("%d/%d", index+1, len(stages))

	// Local placeholders
	placeholders := map[string]string{
		"{progress}":  progress,
		"{timestamp}": time.Now().UTC().Format(time.RFC3339),
	}
	if pCtx.HeadRef != nil && pCtx.BaseRef != nil {
		placeholders["{source}"] = helpers.NormaliseRef(*pCtx.HeadRef)
		placeholders["{target}"] = helpers.NormaliseRef(*pCtx.BaseRef)
	}

	// Set the commit status message accordingly
	var msg string
	switch bus.EventStatus { // nolint:exhaustive // Handled prior to switch to short-circuit
	case promotion.Success:
		msg = "✅ {progress} @ {timestamp}"
	case promotion.Failure:
		msg = "⏳ {progress} @ {timestamp}"
	case promotion.Error:
		msg = "⏳ {progress} @ {timestamp}"
	case promotion.Pending:
		msg = "⏳ {progress} @ {timestamp}"
	}

	// Replace placeholders
	for k, v := range placeholders {
		msg = strings.ReplaceAll(msg, k, v)
		contextValue = strings.ReplaceAll(contextValue, k, v)
	}

	// Truncate (140 max length)
	msg = helpers.Truncate(msg, 140)

	return &msg, &contextValue
}

// CommitOnBranchRequest is a request to create a commit on a branch.
type CommitOnBranchRequest struct {
	Owner, Repository, Branch, Message string
}

// EmptyCommitOnBranch creates an empty commit on a specific branch in a Controller repository using GraphQL mutation.
// It validates the current branch head and updates the expected head OID in the mutation input for consistency.
// Returns the OID of the newly created commit on success or an error on failure.
func (g *Controller) EmptyCommitOnBranch(ctx context.Context, clients *Client, createCommitOnBranchInput githubv4.CreateCommitOnBranchInput) (string, error) {
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

// EmptyCommitOnBranchWithDefaultClient creates an empty commit on a branch using the default client.
func EmptyCommitOnBranchWithDefaultClient(ctx context.Context, req githubv4.CreateCommitOnBranchInput, opts ...GHOption) (string, error) {
	ctl, err := NewController(opts...)
	if err != nil {
		return "", err
	}
	var clients *Client
	if clients, err = ctl.GetGitHubClients(nil); err != nil {
		return "", err
	}
	return ctl.EmptyCommitOnBranch(ctx, clients, req)
}

// RequestTitle generates a title for a promotion request.
func (g *Controller) RequestTitle(pCtx promotion.Context) *string {
	title := fmt.Sprintf(
		"Promote %s to %s",
		helpers.NormaliseRef(*pCtx.HeadRef),
		helpers.NormaliseRef(*pCtx.BaseRef),
	)

	return &title
}

type loggingRoundTripper struct {
	logger *slog.Logger
}

// RoundTrip logs the request and response.
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
