package validator_test

import (
	"errors"
	"io"
	"testing"

	"github.com/warpcomdev/cuesix/internal/validator"
)

// MockCommandRunner is a mock implementation of validator.CommandRunner for testing.
type MockCommandRunner struct {
	RunCommandFunc func(name string, args ...string) ([]byte, error)
}

func (m *MockCommandRunner) RunCommand(name string, args ...string) ([]byte, error) {
	if m.RunCommandFunc != nil {
		return m.RunCommandFunc(name, args...)
	}
	return nil, nil // Default empty response
}


func TestNewValidator(t *testing.T) {
	t.Parallel()
	// Test should fail if New() is called without a CommandRunner, or if nil is passed
	// This test will implicitly rely on the Validate method using the runner, or we can explicitly test that it's set.
	v := validator.New(nil) // Pass nil for now, will cause failure if used in Validate without nil check
	if v == nil {
		t.Fatal("New() should not return nil")
	}
}

func TestValidator_Validate_SyntacticallyValid(t *testing.T) {
	t.Parallel()

	mockRunner := &MockCommandRunner{
		RunCommandFunc: func(name string, args ...string) ([]byte, error) {
			// Simulate success
			return []byte("success"), nil
		},
	}

	v := validator.New(mockRunner) // Pass mock runner
	if v == nil {
		t.Fatal("New() should not return nil")
	}

	valid, err := v.Validate("/path/to/valid/config.yaml")
	
	// In the Red Phase, we expect this to fail because the dummy implementation returns false and nil error.
	// The eventual expectation is valid=true, err=nil.
	if !valid || err != nil {
		t.Errorf("Expected valid=true, err=nil for a syntactically valid config, got valid=%t, err=%v", valid, err)
	}
}

func TestValidator_Validate_SuccessOutputIgnored(t *testing.T) {
	t.Parallel()

	mockRunner := &MockCommandRunner{
		RunCommandFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("ok"), nil
		},
	}

	v := validator.New(mockRunner)
	valid, err := v.Validate("/path/to/valid/config.yaml")
	if !valid || err != nil {
		t.Fatalf("expected valid=true, err=nil, got valid=%t, err=%v", valid, err)
	}
}

func TestValidator_Validate_SyntacticallyInvalid(t *testing.T) {
	t.Parallel()

	mockRunner := &MockCommandRunner{
		RunCommandFunc: func(name string, args ...string) ([]byte, error) {
			// Simulate failure
			return []byte("error output"), io.ErrUnexpectedEOF // Example error
		},
	}

	v := validator.New(mockRunner) // Pass mock runner
	if v == nil {
		t.Fatal("New() should not return nil")
	}

	valid, err := v.Validate("/path/to/invalid/config.yaml")

	// In the Red Phase, we expect this to fail because the dummy implementation returns false and nil error.
	// The eventual expectation is valid=false, err!=nil.
	if valid || err == nil {
		t.Errorf("Expected valid=false, err!=nil for a syntactically invalid config, got valid=%t, err=%v", valid, err)
	}
	var validationErr *validator.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if string(validationErr.Output) != "error output" {
		t.Fatalf("expected output to be captured, got %q", string(validationErr.Output))
	}
	if validationErr.Unwrap() == nil {
		t.Fatalf("expected wrapped error")
	}
}

func TestValidationErrorErrorString(t *testing.T) {
	plain := &validator.ValidationError{Output: []byte("oops")}
	if plain.Error() == "" {
		t.Fatalf("expected error string")
	}
	withCause := &validator.ValidationError{Output: []byte("oops"), Cause: io.ErrUnexpectedEOF}
	if withCause.Error() == "" {
		t.Fatalf("expected error string with cause")
	}
	noOutput := &validator.ValidationError{Cause: io.ErrUnexpectedEOF}
	if noOutput.Error() == "" {
		t.Fatalf("expected error string without output")
	}
}
