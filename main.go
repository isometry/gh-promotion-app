// Package main provides the entrypoint for gh-promotion-app.
package main

import (
	"os"

	"github.com/isometry/gh-promotion-app/cmd"
)

func main() {
	if err := cmd.New().Execute(); err != nil {
		os.Exit(1)
	}
}
