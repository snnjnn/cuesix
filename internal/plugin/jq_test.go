package plugin

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestParseJQTransforms(t *testing.T) {
	t.Parallel()
	root := map[string]any{"jq": []map[string]any{
		{"id": "t1", "prio": 1, "expr": ".a"},
	}}
	transforms, has, err := parseJQTransforms(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has || len(transforms) != 1 || transforms[0].Expr != ".a" {
		t.Fatalf("unexpected result: %#v has=%v", transforms, has)
	}
	if _, ok := root["jq"]; ok {
		t.Fatalf("expected jq removed from root")
	}
	_, has, err = parseJQTransforms(map[string]any{})
	if err != nil || has {
		t.Fatalf("expected no jq transforms, got has=%v err=%v", has, err)
	}
	if _, _, err := parseJQTransforms(map[string]any{"jq": "bad"}); err == nil {
		t.Fatalf("expected config error for invalid content")
	}
	if _, _, err := parseJQTransforms(map[string]any{"jq": []map[string]any{{"expr": ""}}}); err == nil {
		t.Fatalf("expected config error for missing expr")
	}
}

func TestBuildJQPipeline(t *testing.T) {
	t.Parallel()
	transforms := []JQTransform{
		{ID: "a", Prio: 1, Expr: ".a"},
		{ID: "b", Prio: 2, Expr: ".b"},
	}
	p := buildJQPipeline(testutil.Logger(), transforms)
	if p != "(.a) | (.b)" {
		t.Fatalf("unexpected pipeline %q", p)
	}
}

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
	plugin := &JQPlugin{Runner: mock, Timeout: time.Second}
	out, err := plugin.Update(testutil.Logger(), payload)
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
	plugin := &JQPlugin{}
	if out, err := plugin.Update(testutil.Logger(), nil); err != nil || len(out) != 0 {
		t.Fatalf("expected passthrough for empty payload")
	}
	if out, err := plugin.Update(testutil.Logger(), []byte("1")); err != nil || string(out) != "1" {
		t.Fatalf("expected passthrough for non-object")
	}
	payload, _ := json.Marshal(map[string]any{"jq": []map[string]any{}, "foo": "bar"})
	if out, err := plugin.Update(testutil.Logger(), payload); err != nil || len(out) == 0 {
		t.Fatalf("expected payload when no transforms")
	}
	badJSON := []byte("{")
	if _, err := plugin.Update(testutil.Logger(), badJSON); err == nil {
		t.Fatalf("expected decode error")
	}
	badConfig, _ := json.Marshal(map[string]any{"jq": []map[string]any{{"expr": ""}}})
	if _, err := plugin.Update(testutil.Logger(), badConfig); err == nil {
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
	plugin := &JQPlugin{Runner: mock}
	if _, err := plugin.Update(testutil.Logger(), payload); err == nil {
		t.Fatalf("expected exec error")
	}

	mockErrOut := testutil.NewMock(testutil.MockOutput{
		Stdout: []byte(""),
		Stderr: []byte("stderr"),
	})
	plugin.Runner = mockErrOut
	if _, err := plugin.Update(testutil.Logger(), payload); err == nil {
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
	plugin := &JQPlugin{Runner: mock, Timeout: time.Millisecond}
	_, err := plugin.Update(testutil.Logger(), payload)
	if err != nil {
		t.Fatalf("unexpected error with timeout: %v", err)
	}
	if mock.Inputs[0].Cmd != "jq" {
		t.Fatalf("expected jq command, got %s", mock.Inputs[0].Cmd)
	}
}
