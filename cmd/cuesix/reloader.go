package main

import (
	"context"
	"log/slog"
)

// dryRunReloader is a no-op reloader used for dry-run mode.
type dryRunReloader struct{}

// Apply logs the payload size without making changes.
func (r *dryRunReloader) Apply(_ context.Context, logger *slog.Logger, payload []byte, useApi bool) error {
	logger.Info("dry-run reload skipped", "bytes", len(payload))
	return nil
}
