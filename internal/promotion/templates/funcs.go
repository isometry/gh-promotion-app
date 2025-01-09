// Package templates provides a set of standard functions that can be used in promotion templates.
package templates

import (
	"text/template"

	"github.com/google/go-github/v68/github"
	"github.com/isometry/gh-promotion-app/internal/helpers"
)

// StandardFuncs is a set of standard template functions that can be used in promotion templates.
var StandardFuncs = template.FuncMap{
	"sub":      func(a, b int) int { return a - b },
	"add":      func(a, b int) int { return a + b },
	"truncate": helpers.Truncate,
	"limitRepositoryCommits": func(slice []*github.RepositoryCommit, limit int) []*github.RepositoryCommit {
		if len(slice) > limit {
			return slice[:limit]
		}
		return slice
	},
}
