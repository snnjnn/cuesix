package validator

import (
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
func New(runner CommandRunner) Validator { // Modified to accept CommandRunner
	return &validator{runner: runner}
}

type validator struct {
	runner CommandRunner
}

// Validate validates an APISIX configuration file.
// It returns true if the configuration is valid, false otherwise,
// an io.ReadCloser for any error output (e.g., stderr from apisix test), and an error.
func (v *validator) Validate(configPath string) (bool, io.ReadCloser, error) {
	// TODO: Implement actual validation logic using apisix test
	return false, nil, nil
}