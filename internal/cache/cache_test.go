package cache

import (
	"log/slog"
	"testing"
)

func TestCacheChangedDeterministic(t *testing.T) {
	c := &Cache{}

	firstPath, err := c.Changed(slog.Default(), map[string]any{
		"a": 1,
		"b": 2,
	})
	if err != nil {
		t.Fatalf("first Changed returned error: %v", err)
	}
	if firstPath == nil {
		t.Fatalf("expected first Changed to return path")
	}

	secondPath, err := c.Changed(slog.Default(), map[string]any{
		"b": 2,
		"a": 1,
	})
	if err != nil {
		t.Fatalf("second Changed returned error: %v", err)
	}
	if secondPath != nil {
		t.Fatalf("expected same content to return empty path")
	}
}

func TestCacheChangedDifferent(t *testing.T) {
	c := &Cache{}

	firstPath, err := c.Changed(slog.Default(), map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("first Changed returned error: %v", err)
	}
	if firstPath == nil {
		t.Fatalf("expected first Changed to return path")
	}

	secondPath, err := c.Changed(nil, map[string]any{"a": 2})
	if err != nil {
		t.Fatalf("second Changed returned error: %v", err)
	}
	if secondPath == nil {
		t.Fatalf("expected different content to return path")
	}
}

func TestCacheChangedInvalidValue(t *testing.T) {
	c := &Cache{}

	path, err := c.Changed(slog.Default(), map[string]any{
		"a": func() {},
	})
	if err == nil {
		t.Fatalf("expected invalid value to return error")
	}
	if path != nil {
		t.Fatalf("expected empty path on error")
	}
}
