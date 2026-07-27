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
	if rv.Type().Kind() == reflect.Pointer {
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

// PropertyType is an interface that represents the supported repository custom
// property value shapes: a single value (string), a true_false flag (bool), or a
// multi_select list ([]string).
type PropertyType interface {
	string | bool | []string
}

// GetCustomProperty retrieves a repository custom property and coerces it to the
// requested type. Custom properties arrive from GitHub webhooks decoded into
// map[string]any, where single/single_select/true_false values are strings and
// multi_select values are JSON arrays (decoded as []any). Missing keys or values
// that cannot be coerced yield the zero value of the requested type.
func GetCustomProperty[PT PropertyType](props map[string]any, key string) PT {
	var pt PT
	val, ok := props[key]
	if !ok {
		return pt
	}
	switch any(pt).(type) {
	case string:
		if s, ok := val.(string); ok {
			return any(s).(PT)
		}
	case bool:
		switch v := val.(type) {
		case bool:
			return any(v).(PT)
		case string:
			if bv, err := strconv.ParseBool(v); err == nil {
				return any(bv).(PT)
			}
		}
	case []string:
		return any(toStringSlice(val)).(PT)
	}
	return pt
}

// toStringSlice coerces a custom property value into a []string, accepting a
// multi_select array (decoded as []any or already []string) or a lone string,
// which is treated as a single-element list. Non-string elements are skipped.
func toStringSlice(val any) []string {
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{v}
	default:
		return nil
	}
}
