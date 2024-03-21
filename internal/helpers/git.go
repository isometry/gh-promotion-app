package helpers

import (
	"fmt"
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

	return fmt.Sprintf("refs/heads/%s", strings.TrimPrefix(r, "refs/heads/"))
}
