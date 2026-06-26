package helpers_test

import (
	"testing"

	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/stretchr/testify/assert"
)

func TestNormaliseFullRef(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "full_ref_format",
			Input:    "refs/heads/main",
			Expected: "refs/heads/main",
		},
		{
			Name:     "short_ref_format",
			Input:    "main",
			Expected: "refs/heads/main",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, tc.Expected, helpers.NormaliseFullRef(tc.Input))
		})
	}
}

func TestNormaliseRef(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name:     "full_ref_format",
			Input:    "refs/heads/main",
			Expected: "main",
		},
		{
			Name:     "short_ref_format",
			Input:    "main",
			Expected: "main",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, tc.Expected, helpers.NormaliseRef(tc.Input))
		})
	}
}

func TestGetCustomProperty(t *testing.T) {
	const propKey = "key"
	testCases := []struct {
		Name     string
		Key      string
		Props    map[string]any
		Expected any
	}{
		{
			Name:     "does_not_exist",
			Key:      "invalid",
			Props:    map[string]any{},
			Expected: false,
		},
		{
			Name:     "bool_true",
			Key:      propKey,
			Props:    map[string]any{propKey: "true"},
			Expected: true,
		},
		{
			Name:     "bool_native",
			Key:      propKey,
			Props:    map[string]any{propKey: true},
			Expected: true,
		},
		{
			Name:     "bool_type_mismatch",
			Key:      propKey,
			Props:    map[string]any{propKey: []any{"a", "b"}},
			Expected: false,
		},
		{
			Name:     "string",
			Key:      propKey,
			Props:    map[string]any{propKey: "test"},
			Expected: "test",
		},
		{
			Name:     "string_type_mismatch",
			Key:      propKey,
			Props:    map[string]any{propKey: []any{"a"}},
			Expected: "",
		},
		{
			Name:     "multi_select",
			Key:      propKey,
			Props:    map[string]any{propKey: []any{"a", "b"}},
			Expected: []string{"a", "b"},
		},
		{
			Name:     "multi_select_single_string",
			Key:      propKey,
			Props:    map[string]any{propKey: "a"},
			Expected: []string{"a"},
		},
		{
			Name:     "multi_select_missing",
			Key:      "invalid",
			Props:    map[string]any{},
			Expected: []string(nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			switch tc.Expected.(type) {
			case bool:
				assert.Equal(t, tc.Expected, helpers.GetCustomProperty[bool](tc.Props, tc.Key))
			case string:
				assert.Equal(t, tc.Expected, helpers.GetCustomProperty[string](tc.Props, tc.Key))
			case []string:
				assert.Equal(t, tc.Expected, helpers.GetCustomProperty[[]string](tc.Props, tc.Key))
			}
		})
	}
}
