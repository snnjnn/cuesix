package compiler_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/testutil"
	"go.yaml.in/yaml/v4"
)

func TestDefaultMergingRulesFromFiles(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir testdata: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		entry := entry
		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, entry.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var spec struct {
				Name     string         `yaml:"name"`
				Left     map[string]any `yaml:"left"`
				Right    map[string]any `yaml:"right"`
				Expected map[string]any `yaml:"expected"`
				Error    string         `yaml:"error"`
			}
			if err := yaml.Unmarshal(content, &spec); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			if spec.Left == nil || spec.Right == nil {
				t.Fatalf("left or right missing in %s", path)
			}
			left := normalizeOrFatal(t, spec.Left)
			right := normalizeOrFatal(t, spec.Right)
			expected := normalizeOrFatal(t, spec.Expected)
			merged, mergeErr := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
				{Path: "left.yaml", Data: left},
				{Path: "right.yaml", Data: right},
			}))
			if spec.Error != "" {
				if mergeErr == nil || !strings.Contains(mergeErr.Error(), spec.Error) {
					t.Fatalf("expected error containing %q, got %v merged=%v", spec.Error, mergeErr, merged)
				}
				return
			}
			if mergeErr != nil {
				t.Fatalf("Merge returned error: %v", mergeErr)
			}
			if diff := cmp.Diff(expected, merged); diff != "" {
				t.Fatalf("merged diff (-want +got):\n%s", diff)
			}
		})
	}
}

func normalizeOrFatal(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	if v == nil {
		return nil
	}
	norm, err := normalizeValue(v)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	out, ok := norm.(map[string]any)
	if !ok {
		t.Fatalf("expected map after normalize, got %T", norm)
	}
	return out
}

func normalizeValue(v any) (any, error) {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			n, err := normalizeValue(val)
			if err != nil {
				return nil, err
			}
			out[k] = n
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("non-string key %v", k)
			}
			n, err := normalizeValue(val)
			if err != nil {
				return nil, err
			}
			out[ks] = n
		}
		return out, nil
	case []interface{}:
		out := make([]any, len(typed))
		for i, val := range typed {
			n, err := normalizeValue(val)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	default:
		return typed, nil
	}
}
