package helpers

// Ptr returns a pointer to the value passed as an argument. If the value is nil, it returns a nil pointer.
func Ptr[T any](v T) *T {
	if any(v) == nil {
		return nil
	}
	return &v
}
