package promotion

import (
	"fmt"

	"github.com/pkg/errors"
)

// InternalError represents an error type with an underlying cause, used to encapsulate failures during processing.
type InternalError struct {
	Cause error
}

func (m *InternalError) Error() string {
	return fmt.Sprintf("promotion error: %v", m.Cause)
}

// NewInternalError creates and returns a new `InternalError` with a formatted error message as its cause.
func NewInternalError(format string, args ...any) error {
	return &InternalError{Cause: errors.Errorf(format, args...)}
}
