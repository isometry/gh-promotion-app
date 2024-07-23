package promotion

import (
	"fmt"

	"github.com/pkg/errors"
)

type InternalError struct {
	Cause error
}

func (m *InternalError) Error() string {
	return fmt.Sprintf("promotion error: %v", m.Cause)
}

func NewInternalError(format string, args ...any) error {
	return &InternalError{Cause: errors.Errorf(format, args...)}
}
