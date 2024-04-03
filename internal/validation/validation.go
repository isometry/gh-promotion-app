package validation

import (
	"fmt"
	"strings"

	"github.com/google/go-github/v59/github"
)

type WebhookSecret string

func (s *WebhookSecret) ValidateSignature(body []byte, headers map[string]string) (err error) {
	signature := headers[strings.ToLower(github.SHA256SignatureHeader)]
	if signature == "" {
		return fmt.Errorf("missing HMAC-SHA256 signature")
	}

	if contentType := headers["content-type"]; contentType != "application/json" {
		return fmt.Errorf("unsupported content type: %s", contentType)
	}

	return github.ValidateSignature(signature, body, []byte(*s))
}
