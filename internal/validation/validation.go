// Package validation provides functionality for validating webhook signatures to verify request authenticity.
package validation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-github/v68/github"
)

// WebhookSecret represents a secret used to validate webhook signatures for verifying request authenticity.
type WebhookSecret string

// NewWebhookSecret creates a new WebhookSecret instance from the provided secret string pointer and returns its address.
func NewWebhookSecret(secret string) *WebhookSecret {
	s := WebhookSecret(secret)
	return &s
}

// ValidateSignature validates the HMAC-SHA256 signature of a webhook request using the provided body and headers.
func (s *WebhookSecret) ValidateSignature(body []byte, headers map[string]string) error {
	if s == nil {
		return errors.New("missing webhook secret")
	}
	signature, found := headers[strings.ToLower(github.SHA256SignatureHeader)]
	if !found {
		return errors.New("missing HMAC-SHA256 signature")
	}

	if contentType := headers["content-type"]; contentType != "application/json" {
		return fmt.Errorf("unsupported content type: %s", contentType)
	}

	return github.ValidateSignature(signature, body, []byte(*s))
}
