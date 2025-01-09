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
	return fmt.Sprintf("internal promotion error: %v", m.Cause)
}

// NewInternalErrorf creates and returns a new `InternalError` with a formatted error message as its cause.
func NewInternalErrorf(format string, args ...any) error {
	return &InternalError{Cause: errors.Errorf(format, args...)}
}

// NewInternalError creates and returns a new `InternalError` with the provided message string as cause.
func NewInternalError(message string) error {
	return &InternalError{Cause: errors.New(message)}
}
