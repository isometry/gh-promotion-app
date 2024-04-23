package helpers

func Ptr[T any](v T) *T {
	if any(v) == nil {
		return nil
	}
	return &v
}
