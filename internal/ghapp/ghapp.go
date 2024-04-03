package ghapp

import (
	"net/http"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v59/github"
	"github.com/isometry/gh-promotion-app/internal/validation"
)

type GHAppCredentials struct {
	AppId         int64                     `json:"app_id"`
	PrivateKey    string                    `json:"private_key"`
	WebhookSecret *validation.WebhookSecret `json:"webhook_secret"`
}

type GHAppEvent interface {
	GetInstallation() *github.Installation
}

func (g *GHAppCredentials) GetAppClient() (*github.Client, error) {
	transport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, g.AppId, []byte(g.PrivateKey))
	if err != nil {
		return nil, err
	}

	client := github.NewClient(&http.Client{Transport: transport})

	return client, nil
}

func (g *GHAppCredentials) GetInstallationClient(installationId int64) (*github.Client, error) {
	itr, err := ghinstallation.New(http.DefaultTransport, g.AppId, installationId, []byte(g.PrivateKey))
	if err != nil {
		return nil, err
	}

	client := github.NewClient(&http.Client{Transport: itr})
	return client, nil
}
