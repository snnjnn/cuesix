package runner_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/runner"
)

func TestRunCommandWithWorkdirAndInput(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	sys := runner.New()
	stdout, stderr, err := sys.RunCommand(context.Background(), tmp, []byte("hello"), "sh", "-c", "cat; printf '\\n'; pwd")
	if err != nil {
		t.Fatalf("RunCommand error: %v", err)
	}
	if len(stderr) != 0 {
		t.Fatalf("expected empty stderr, got %s", stderr)
	}
	parts := bytes.SplitN(stdout, []byte("\n"), 3)
	if string(bytes.TrimSpace(parts[0])) != "hello" {
		t.Fatalf("expected input echoed, got %q", parts[0])
	}
	if wd := strings.TrimSpace(string(parts[1])); wd != tmp {
		t.Fatalf("expected workdir %s, got %s", tmp, wd)
	}
}

func TestRunCommandContextCancel(t *testing.T) {
	t.Parallel()
	sys := runner.New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, _, err := sys.RunCommand(ctx, "", nil, "sleep", "1")
	if err == nil {
		t.Fatalf("expected command to be killed on timeout")
	}
}
