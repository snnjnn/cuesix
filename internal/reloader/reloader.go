package reloader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Reloader replaces the live config file.
type Reloader struct {
	// ConfigPath points to the dynamic config file under the APISIX home folder.
	ConfigPath string
	// Logger for this reloader instance.
	Logger *slog.Logger
}

// Apply writes the payload to ConfigPath.
func (r *Reloader) Apply(ctx context.Context, payload []byte, useApi bool) error {
	if r == nil {
		return errors.New("reloader is nil")
	}
	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if r.ConfigPath == "" {
		return errors.New("config path is required")
	}
	if len(payload) == 0 {
		return errors.New("payload is required")
	}

	if err := replaceWithPayload(payload, r.ConfigPath); err != nil {
		logger.Error("replace config failed", "error", err)
		return err
	}
	logger.Info("config file updated successfully")
	return nil
}



func replaceWithPayload(payload []byte, destPath string) error {
	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, ".sixpack-reload-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if info, err := os.Stat(destPath); err == nil {
		if err := os.Chmod(tmpPath, info.Mode()); err != nil {
			closeErr := os.Remove(tmpPath)
			if closeErr != nil {
				return fmt.Errorf("chmod temp file: %w (cleanup failed: %v)", err, closeErr)
			}
			return fmt.Errorf("chmod temp file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		closeErr := os.Remove(tmpPath)
		if closeErr != nil {
			return fmt.Errorf("stat config file: %w (cleanup failed: %v)", err, closeErr)
		}
		return fmt.Errorf("stat config file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
