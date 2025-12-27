package validator

import (
	"bytes"
	"os/exec"
)

// systemCommandRunner implements CommandRunner by executing system commands.
type systemCommandRunner struct{}

// NewSystemCommandRunner creates a new SystemCommandRunner.
func NewSystemCommandRunner() CommandRunner {
	return &systemCommandRunner{}
}

// RunCommand executes a system command and returns its combined stdout and stderr.
func (r *systemCommandRunner) RunCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer // Capture stderr for error messages
	cmd.Stderr = &stderr

	// Stdin/Stdout are not explicitly handled here, assuming apisix test works with files.
	// We only care about stderr for error messages and exit code.
	
	err := cmd.Run() // cmd.Run() waits for command to complete and returns error if non-zero exit.
	
	if err != nil {
		// Combine stderr output with the error
		return stderr.Bytes(), err
	}
	return nil, nil // If successful, no error output from stderr is expected for apisix test.
}
