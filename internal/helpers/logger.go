package helpers

import (
	"io"
	"log/slog"
)

func NewNoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
