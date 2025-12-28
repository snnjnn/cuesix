package testutil

import (
	"io"
	"log/slog"
	"sync"
)

var (
	once   sync.Once
	logger *slog.Logger
)

// Logger returns a shared test logger that only emits warnings or higher.
func Logger() *slog.Logger {
	once.Do(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))
	})
	return logger
}
