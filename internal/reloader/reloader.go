package reloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// TempFile is the temporary writable file used during atomic replacement.
type TempFile interface {
	io.WriteCloser
	Name() string
	Sync() error
}

// FileSystem exposes the filesystem operations needed by the reloader.
type FileSystem interface {
	Stat(name string) (fs.FileInfo, error)
	CreateTemp(dir, pattern string) (TempFile, error)
	Chmod(name string, mode fs.FileMode) error
	Rename(oldPath, newPath string) error
	Remove(name string) error
}

type osFS struct{}

func (osFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (osFS) CreateTemp(dir, pattern string) (TempFile, error) {
	return os.CreateTemp(dir, pattern)
}

func (osFS) Chmod(name string, mode fs.FileMode) error {
	return os.Chmod(name, mode)
}

func (osFS) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (osFS) Remove(name string) error {
	return os.Remove(name)
}

// FileReloader replaces the live config file.
type FileReloader struct {
	// Virtualgw that must match to actually save the config file
	Virtualgw string
	// ConfigPath points to the dynamic config file under the APISIX home folder.
	ConfigPath string
	// Logger for this reloader instance.
	Logger *slog.Logger
	// FS overrides the default OS-backed filesystem for testing.
	FS FileSystem
}

// Apply writes the payload to ConfigPath.
func (r *FileReloader) Apply(ctx context.Context, virtualgw string, payload []byte) error {
	if r == nil {
		return errors.New("reloader is nil")
	}
	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if virtualgw != r.Virtualgw {
		logger.Debug("Virtual gateway config will not be saved to disk", "virtualgw", virtualgw, "allowed", r.Virtualgw)
		return nil
	}
	if r.ConfigPath == "" {
		return errors.New("config path is required")
	}
	if len(payload) == 0 {
		return errors.New("payload is required")
	}

	fsys := r.FS
	if fsys == nil {
		fsys = osFS{}
	}
	if err := replaceWithPayload(fsys, payload, r.ConfigPath); err != nil {
		logger.Error("replace config failed", "error", err)
		return err
	}
	logger.Info("config file updated successfully")
	return nil
}

func replaceWithPayload(fsys FileSystem, payload []byte, destPath string) error {
	dir := filepath.Dir(destPath)
	tmp, err := fsys.CreateTemp(dir, ".sixpack-reload-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = fsys.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = fsys.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = fsys.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if info, err := fsys.Stat(destPath); err == nil {
		if err := fsys.Chmod(tmpPath, info.Mode()); err != nil {
			closeErr := fsys.Remove(tmpPath)
			if closeErr != nil {
				return fmt.Errorf("chmod temp file: %w (cleanup failed: %v)", err, closeErr)
			}
			return fmt.Errorf("chmod temp file: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		closeErr := fsys.Remove(tmpPath)
		if closeErr != nil {
			return fmt.Errorf("stat config file: %w (cleanup failed: %v)", err, closeErr)
		}
		return fmt.Errorf("stat config file: %w", err)
	}
	if err := fsys.Rename(tmpPath, destPath); err != nil {
		_ = fsys.Remove(tmpPath)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
