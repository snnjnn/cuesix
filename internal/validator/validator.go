package validator

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/warpcomdev/cuesix/internal/runner"
)

type MirrorError string

func (e MirrorError) Error() string {
	return string(e)
}

const (
	ErrSourceDirRequired MirrorError = "apisix home path is required"
	ErrMirrorDirRequired MirrorError = "mirror path is required"
	ErrMissingDir        MirrorError = "apisix home directory does not exist"
	ErrSourceIsNotDir    MirrorError = "apisix home path is not a directory"
)

type Validator struct {
	runner      Runner
	mirrorDir   string
	useExisting bool
	timeout     time.Duration
	logger      *slog.Logger
}

type Runner interface {
	RunCommand(ctx context.Context, workDir string, input []byte, name string, args ...string) (stdout, stderr []byte, err error)
}

func BuildConfigPath(apisixHome string, isYAML bool) string {
	profile := strings.TrimSpace(os.Getenv("APISIX_PROFILE"))
	ext := ".json"
	if isYAML {
		ext = ".yaml"
	}
	name := "apisix" + ext
	if strings.TrimSpace(profile) != "" {
		name = fmt.Sprintf("apisix-%s%s", strings.TrimSpace(profile), ext)
	}
	return filepath.Join(apisixHome, "conf", name)
}

// New creates a validator using a mirrored APISIX home directory.
func New(logger *slog.Logger, sourceDir string, mirrorDir string, useExisting bool, timeout time.Duration, r Runner) (zero Validator, err error) {
	if logger == nil {
		logger = slog.Default()
	}
	if r == nil {
		r = runner.New()
	}
	if err := prepareMirror(sourceDir, mirrorDir, useExisting); err != nil {
		return zero, err
	}
	return Validator{
		mirrorDir:   mirrorDir,
		useExisting: useExisting,
		runner:      r,
		timeout:     timeout,
		logger:      logger,
	}, nil
}

func (v Validator) Cleanup() error {
	if v.useExisting {
		return nil
	}
	return os.RemoveAll(v.mirrorDir)
}

// MirrorDir returns the configured mirror directory path.
func (v Validator) MirrorDir() string {
	return v.mirrorDir
}

// ValidationError captures stderr and the underlying error from apisix test.
type ValidationError struct {
	Output []byte
	Cause  error
}

func (e *ValidationError) Error() string {
	if len(e.Output) == 0 && e.Cause != nil {
		return e.Cause.Error()
	}
	if e.Cause != nil {
		return fmt.Sprintf("validation failed: %s: %s", e.Cause.Error(), string(e.Output))
	}
	return fmt.Sprintf("validation failed: %s", string(e.Output))
}

func (e *ValidationError) Unwrap() error {
	return e.Cause
}

// Validate validates an APISIX configuration file.
// It returns true if the configuration is valid, false otherwise,
// and an error with output attached when validation fails.
func (v Validator) Validate(candidate []byte, isYAML bool) (bool, error) {
	if v.runner == nil {
		return false, errors.New("validator runner is nil")
	}
	if len(candidate) == 0 {
		return false, errors.New("candidate config is required")
	}

	logger := v.logger

	configPath := BuildConfigPath(v.mirrorDir, isYAML)
	logger.Info("validating temporal config file", "path", configPath)
	if err := replaceDynamicConfig(configPath, candidate); err != nil {
		return false, err
	}

	ctx := context.Background()
	if v.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, v.timeout)
		defer cancel()
	}
	_, errBytes, err := v.runner.RunCommand(ctx, v.mirrorDir, nil, "apisix", "test", "-c", configPath)

	if err != nil {
		// If command returns an error, it means apisix test failed.
		// We return false with output attached.
		return false, &ValidationError{Output: errBytes, Cause: err}
	}

	// If command returns no error, it means apisix test succeeded.
	logger.Info("validation succeeded", "path", configPath)
	return true, nil
}

func replaceDynamicConfig(target string, candidate []byte) error {
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode()
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.WriteFile(target, candidate, mode)
}

// prepareMirror copies the entire APISIX home directory into a mirror folder.
// mirrorDir must be provided by the caller.
func prepareMirror(sourceDir string, mirrorDir string, useExisting bool) error {
	if sourceDir == "" {
		return ErrSourceDirRequired
	}
	if mirrorDir == "" {
		return ErrMirrorDirRequired
	}
	if stat, err := os.Stat(sourceDir); err != nil {
		if os.IsNotExist(err) {
			return ErrMissingDir
		}
		return fmt.Errorf("failed to stat source dir: %w", err)
	} else {
		if !stat.IsDir() {
			return ErrSourceIsNotDir
		}
	}
	if stat, err := os.Stat(mirrorDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat mirror dir: %w", err)
		}
	} else if !stat.IsDir() {
		return fmt.Errorf("mirror path is not a directory: %s", mirrorDir)
	}
	if !useExisting {
		if err := clearDir(mirrorDir); err != nil {
			return fmt.Errorf("failed to clear mirror folder: %w", err)
		}
		if err := os.CopyFS(mirrorDir, os.DirFS(sourceDir)); err != nil {
			return fmt.Errorf("failed to populate mirror folder: %w", err)
		}
	}
	return nil
}

func clearDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
