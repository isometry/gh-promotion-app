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
