package promotion

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/go-github/v59/github"
)

type Context struct {
	Client     *github.Client
	EventType  *string
	Owner      *string
	Repository *string
	BaseRef    *string
	HeadRef    *string
	HeadSHA    *string
}

func (p *Context) FindRequest() (requestUrl *string, err error) {
	slog.Info("finding promotion requests", slog.String("owner", *p.Owner), slog.String("repository", *p.Repository), slog.String("sha", *p.HeadSHA))
	prListOptions := &github.PullRequestListOptions{
		State: "open",
	}

	if p.HeadRef != nil {
		slog.Info("limiting promotion request search to head ref", slog.String("head", *p.HeadRef))
		prListOptions.Head = *p.HeadRef
	}

	if p.BaseRef != nil {
		// limit scope if we have a base ref
		slog.Info("limiting promotion request search to base ref", slog.String("base", *p.BaseRef))
		prListOptions.Base = *p.BaseRef
	}

	prs, _, err := p.Client.PullRequests.List(context.Background(), *p.Owner, *p.Repository, prListOptions)
	if err != nil {
		slog.Error("failed to list pull requests", slog.Any("error", err))
		return nil, err
	}

	for _, pr := range prs {
		if *pr.Head.SHA == *p.HeadSHA && IsPromotionRequest(pr) {
			slog.Info("found matching promotion request", slog.String("pr", *pr.URL))
			p.HeadRef = pr.Head.Ref
			p.BaseRef = pr.Base.Ref
			return pr.URL, nil
		}
	}

	slog.Info("no matching promotion request found")
	return nil, &NoPromotionRequestError{}
}

func (p *Context) RequestTitle() *string {
	title := fmt.Sprintf(
		"Promote %s to %s",
		strings.TrimPrefix(*p.HeadRef, "refs/heads/"),
		strings.TrimPrefix(*p.BaseRef, "refs/heads/"),
	)
	return &title
}

func (p *Context) CreateRequest() (pr *github.PullRequest, err error) {
	pr, _, err = p.Client.PullRequests.Create(context.Background(), *p.Owner, *p.Repository, &github.NewPullRequest{
		Title: p.RequestTitle(),
		Head:  p.HeadRef,
		Base:  p.BaseRef,
	})
	if err != nil {
		return nil, err
	}

	return pr, nil
}

// FastForwardRefToSha pushes a commit to a ref, used to merge an open pull request via fast-forward
func (p *Context) FastForwardRefToSha(ctx context.Context) error {
	slog.Info("attempting fast forward", slog.String("headRef", *p.HeadRef), slog.String("headSHA", *p.HeadSHA))
	reference := github.Reference{
		Ref: p.BaseRef,
		Object: &github.GitObject{
			SHA: github.String(*p.HeadSHA),
		},
	}
	_, _, err := p.Client.Git.UpdateRef(ctx, *p.Owner, *p.Repository, &reference, false)
	if err != nil {
		slog.Info("failed fast forward", slog.String("headRef", *p.HeadRef), slog.String("headSHA", *p.HeadSHA), slog.Any("error", err))
		return err
	}

	slog.Info("successful fast forward", slog.String("headRef", *p.HeadRef), slog.String("headSHA", *p.HeadSHA))
	return nil
}
