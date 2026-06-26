package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomPropertiesString(t *testing.T) {
	props := CustomProperties{
		"string": json.RawMessage(`"dev,stage,prod"`),
		"array":  json.RawMessage(`["one", "two"]`),
		"null":   json.RawMessage(`null`),
	}

	value, found := props.String("string")
	require.True(t, found)
	assert.Equal(t, "dev,stage,prod", value)

	_, found = props.String("array")
	assert.False(t, found)
	_, found = props.String("null")
	assert.False(t, found)
	_, found = props.String("missing")
	assert.False(t, found)
}

func TestCustomPropertiesBool(t *testing.T) {
	props := CustomProperties{
		"bool_true":    json.RawMessage(`true`),
		"string_true":  json.RawMessage(`"true"`),
		"string_false": json.RawMessage(`"false"`),
		"invalid":      json.RawMessage(`"not-bool"`),
		"array":        json.RawMessage(`["true"]`),
	}

	assert.True(t, props.Bool("bool_true"))
	assert.True(t, props.Bool("string_true"))
	assert.False(t, props.Bool("string_false"))
	assert.False(t, props.Bool("invalid"))
	assert.False(t, props.Bool("array"))
	assert.False(t, props.Bool("missing"))
}
