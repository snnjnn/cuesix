package cache

import (
	"os"
	"testing"
)

func TestCacheChangedDeterministic(t *testing.T) {
	c := &Cache{}

	firstPath, err := c.Changed(map[string]any{
		"a": 1,
		"b": 2,
	})
	if err != nil {
		t.Fatalf("first Changed returned error: %v", err)
	}
	if firstPath == "" {
		t.Fatalf("expected first Changed to return path")
	}
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("expected temp file to exist: %v", err)
	}
	defer func() {
		if err := os.Remove(firstPath); err != nil {
			t.Fatalf("failed to remove temp file: %v", err)
		}
	}()

	secondPath, err := c.Changed(map[string]any{
		"b": 2,
		"a": 1,
	})
	if err != nil {
		t.Fatalf("second Changed returned error: %v", err)
	}
	if secondPath != "" {
		t.Fatalf("expected same content to return empty path")
	}
}

func TestCacheChangedDifferent(t *testing.T) {
	c := &Cache{}

	firstPath, err := c.Changed(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("first Changed returned error: %v", err)
	}
	if firstPath == "" {
		t.Fatalf("expected first Changed to return path")
	}
	defer func() {
		if err := os.Remove(firstPath); err != nil {
			t.Fatalf("failed to remove temp file: %v", err)
		}
	}()

	secondPath, err := c.Changed(map[string]any{"a": 2})
	if err != nil {
		t.Fatalf("second Changed returned error: %v", err)
	}
	if secondPath == "" {
		t.Fatalf("expected different content to return path")
	}
	defer func() {
		if err := os.Remove(secondPath); err != nil {
			t.Fatalf("failed to remove temp file: %v", err)
		}
	}()
}

func TestCacheChangedInvalidValue(t *testing.T) {
	c := &Cache{}

	path, err := c.Changed(map[string]any{
		"a": map[any]any{1: "b"},
	})
	if err == nil {
		t.Fatalf("expected invalid value to return error")
	}
	if path != "" {
		t.Fatalf("expected empty path on error")
	}
}
