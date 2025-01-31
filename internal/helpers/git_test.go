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
	testCases := []struct {
		Name     string
		Key      string
		Props    map[string]string
		Expected any
	}{
		{
			Name:     "does_not_exist",
			Key:      "invalid",
			Props:    map[string]string{},
			Expected: false,
		},
		{
			Name:     "bool_true",
			Key:      "key",
			Props:    map[string]string{"key": "true"},
			Expected: true,
		},
		{
			Name:     "string",
			Key:      "key",
			Props:    map[string]string{"key": "test"},
			Expected: "test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			switch tc.Expected.(type) {
			case bool:
				assert.Equal(t, tc.Expected, helpers.GetCustomProperty[bool](tc.Props, tc.Key))
			case string:
				assert.Equal(t, tc.Expected, helpers.GetCustomProperty[string](tc.Props, tc.Key))
			}
		})
	}
}
