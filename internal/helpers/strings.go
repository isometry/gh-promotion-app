package helpers

func String(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
