package plugin_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/warpcomdev/sixpack/internal/plugin"
	"github.com/warpcomdev/sixpack/internal/testutil"
)

func TestJQPluginUpdate(t *testing.T) {
	t.Parallel()
	root := map[string]any{
		"jq": []map[string]any{
			{"id": "t1", "prio": 1, "expr": ".foo = \"bar\""},
		},
		"foo": "baz",
	}
	payload, _ := json.Marshal(root)
	mock := testutil.NewMock(testutil.MockOutput{
		Stdout: []byte(`{"foo":"bar"}`),
	})
	p := &plugin.JQPlugin{Runner: mock, Timeout: time.Second}
	out, err := p.Update(testutil.Logger(), payload)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if string(out) != `{"foo":"bar"}` {
		t.Fatalf("unexpected output %s", out)
	}
	if len(mock.Inputs) != 1 {
		t.Fatalf("expected runner called once")
	}
	call := mock.Inputs[0]
	if call.Cmd != "jq" || len(call.Args) != 1 {
		t.Fatalf("unexpected command %+v", call)
	}
	if len(mock.Inputs[0].Input) == 0 {
		t.Fatalf("expected payload passed to runner")
	}
}

func TestJQPluginUpdateEdgeCases(t *testing.T) {
	t.Parallel()
	p := &plugin.JQPlugin{}
	if out, err := p.Update(testutil.Logger(), nil); err != nil || len(out) != 0 {
		t.Fatalf("expected passthrough for empty payload")
	}
	if out, err := p.Update(testutil.Logger(), []byte("1")); err != nil || string(out) != "1" {
		t.Fatalf("expected passthrough for non-object")
	}
	payload, _ := json.Marshal(map[string]any{"jq": []map[string]any{}, "foo": "bar"})
	if out, err := p.Update(testutil.Logger(), payload); err != nil || len(out) == 0 {
		t.Fatalf("expected payload when no transforms")
	}
	badJSON := []byte("{")
	if _, err := p.Update(testutil.Logger(), badJSON); err == nil {
		t.Fatalf("expected decode error")
	}
	badConfig, _ := json.Marshal(map[string]any{"jq": []map[string]any{{"expr": ""}}})
	if _, err := p.Update(testutil.Logger(), badConfig); err == nil {
		t.Fatalf("expected config error for empty expr")
	}
}

func TestJQPluginRunnerError(t *testing.T) {
	t.Parallel()
	root := map[string]any{
		"jq": []map[string]any{{"id": "t1", "expr": ".a"}},
		"a":  "b",
	}
	payload, _ := json.Marshal(root)
	mock := testutil.NewMock(testutil.MockOutput{
		Err: errors.New("boom"),
	})
	p := &plugin.JQPlugin{Runner: mock}
	if _, err := p.Update(testutil.Logger(), payload); err == nil {
		t.Fatalf("expected exec error")
	}

	mockErrOut := testutil.NewMock(testutil.MockOutput{
		Stdout: []byte(""),
		Stderr: []byte("stderr"),
	})
	p.Runner = mockErrOut
	if _, err := p.Update(testutil.Logger(), payload); err == nil {
		t.Fatalf("expected exec error for stderr output")
	}
}

func TestJQPluginTimeout(t *testing.T) {
	t.Parallel()
	root := map[string]any{
		"jq": []map[string]any{{"id": "t1", "expr": ".a"}},
		"a":  "b",
	}
	payload, _ := json.Marshal(root)
	mock := testutil.NewMock(testutil.MockOutput{
		Stdout: []byte(`{"a":"b"}`),
	})
	p := &plugin.JQPlugin{Runner: mock, Timeout: time.Millisecond}
	_, err := p.Update(testutil.Logger(), payload)
	if err != nil {
		t.Fatalf("unexpected error with timeout: %v", err)
	}
	if mock.Inputs[0].Cmd != "jq" {
		t.Fatalf("expected jq command, got %s", mock.Inputs[0].Cmd)
	}
}
