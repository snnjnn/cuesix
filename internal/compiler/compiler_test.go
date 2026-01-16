package compiler_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"iter"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func snippetSeq(snips []compiler.Snippet) iter.Seq[compiler.Snippet] {
	return func(yield func(compiler.Snippet) bool) {
		for _, s := range snips {
			if !yield(s) {
				return
			}
		}
	}
}

func TestMergeRoutesWithDefaultRules(t *testing.T) {
	t.Parallel()
	left := map[string]any{
		"routes": []any{
			map[string]any{
				"id":    "1",
				"hosts": []any{"a.example.com"},
				"labels": map[string]any{
					"env": "dev",
				},
			},
		},
	}
	right := map[string]any{
		"routes": []any{
			map[string]any{
				"id":    "1",
				"hosts": []any{"b.example.com"},
				"labels": map[string]any{
					"team": "api",
				},
			},
			map[string]any{
				"uris": []any{"/new"},
			},
		},
	}
	merged, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Path: "left.yaml", Data: left},
		{Path: "right.yaml", Data: right},
	}))
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}
	routes, ok := merged["routes"].([]any)
	if !ok || len(routes) != 2 {
		t.Fatalf("expected two routes, got %v", merged["routes"])
	}
	var mergedRoute map[string]any
	for _, item := range routes {
		if m, ok := item.(map[string]any); ok && m["id"] == "1" {
			mergedRoute = m
			break
		}
	}
	if mergedRoute == nil {
		t.Fatalf("merged route not found")
	}
	hosts, ok := mergedRoute["hosts"].([]any)
	if !ok || len(hosts) != 2 {
		t.Fatalf("expected merged hosts, got %v", mergedRoute["hosts"])
	}
	if !contains(hosts, "a.example.com") || !contains(hosts, "b.example.com") {
		t.Fatalf("expected both hosts in merged result")
	}
	labels, ok := mergedRoute["labels"].(map[string]any)
	if !ok || labels["env"] != "dev" || labels["team"] != "api" {
		t.Fatalf("expected merged labels, got %v", mergedRoute["labels"])
	}
}

func TestMergeServicesRejectDuplicate(t *testing.T) {
	t.Parallel()
	left := map[string]any{
		"services": []any{
			map[string]any{"id": "svc", "name": "left"},
		},
	}
	right := map[string]any{
		"services": []any{
			map[string]any{"id": "svc", "name": "right"},
		},
	}
	_, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Path: "left.yaml", Data: left},
		{Path: "right.yaml", Data: right},
	}))
	if err == nil || !strings.Contains(err.Error(), "duplicate id svc without merge rule at /services") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestMergeGlobalRulesRequireID(t *testing.T) {
	t.Parallel()
	left := map[string]any{
		"global_rules": []any{
			map[string]any{"id": "ok", "desc": "first"},
		},
	}
	right := map[string]any{
		"global_rules": []any{
			map[string]any{"desc": "missing id"},
		},
	}
	_, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Path: "left.yaml", Data: left},
		{Path: "right.yaml", Data: right},
	}))
	if err == nil || !strings.Contains(err.Error(), "missing id attribute") {
		t.Fatalf("expected missing id error, got %v", err)
	}
}

func TestFetchReadsYAMLFiles(t *testing.T) {
	t.Parallel()
	fs1 := fstest.MapFS{
		"a.yaml": {Data: []byte("routes:\n- id: a")},
		"b.yml":  {Data: []byte("services:\n- id: b")},
		"c.txt":  {Data: []byte("ignore")},
	}
	var paths []string
	logger := testutil.Logger()
	for snippet, err := range compiler.Fetch(logger, compiler.Enumerate(logger, fs1)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		paths = append(paths, snippet.Path)
		if snippet.Data == nil {
			t.Fatalf("expected data for %s", snippet.Path)
		}
	}
	if len(paths) != 2 || paths[0] != "a.yaml" || paths[1] != "b.yml" {
		t.Fatalf("unexpected paths %v", paths)
	}
}

func TestFetchPropagatesErrors(t *testing.T) {
	t.Parallel()
	badFS := fstest.MapFS{
		"bad.yaml": {Data: []byte("{not yaml")},
	}
	var gotErr error
	logger := testutil.Logger()
	for _, err := range compiler.Fetch(logger, compiler.Enumerate(logger, badFS)) {
		gotErr = err
		break
	}
	if gotErr == nil {
		t.Fatalf("expected parse error")
	}
}

func TestApplyMergeRulesScalarConflict(t *testing.T) {
	t.Parallel()
	_, err := compiler.ApplyMergeRules("a", "b", compiler.MergingRule{Path: "/root", Kind: compiler.KindScalar})
	if err == nil || !strings.Contains(err.Error(), "scalar conflict") {
		t.Fatalf("expected scalar conflict, got %v", err)
	}
}

func TestApplyMergeRulesTypeMismatch(t *testing.T) {
	t.Parallel()
	_, err := compiler.ApplyMergeRules(map[string]any{}, []any{}, compiler.MergingRule{Path: "/root", Kind: compiler.KindMap})
	if err == nil || !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("expected type mismatch, got %v", err)
	}
}

func TestApplyMergeRulesListValidation(t *testing.T) {
	t.Parallel()
	_, err := compiler.ApplyMergeRules([]any{"a"}, []any{"b"}, compiler.MergingRule{Path: "/list", Kind: compiler.KindScalarList})
	if err != nil {
		t.Fatalf("expected scalar list merge success, got %v", err)
	}
	_, err = compiler.ApplyMergeRules([]any{map[string]any{}}, []any{}, compiler.MergingRule{Path: "/list", Kind: compiler.KindScalarList})
	if err == nil || !strings.Contains(err.Error(), "scalar list item must be a scalar") {
		t.Fatalf("expected scalar list error, got %v", err)
	}
	_, err = compiler.ApplyMergeRules([]any{"a"}, []any{1}, compiler.MergingRule{Path: "/list", Kind: compiler.KindList, IDAttr: "id", IDOptional: true})
	if err == nil {
		t.Fatalf("expected list item map error, got nil")
	}
}

func contains(values []any, target any) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
