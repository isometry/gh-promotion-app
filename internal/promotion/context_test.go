package promotion_test

import (
	"github.com/isometry/gh-promotion-app/internal/promotion"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestString(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    *string
		Expected string
	}{
		{
			Name:     "nil_string",
			Input:    nil,
			Expected: "",
		},
		{
			Name:     "empty_string",
			Input:    new(string),
			Expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, tc.Expected, promotion.String(tc.Input))
		})
	}
}

// The 'GitHub/ClientV3' is not an interface and thus cannot be mocked.

func TestContext_FindRequest(t *testing.T) {
	t.Skip("not implemented")
}

func TestContext_CreateRequest(t *testing.T) {
	t.Skip("not implemented")
}

func TestContext_FastForwardRefToSha(t *testing.T) {
	t.Skip("not implemented")
}
