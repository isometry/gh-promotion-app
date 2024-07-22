package helpers

func String(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
