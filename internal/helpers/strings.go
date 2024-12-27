package helpers

// String returns the dereferenced value of the input pointer if it's not nil, otherwise, it returns an empty string.
func String(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Truncate shortens the given string to the specified length, appending "..." if truncation occurs.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
