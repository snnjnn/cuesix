package plugin

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/testutil"
)

type mockJQRunner struct {
	inputs      [][]byte
	expressions []string
	stdout      []byte
	stderr      []byte
	err         error
}

func (m *mockJQRunner) Run(_ context.Context, input []byte, expression string) ([]byte, []byte, error) {
	m.inputs = append(m.inputs, input)
	m.expressions = append(m.expressions, expression)
	return m.stdout, m.stderr, m.err
}

type deadlineJQRunner struct{}

func (deadlineJQRunner) Run(ctx context.Context, _ []byte, _ string) ([]byte, []byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, nil, errors.New("missing deadline")
	}
	return []byte(`{"ok":true}`), nil, nil
}

func TestJQPluginNoConfig(t *testing.T) {
	plugin := &JQPlugin{}
	input := []byte(`{"routes":[{"id":1}]}`)
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if string(got) != string(input) {
		t.Fatalf("expected output unchanged, got %q", got)
	}
}

func TestJQPluginNoConfigSkipsRun(t *testing.T) {
	runner := &mockJQRunner{}
	plugin := &JQPlugin{Runner: runner}
	input := []byte(`{"routes":[{"id":1}]}`)
	_, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if len(runner.inputs) != 0 {
		t.Fatalf("expected 0 runner calls, got %d", len(runner.inputs))
	}
}

func TestJQPluginRemovesJQEntry(t *testing.T) {
	runner := &mockJQRunner{}
	plugin := &JQPlugin{Runner: runner}
	input := []byte(`{"routes":[{"id":1}],"jq":[{"expr":".routes[0].id = 2"}]}`)
	runner.stdout = []byte(`{"routes":[{"id":2}]}`)
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if strings.Contains(string(got), `"jq"`) {
		t.Fatalf("expected jq entry removed, got %q", got)
	}
	if len(runner.inputs) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(runner.inputs))
	}
	if strings.Contains(string(runner.inputs[0]), `"jq"`) {
		t.Fatalf("expected jq removed from runner input, got %q", string(runner.inputs[0]))
	}
}

func TestJQPluginAppliesTransformsInPrioOrder(t *testing.T) {
	runner := &mockJQRunner{stdout: []byte(`{"routes":[{"id":20}]}`)}
	plugin := &JQPlugin{Runner: runner}
	input := []byte(`{"routes":[{"id":1}],"jq":[{"id":"first","prio":10,"expr":".routes[0].id = 10"},{"id":"second","prio":20,"expr":".routes[0].id = 20"}]}`)
	_, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if len(runner.expressions) != 1 {
		t.Fatalf("expected 1 expression, got %d", len(runner.expressions))
	}
	if runner.expressions[0] != "(.routes[0].id = 20) | (.routes[0].id = 10)" {
		t.Fatalf("unexpected pipeline: %q", runner.expressions[0])
	}
}

func TestJQPluginRejectsNonListConfig(t *testing.T) {
	plugin := &JQPlugin{}
	input := []byte(`{"jq":{"expr":".a = 1"}}`)
	_, err := plugin.Update(testutil.Logger(), input)
	if err == nil {
		t.Fatalf("expected error for non-list jq config")
	}
	var cfgErr *JQConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected JQConfigError, got %T", err)
	}
	if cfgErr.Field != "jq" {
		t.Fatalf("unexpected field: %q", cfgErr.Field)
	}
}

func TestJQPluginRequiresExpression(t *testing.T) {
	plugin := &JQPlugin{}
	input := []byte(`{"jq":[{"id":"missing"}]}`)
	_, err := plugin.Update(testutil.Logger(), input)
	if err == nil {
		t.Fatalf("expected error for missing expr")
	}
	var cfgErr *JQConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected JQConfigError, got %T", err)
	}
	if cfgErr.Field != "jq[0].expr" {
		t.Fatalf("unexpected field: %q", cfgErr.Field)
	}
}

func TestJQPluginRejectsUnknownField(t *testing.T) {
	plugin := &JQPlugin{}
	input := []byte(`{"jq":[{"expr":".a = 1","extra":true}]}`)
	_, err := plugin.Update(testutil.Logger(), input)
	if err == nil {
		t.Fatalf("expected error for unknown field")
	}
	var cfgErr *JQConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected JQConfigError, got %T", err)
	}
	if cfgErr.Field != "jq" {
		t.Fatalf("unexpected field: %q", cfgErr.Field)
	}
}

func TestJQPluginRejectsInvalidPrio(t *testing.T) {
	plugin := &JQPlugin{}
	input := []byte(`{"jq":[{"expr":".a = 1","prio":"high"}]}`)
	_, err := plugin.Update(testutil.Logger(), input)
	if err == nil {
		t.Fatalf("expected error for invalid prio")
	}
	var cfgErr *JQConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected JQConfigError, got %T", err)
	}
	if cfgErr.Field != "jq" {
		t.Fatalf("unexpected field: %q", cfgErr.Field)
	}
}

func TestJQPluginDecodeError(t *testing.T) {
	plugin := &JQPlugin{}
	_, err := plugin.Update(testutil.Logger(), []byte("{"))
	if err == nil {
		t.Fatalf("expected error")
	}
	var decodeErr *JQDecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected JQDecodeError, got %T", err)
	}
}

func TestJQPluginExecError(t *testing.T) {
	runner := &mockJQRunner{stderr: []byte("failed")}
	plugin := &JQPlugin{Runner: runner}
	input := []byte(`{"jq":[{"expr":".a = 1"}]}`)
	_, err := plugin.Update(testutil.Logger(), input)
	if err == nil {
		t.Fatalf("expected error")
	}
	var execErr *JQExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected JQExecError, got %T", err)
	}
}

func TestJQPluginEmptyListRemovesJQ(t *testing.T) {
	plugin := &JQPlugin{}
	input := []byte(`{"jq":[],"a":1}`)
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if strings.Contains(string(got), `"jq"`) {
		t.Fatalf("expected jq entry removed, got %q", got)
	}
}

func TestJQPluginNonObjectRootSkips(t *testing.T) {
	plugin := &JQPlugin{}
	input := []byte(`[{"jq":[{"expr":".a = 1"}]}]`)
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if string(got) != string(input) {
		t.Fatalf("expected output unchanged, got %q", got)
	}
}

func TestSystemJQRunner(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available in PATH")
	}
	runner := systemJQRunner{}
	stdout, stderr, err := runner.Run(context.Background(), []byte(`{"a":1}`), ".a")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(stderr) != 0 {
		t.Fatalf("expected empty stderr, got %q", string(stderr))
	}
	if strings.TrimSpace(string(stdout)) != "1" {
		t.Fatalf("unexpected stdout: %q", string(stdout))
	}
}

func TestJQPluginTimeoutPassesContext(t *testing.T) {
	plugin := &JQPlugin{
		Runner:  deadlineJQRunner{},
		Timeout: 10 * time.Millisecond,
	}
	input := []byte(`{"jq":[{"expr":".a = 1"}]}`)
	_, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
}
