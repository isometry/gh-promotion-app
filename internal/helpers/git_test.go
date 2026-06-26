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
