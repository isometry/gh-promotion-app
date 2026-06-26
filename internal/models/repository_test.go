package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryContextUnmarshalCustomPropertiesStoresRawValues(t *testing.T) {
	payload := []byte(`{
		"name": "repo",
		"full_name": "owner/repo",
		"owner": {"login": "owner"},
		"custom_properties": {
			"promotion_stages": "dev,stage,prod",
			"unrelated_multi_select": ["one", "two"],
			"unrelated_null": null,
			"create_pull_request_in_draft_mode": true
		}
	}`)

	var repo RepositoryContext
	require.NoError(t, json.Unmarshal(payload, &repo))

	require.Contains(t, repo.CustomProperties, "promotion_stages")
	assert.JSONEq(t, `"dev,stage,prod"`, string(repo.CustomProperties["promotion_stages"]))
	require.Contains(t, repo.CustomProperties, "create_pull_request_in_draft_mode")
	assert.JSONEq(t, `true`, string(repo.CustomProperties["create_pull_request_in_draft_mode"]))
	require.Contains(t, repo.CustomProperties, "unrelated_multi_select")
	assert.JSONEq(t, `["one", "two"]`, string(repo.CustomProperties["unrelated_multi_select"]))
	require.Contains(t, repo.CustomProperties, "unrelated_null")
	assert.JSONEq(t, `null`, string(repo.CustomProperties["unrelated_null"]))
}
