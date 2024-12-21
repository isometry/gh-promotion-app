package helpers

import (
	"io"
	"log/slog"
)

// NewNoopLogger creates and returns a no-operation logger that discards all log output.
func NewNoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
