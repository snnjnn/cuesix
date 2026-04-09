package runner

import (
	"bytes"
	"context"
	"os/exec"
)

type System struct{}

// RunCommand executes an external command with optional working dir and stdin.
func (System) RunCommand(ctx context.Context, workDir string, input []byte, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	if len(input) > 0 {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// New returns the default system command runner.
func New() System {
	return System{}
}
