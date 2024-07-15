package helpers

import (
	"reflect"
	"strings"
)

func NormaliseRefPtr[S string | *string](ref S) *string {
	rn := NormaliseRef(ref)
	return &rn
}

func NormaliseRef[S string | *string](ref S) string {
	rv := reflect.ValueOf(ref)
	r := rv.String()
	if rv.Type().Kind() == reflect.Ptr {
		r = rv.Elem().String()
	}

	return strings.TrimPrefix(r, "refs/heads/")
}

func NormaliseFullRef[S string | *string](ref S) string {
	return "refs/heads/" + NormaliseRef(ref)
}

func NormaliseFullRefPtr[S string | *string](ref S) *string {
	rn := NormaliseFullRef(ref)
	return &rn
}

func ExtractRefFromFullRef(fullRef string) string {
	return strings.TrimPrefix(fullRef, "refs/heads/")
}
