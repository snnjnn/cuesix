package validator

import (
	"bytes"
	"io"
)

// CommandRunner defines an interface for running external commands.
type CommandRunner interface {
	RunCommand(name string, args ...string) ([]byte, error)
}

// Validator defines the interface for validating APISIX configurations.
type Validator interface {
	Validate(configPath string) (bool, io.ReadCloser, error)
}

// New creates a new Validator.
func New(runner CommandRunner) Validator {
	if runner == nil {
		runner = NewSystemCommandRunner() // Use systemCommandRunner if none provided
	}
	return &validator{runner: runner}
}

type validator struct {
	runner CommandRunner
}

// Validate validates an APISIX configuration file.
// It returns true if the configuration is valid, false otherwise,
// an io.ReadCloser for any error output (e.g., stderr from apisix test), and an error.
func (v *validator) Validate(configPath string) (bool, io.ReadCloser, error) {
	outputBytes, err := v.runner.RunCommand("apisix", "test", "-c", configPath)

	if err != nil {
		// If command returns an error, it means apisix test failed.
		// We return false, and the error output.
		return false, io.NopCloser(bytes.NewReader(outputBytes)), err
	}

	// If command returns no error, it means apisix test succeeded.
	return true, io.NopCloser(bytes.NewReader(outputBytes)), nil
}