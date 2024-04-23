package validation_test

import (
	"strings"
	"testing"

	"github.com/google/go-github/v60/github"
	"github.com/isometry/gh-promotion-app/internal/validation"
)

func TestWebhookSecret_ValidateSignature(t *testing.T) {
	testCases := []struct {
		Name        string
		Headers     map[string]string
		Body        string
		ExpectError bool
	}{
		{
			Name:        "invalid_headers",
			Headers:     map[string]string{},
			ExpectError: true,
		},
		{
			Name: "invalid_content_type",
			Headers: map[string]string{
				"content-type": "application/xml",
			},
			ExpectError: true,
		},
		{
			Name: "invalid_signature_value",
			Headers: map[string]string{
				strings.ToLower(github.SHA256SignatureHeader): "invalid",
			},
			ExpectError: true,
		},
		{
			Name: "invalid_signature_sha256",
			Headers: map[string]string{
				strings.ToLower(github.SHA256SignatureHeader): "sha256=844d7743b13e1bdd66b003c29ebe5184dcf985434dde9f125952595cd533213e",
				"content-type": "application/json",
			},
			Body:        `{"key": "value"}`,
			ExpectError: true,
		},
		{
			Name: "valid_signature_sha256",
			Headers: map[string]string{
				strings.ToLower(github.SHA256SignatureHeader): "sha256=bc7daef0d3e3b227f6f1dd1b6e8ee0711a94bfd6a61ca28ec3c4aa22a33d27d8",
				"content-type": "application/json",
			},
			Body: `{"key": "value"}`,
		},
	}

	_inst := validation.WebhookSecret("key")
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			if err := _inst.ValidateSignature([]byte(tc.Body), tc.Headers); (err != nil) != tc.ExpectError {
				t.Errorf("WebhookSecret.ValidateSignature() error = %v, expectError %v", err, tc.ExpectError)
			}
		})
	}
}
