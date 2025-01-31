// Package helpers provides utility functions for normalising and manipulating Git references.
package helpers

import (
	"reflect"
	"strconv"
	"strings"
)

// NormaliseRefPtr is a helper function that normalizes a Git reference and returns a pointer to the resulting string.
func NormaliseRefPtr[S string | *string](ref S) *string {
	rn := NormaliseRef(ref)
	return &rn
}

// NormaliseRef removes the "refs/heads/" prefix from a Git reference string, handling both string and *string input types.
func NormaliseRef[S string | *string](ref S) string {
	rv := reflect.ValueOf(ref)
	r := rv.String()
	if rv.Type().Kind() == reflect.Ptr {
		r = rv.Elem().String()
	}

	return strings.TrimPrefix(r, "refs/heads/")
}

// NormaliseFullRef returns a fully qualified Git reference string by prefixing "refs/heads/" to the normalized input reference.
func NormaliseFullRef[S string | *string](ref S) string {
	return "refs/heads/" + NormaliseRef(ref)
}

// NormaliseFullRefPtr returns a normalized full Git reference as a string pointer from the given string or string pointer.
func NormaliseFullRefPtr[S string | *string](ref S) *string {
	rn := NormaliseFullRef(ref)
	return &rn
}

// PropertyType is an interface that represents a property type that can be either a string or a boolean.
type PropertyType interface {
	string | bool
}

// GetCustomProperty is a helper function that retrieves a custom property from a map of string properties.
func GetCustomProperty[PT PropertyType](props map[string]string, key string) PT {
	var pt PT
	if val, ok := props[key]; ok {
		switch any(pt).(type) {
		case string:
			return any(val).(PT)
		case bool:
			bv, err := strconv.ParseBool(val)
			if err != nil {
				return any(false).(PT)
			}
			return any(bv).(PT)
		}
	}
	return pt
}
