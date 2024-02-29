package helpers

import (
	"fmt"
	"strings"
)

func StandardRef(i *string) *string {
	o := fmt.Sprintf("refs/heads/%s", strings.TrimPrefix(*i, "refs/heads/"))
	return &o
}
