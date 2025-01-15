//go:build test_e2e

package cmd

import (
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed tests/deployment_status.pending.json
var validPayloadDeploymentStatusPending string

//go:embed tests/deployment_status.success.json
var validPayloadDeploymentStatusSuccess string

//go:embed tests/push_event.json
var validPayloadPushEvent string

func generateHmacSha256(payload, key string) string {
	mac := hmac.New(sha256.New, []byte(key))

	mac.Write([]byte(payload))
	b := make([]byte, hex.EncodedLen(sha256.Size))
	hex.Encode(b, mac.Sum(nil))
	return string(b)
}
