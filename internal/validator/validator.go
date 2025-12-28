package validator

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type commandRunner interface {
	RunCommand(workDir string, name string, args ...string) ([]byte, error)
}

type mirrorError string

func (e mirrorError) Error() string {
	return string(e)
}

const (
	ErrSourceDirRequired mirrorError = "apisix home path is required"
	ErrMirrorDirRequired mirrorError = "mirror path is required"
	ErrMissingDir        mirrorError = "apisix home directory does not exist"
	ErrSourceIsNotDir    mirrorError = "apisix home path is not a directory"
)

// Validator validates APISIX dynamic configuration payloads.
type Validator interface {
	Validate(logger *slog.Logger, candidate []byte, isYAML bool) (bool, error)
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
func New(sourceDir string, mirrorDir string, useExisting bool) (Validator, error) {
	return newWithRunner(sourceDir, mirrorDir, useExisting, systemCommandRunner{})
}

func newWithRunner(sourceDir string, mirrorDir string, useExisting bool, runner commandRunner) (Validator, error) {
	if runner == nil {
		runner = systemCommandRunner{}
	}
	if err := prepareMirror(sourceDir, mirrorDir, useExisting); err != nil {
		return nil, err
	}
	return &mirrorValidator{mirrorDir: mirrorDir, runner: runner}, nil
}

type mirrorValidator struct {
	runner    commandRunner
	mirrorDir string
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
func (v *mirrorValidator) Validate(logger *slog.Logger, candidate []byte, isYAML bool) (bool, error) {
	if len(candidate) == 0 {
		return false, errors.New("candidate config is required")
	}

	configPath := BuildConfigPath(v.mirrorDir, isYAML)
	logger.Info("validating temporal config file", "path", configPath)
	if err := replaceDynamicConfig(configPath, candidate); err != nil {
		return false, err
	}

	outputBytes, err := v.runner.RunCommand(v.mirrorDir, "apisix", "test", "-c", configPath)

	if err != nil {
		// If command returns an error, it means apisix test failed.
		// We return false with output attached.
		return false, &ValidationError{Output: outputBytes, Cause: err}
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

type systemCommandRunner struct{}

func (systemCommandRunner) RunCommand(workDir string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = workDir
	return cmd.CombinedOutput()
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
