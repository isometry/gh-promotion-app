package ghapp

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v59/github"
)

type GHAppCredentials struct {
	AppId         int64  `json:"app_id"`
	PrivateKey    string `json:"private_key"`
	WebhookSecret string `json:"webhook_secret"`
}

type GHAppEvent interface {
	GetInstallation() *github.Installation
}

func (g *GHAppCredentials) ValidateSignature(body []byte, headers map[string]string) (err error) {
	signature := headers[strings.ToLower(github.SHA256SignatureHeader)]
	if signature == "" {
		return fmt.Errorf("missing HMAC-SHA256 signature")
	}

	if contentType := headers["content-type"]; contentType != "application/json" {
		return fmt.Errorf("unsupported content type: %s", contentType)
	}

	return github.ValidateSignature(signature, body, []byte(g.WebhookSecret))
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
