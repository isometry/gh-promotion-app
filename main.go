// Package main provides the entrypoint for gh-promotion-app.
package main

import (
	"fmt"
	"os"

	"github.com/isometry/gh-promotion-app/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
