package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/bradleyfalzon/ghinstallation/v2"

	"github.com/google/go-github/v59/github"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

var (
	promotionStages = []string{"main", "staging", "canary", "production"}
)

type NoPullRequestError struct{}

func (m *NoPullRequestError) Error() string {
	return "no pull request found"
}

func validateSignature(headers map[string]string, body string, secretToken []byte) (err error) {
	signature := headers[github.SHA256SignatureHeader]
	if signature == "" {
		return fmt.Errorf("missing signature")
	}

	contentType := headers["Content-Type"]
	if contentType != "application/json" {
		return fmt.Errorf("unsupported content type: %s", contentType)
	}

	return github.ValidateSignature(signature, []byte(body), secretToken)
}

func githubAppClient(appId int64, privateKey []byte) (*github.Client, error) {
	transport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appId, privateKey)
	if err != nil {
		return nil, err
	}

	client := github.NewClient(&http.Client{Transport: transport})

	return client, nil
}

func githubAppInstallationClient(appId int64, privateKey []byte, installationId int64) (*github.Client, error) {
	itr, err := ghinstallation.New(http.DefaultTransport, appId, installationId, privateKey)
	if err != nil {
		return nil, err
	}

	client := github.NewClient(&http.Client{Transport: itr})
	return client, nil
}

func findOpenPullRequest(c *github.Client, owner, repository, headRef, baseRef string) (pr *github.PullRequest, err error) {
	prs, _, err := c.PullRequests.List(context.Background(), owner, repository, &github.PullRequestListOptions{
		State: "open",
		Head:  headRef,
		Base:  baseRef,
	})
	if err != nil {
		return nil, err
	}

	if len(prs) == 0 {
		return nil, &NoPullRequestError{}
	}

	return prs[0], nil
}

func handleRequest(ctx context.Context, request events.LambdaFunctionURLRequest) (response *string, err error) {
	err = validateSignature(request.Headers, request.Body, []byte(os.Getenv("GITHUB_WEBHOOK_SECRET")))
	if err != nil {
		return nil, err
	}

	// request is valid

	payload := json.RawMessage(request.Body)
	eventType := request.Headers[github.EventTypeHeader]
	rawEvent := github.Event{
		Type:       &eventType,
		RawPayload: &payload,
	}

	event, err := rawEvent.ParsePayload()
	if err != nil {
		return nil, err
	}

	var (
		appId          int64
		installationId int64
		owner          string
		repository     string
		pullRequest    *github.PullRequest
		headRef        string
		baseRef        string
	)

	switch e := event.(type) {
	case *github.DeploymentStatusEvent:
		appId = *e.Installation.AppID
		installationId = *e.Installation.ID
		owner = *e.Repo.Owner.Login
		repository = *e.Repo.Name
		headRef = *e.Deployment.Ref
	case *github.PullRequestEvent:
		appId = *e.Installation.AppID
		installationId = *e.Installation.ID
		owner = *e.Repo.Owner.Login
		repository = *e.Repo.Name
		pullRequest = e.PullRequest
		headRef = *e.PullRequest.Head.Ref
		baseRef = *e.PullRequest.Base.Ref
	case *github.PushEvent:
		appId = *e.Installation.AppID
		installationId = *e.Installation.ID
		owner = *e.Repo.Owner.Login
		repository = *e.Repo.Name
		headRef = *e.Ref
	}

	if appId == 0 || installationId == 0 {
		return nil, fmt.Errorf("missing app or installation id")
	}

	privateKey := []byte(os.Getenv("GITHUB_PRIVATE_KEY"))
	if privateKey == nil {
		return nil, fmt.Errorf("missing private key")
	}

	client, err := githubAppInstallationClient(appId, privateKey, installationId)
	if err != nil {
		return nil, err
	}

	if pullRequest == nil {
		pullRequest, err = findOpenPullRequest(client, owner, repository, headRef, baseRef)
		if err != nil {
			return nil, err
		}
	}

	return
}

func localRepoUrl(path string) (repoPath string) {
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	repoPath = fmt.Sprintf("file://%s", filepath.Join(pwd, path))

	return
}

func cloneRepo(repoUrl, branch string) (r *git.Repository, err error) {
	return git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:           repoUrl,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		NoCheckout:    true,
	})
}

// fastForward pushes the src branch to the dst branch
func fastForward(repo *git.Repository, src, dst string) (err error) {
	return repo.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", src, dst))},
	})
}

func testFastForward(path, src, dst string) {
	repo, err := cloneRepo(localRepoUrl(path), src)
	if err != nil {
		log.Fatal(err)
	}

	if err := fastForward(repo, src, dst); err != nil {
		log.Fatal(err)
	}
}

func main() {
	// src := os.Args[1]
	// dst := os.Args[2]
	// testFastForward("testrepo", src, dst)

	lambda.Start(handleRequest)
}
