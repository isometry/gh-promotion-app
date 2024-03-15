package promotion

import (
	"context"
	"fmt"
	"github.com/google/go-github/v60/github"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
	"log/slog"
	"os"
	"strings"
)

type Context struct {
	Client     *github.Client
	ClientV4   *githubv4.Client
	Logger     *slog.Logger
	EventType  *string
	Owner      *string
	Repository *string
	BaseRef    *string
	HeadRef    *string
	HeadSHA    *string
	Promoter   *Promoter
}

func String(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (p *Context) LogValue() slog.Value {
	logAttr := make([]slog.Attr, 1, 6)
	logAttr[0] = slog.String("eventType", String(p.EventType))
	if p.Owner != nil {
		logAttr = append(logAttr, slog.String("owner", String(p.Owner)))
	}
	if p.Repository != nil {
		logAttr = append(logAttr, slog.String("repository", String(p.Repository)))
	}
	if p.HeadSHA != nil {
		logAttr = append(logAttr, slog.String("headSHA", String(p.HeadSHA)))
	}
	if p.HeadRef != nil {
		logAttr = append(logAttr, slog.String("headRef", String(p.HeadRef)))
	}
	if p.BaseRef != nil {
		logAttr = append(logAttr, slog.String("baseRef", String(p.BaseRef)))
	}
	return slog.GroupValue(logAttr...)
}

func (p *Context) FindPullRequest(ctx context.Context) (pr *github.PullRequest, err error) {
	p.Logger.Info("finding promotion requests", slog.String("owner", *p.Owner), slog.String("repository", *p.Repository))
	prListOptions := &github.PullRequestListOptions{
		State: "open",
	}

	if p.HeadRef != nil && *p.HeadRef != "" {
		p.Logger.Info("limiting promotion request search to head ref", slog.String("head", *p.HeadRef))
		prListOptions.Head = *p.HeadRef
	}

	if p.BaseRef != nil {
		// limit scope if we have a base ref
		p.Logger.Info("limiting promotion request search to base ref", slog.String("base", *p.BaseRef))
		prListOptions.Base = *p.BaseRef
	}

	prs, _, err := p.Client.PullRequests.List(ctx, *p.Owner, *p.Repository, prListOptions)
	if err != nil {
		p.Logger.Error("failed to list pull requests", slog.Any("error", err))
		return nil, err
	}

	for _, pr := range prs {
		if *pr.Head.SHA == *p.HeadSHA && p.Promoter.IsPromotionRequest(pr) {
			p.Logger.Info("found matching promotion request", slog.String("pr", *pr.URL))
			p.HeadRef = pr.Head.Ref
			p.BaseRef = pr.Base.Ref
			return pr, nil
		}
	}

	p.Logger.Info("no matching promotion request found")
	return nil, new(NoPromotionRequestError)
}

func (p *Context) RequestTitle() *string {
	title := fmt.Sprintf(
		"Promote %s to %s",
		strings.TrimPrefix(*p.HeadRef, "refs/heads/"),
		strings.TrimPrefix(*p.BaseRef, "refs/heads/"),
	)
	return &title
}

func (p *Context) CreatePullRequest(ctx context.Context) (*github.PullRequest, error) {
	pr, _, err := p.Client.PullRequests.Create(ctx, *p.Owner, *p.Repository, &github.NewPullRequest{
		Title:               p.RequestTitle(),
		Head:                p.HeadRef,
		Base:                p.BaseRef,
		MaintainerCanModify: github.Bool(false),
	})

	if err != nil {
		return nil, err
	}

	return pr, nil
}

// FastForwardRefToSha pushes a commit to a ref, used to merge an open pull request via fast-forward
func (p *Context) FastForwardRefToSha(ctx context.Context) error {
	ctxLogger := p.Logger.With(slog.String("headRef", *p.HeadRef), slog.String("headSHA", *p.HeadSHA), slog.String("owner", *p.Owner), slog.String("repository", *p.Repository))
	ctxLogger.Info("attempting fast forward", slog.String("headRef", *p.HeadRef), slog.String("headSHA", *p.HeadSHA))
	reference := github.Reference{
		Ref: StageRef(*p.BaseRef),
		Object: &github.GitObject{
			SHA: p.HeadSHA,
		},
	}
	_, _, err := p.Client.Git.UpdateRef(ctx, *p.Owner, *p.Repository, &reference, false)
	if err != nil {
		ctxLogger.Error("failed fast forward", slog.Any("error", err))
		return err
	}

	ctxLogger.Info("successful fast forward")
	return nil
}

func (p *Context) EmptyCommitOnBranch(ctx context.Context, createCommitOnBranchInput githubv4.CreateCommitOnBranchInput) (string, error) {
	return EmptyCommitOnBranch(ctx, p.ClientV4, createCommitOnBranchInput)
}

type CommitOnBranchRequest struct {
	Owner, Repository, Branch, Message string
}

func EmptyCommitOnBranchWithDefaultClient(ctx context.Context, req githubv4.CreateCommitOnBranchInput) (string, error) {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)

	httpClient := oauth2.NewClient(context.Background(), src)
	client := githubv4.NewClient(httpClient)
	return EmptyCommitOnBranch(ctx, client, req)
}

func EmptyCommitOnBranch(ctx context.Context, client *githubv4.Client, createCommitOnBranchInput githubv4.CreateCommitOnBranchInput) (string, error) {
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

	if err := client.Query(ctx, &query, variables); err != nil {
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

	if err := client.Mutate(ctx, &mutation, createCommitOnBranchInput, nil); err != nil {
		return "", errors.Wrap(err, "failed to create commit on branch")
	}
	return string(mutation.CreateCommitOnBranch.Commit.Oid), nil
}
