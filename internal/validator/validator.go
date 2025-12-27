package validator

import "fmt"

// CommandRunner defines an interface for running external commands.
type CommandRunner interface {
	RunCommand(name string, args ...string) ([]byte, error)
}

// Validator defines the interface for validating APISIX configurations.
type Validator interface {
	Validate(configPath string) (bool, error)
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
func (v *validator) Validate(configPath string) (bool, error) {
	outputBytes, err := v.runner.RunCommand("apisix", "test", "-c", configPath)

	if err != nil {
		// If command returns an error, it means apisix test failed.
		// We return false with output attached.
		return false, &ValidationError{Output: outputBytes, Cause: err}
	}

	// If command returns no error, it means apisix test succeeded.
	return true, nil
}
