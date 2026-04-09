package testutil

import "context"

type MockInput struct {
	WorkDir string
	Input   []byte
	Cmd     string
	Args    []string
}

type MockOutput struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

type MockRunner struct {
	Inputs   []MockInput
	position int
	output   []MockOutput
}

// RunCommand records command input and replays the next mocked output.
func (m *MockRunner) RunCommand(_ context.Context, workDir string, input []byte, cmd string, args ...string) ([]byte, []byte, error) {
	m.Inputs = append(m.Inputs, MockInput{
		WorkDir: workDir,
		Input:   input,
		Cmd:     cmd,
		Args:    args,
	})
	position := m.position
	if position >= len(m.output) {
		return nil, nil, nil
	}
	m.position += 1
	output := m.output[position]
	return output.Stdout, output.Stderr, output.Err
}

// NewMock builds a mock runner with queued command outputs.
func NewMock(output ...MockOutput) *MockRunner {
	return &MockRunner{
		output: output,
	}
}
