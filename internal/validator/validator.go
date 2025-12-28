package validator

import (
	"errors"
	"fmt"
	"io/fs"
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
	Validate(candidate []byte, isYAML bool) (bool, error)
}

// New creates a validator using a mirrored APISIX home directory.
func New(sourceDir string, mirrorDir string) (Validator, error) {
	return newWithRunner(sourceDir, mirrorDir, systemCommandRunner{})
}

func newWithRunner(sourceDir string, mirrorDir string, runner commandRunner) (Validator, error) {
	if runner == nil {
		runner = systemCommandRunner{}
	}
	mirrorDir, err := prepareMirror(sourceDir, mirrorDir)
	if err != nil {
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
func (v *mirrorValidator) Validate(candidate []byte, isYAML bool) (bool, error) {
	if len(candidate) == 0 {
		return false, errors.New("candidate config is required")
	}
	profile := strings.TrimSpace(os.Getenv("APISIX_PROFILE"))
	ext := ".json"
	if isYAML {
		ext = ".yaml"
	}

	if err := replaceDynamicConfig(v.mirrorDir, profile, ext, candidate); err != nil {
		return false, err
	}

	configPath := filepath.Join(v.mirrorDir, "conf", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		return false, fmt.Errorf("config.yaml not found: %w", err)
	}

	outputBytes, err := v.runner.RunCommand(v.mirrorDir, "apisix", "test", "-c", configPath)

	if err != nil {
		// If command returns an error, it means apisix test failed.
		// We return false with output attached.
		return false, &ValidationError{Output: outputBytes, Cause: err}
	}

	// If command returns no error, it means apisix test succeeded.
	return true, nil
}

func replaceDynamicConfig(mirrorDir string, profile string, ext string, candidate []byte) error {
	target := ConfigPath(mirrorDir, profile, ext == ".yaml")
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode()
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.WriteFile(target, candidate, mode)
}

// ConfigPath builds the APISIX dynamic config path for a home directory.
// If profile is empty, it uses apisix.{ext}.
func ConfigPath(homeDir string, profile string, isYAML bool) string {
	ext := ".json"
	if isYAML {
		ext = ".yaml"
	}
	name := "apisix" + ext
	if strings.TrimSpace(profile) != "" {
		name = fmt.Sprintf("apisix-%s%s", strings.TrimSpace(profile), ext)
	}
	return filepath.Join(homeDir, "conf", name)
}

type systemCommandRunner struct{}

func (systemCommandRunner) RunCommand(workDir string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = workDir
	return cmd.CombinedOutput()
}

// prepareMirror copies the entire APISIX home directory into a mirror folder.
// mirrorDir must be provided by the caller.
func prepareMirror(sourceDir string, mirrorDir string) (string, error) {
	if sourceDir == "" {
		return "", ErrSourceDirRequired
	}
	if mirrorDir == "" {
		return "", ErrMirrorDirRequired
	}
	if stat, err := os.Stat(sourceDir); err != nil {
		if os.IsNotExist(err) {
			return "", ErrMissingDir
		}
		return "", fmt.Errorf("failed to stat source dir: %w", err)
	} else {
		if !stat.IsDir() {
			return "", ErrSourceIsNotDir
		}
	}
	if err := os.RemoveAll(mirrorDir); err != nil {
		// RemoveAll would not error if the folder does not exist.
		// If it complains, it is because there is an actual error
		// while trying to remove an existing folder, so we should
		// not ignore it.
		return "", fmt.Errorf("failed to remove mirror folder: %w", err)
	}
	if err := os.MkdirAll(mirrorDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create mirror folder: %w", err)
	}
	if err := os.CopyFS(mirrorDir, os.DirFS(sourceDir)); err != nil {
		return "", fmt.Errorf("failed to populate mirror folder: %w", err)
	}
	return mirrorDir, nil
}
