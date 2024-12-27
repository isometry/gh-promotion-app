package helpers_test

import (
	"testing"

	"github.com/isometry/gh-promotion-app/internal/helpers"
	"github.com/stretchr/testify/assert"
)

func TestPtr(t *testing.T) {
	testCases := []struct {
		Name  string
		Input any
	}{
		{
			Name:  "nil",
			Input: nil,
		},
		{
			Name:  "string",
			Input: "v",
		},
		{
			Name:  "int",
			Input: 1,
		},
		{
			Name:  "bool",
			Input: true,
		},
		{
			Name:  "struct",
			Input: struct{ Name string }{Name: "v"},
		},
		{
			Name:  "slice",
			Input: []string{"v"},
		},
		{
			Name:  "map",
			Input: map[string]string{"k": "v"},
		},
		{
			Name:  "pointer",
			Input: new(string),
		},
		{
			Name:  "interface",
			Input: any(nil),
		},
		{
			Name:  "channel",
			Input: make(chan int),
		},
		{
			Name:  "array",
			Input: [1]string{"v"},
		},
		{
			Name:  "nil_pointer",
			Input: (*string)(nil),
		},
		{
			Name:  "nil_interface",
			Input: (*any)(nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Input == nil {
				assert.Nil(t, helpers.Ptr(tc.Input))
			} else {
				assert.Equal(t, &tc.Input, helpers.Ptr(tc.Input))
			}
		})
	}
}
