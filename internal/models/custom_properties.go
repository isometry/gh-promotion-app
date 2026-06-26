package models

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// CustomProperties contains GitHub repository custom property values as raw JSON.
type CustomProperties map[string]json.RawMessage

// String returns a custom property as a string when the property value is a JSON string.
func (p CustomProperties) String(key string) (string, bool) {
	raw, found := p[key]
	if !found {
		return "", false
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", false
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

// Bool returns a custom property as a boolean when the property value is either
// a JSON boolean or a string parseable by strconv.ParseBool.
func (p CustomProperties) Bool(key string) bool {
	raw, found := p[key]
	if !found {
		return false
	}

	var value bool
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}

	stringValue, found := p.String(key)
	if !found {
		return false
	}
	parsedValue, err := strconv.ParseBool(stringValue)
	return err == nil && parsedValue
}
