// Package templates provides a set of standard functions that can be used in GitHub-related request templates.
package templates

import (
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// StandardFuncs is a map of standard functions that can be used in GitHub-related request templates.
var StandardFuncs = template.FuncMap{
	"cutHeadLines": func(n int, v string) string {
		tmp := strings.Split(v, "\n")
		if n == -1 || n > len(tmp) {
			return v
		}
		return strings.Join(tmp[n:], "\n")
	},
	"color": func(color, v string) string {
		tmp := strings.Split(v, "\n")
		for i, line := range tmp {
			// @Note: monospace is not supported on GitHub-flavored markdown
			tmp[i] = fmt.Sprintf(`${{\large\color{%s}{\texttt{ %s \}}}}\$`, color, line)
		}
		return strings.Join(tmp, "\n")
	},
	"toYaml": func(v any) string {
		b, _ := yaml.Marshal(v)
		return string(b)
	},
	"replace": func(old, new, s string) string { //nolint:revive // false positive
		return strings.ReplaceAll(s, old, new)
	},
	"substr": func(s string, start, end int) string {
		if end == -1 {
			return s[start:]
		}
		return s[start:end]
	},
	"addLinesPrefix": func(prefix, value string) string {
		return prefix + strings.Join(strings.Split(value, "\n"), "\n"+prefix)
	},
}
