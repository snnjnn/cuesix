package main

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/cache"
	"github.com/warpcomdev/cuesix/internal/plugin"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

func TestPluginCacheChangedRunsPipelineInOrder(t *testing.T) {
	var steps []string
	pre := plugin.PreRenderFunc(func(_ *slog.Logger, value map[string]any) (map[string]any, error) {
		steps = append(steps, "pre")
		value["pre"] = "ok"
		return value, nil
	})
	post := plugin.PostRenderFunc(func(_ *slog.Logger, value []byte) ([]byte, error) {
		steps = append(steps, "post")
		return append([]byte("post:"), value...), nil
	})
	pc := &pluginCache{
		preRender:  pre,
		postRender: post,
		cache:      &cache.Cache{},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	out, err := pc.Changed(logger, map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("Changed returned error: %v", err)
	}
	if out == nil {
		t.Fatalf("expected output")
	}
	if len(steps) != 2 || steps[0] != "pre" || steps[1] != "post" {
		t.Fatalf("unexpected step order: %#v", steps)
	}
}

func TestPluginCacheChangedSkipsPostWhenUnchanged(t *testing.T) {
	postCalls := 0
	pre := plugin.PreRenderFunc(func(_ *slog.Logger, value map[string]any) (map[string]any, error) {
		value["pre"] = "ok"
		return value, nil
	})
	post := plugin.PostRenderFunc(func(_ *slog.Logger, value []byte) ([]byte, error) {
		postCalls++
		return value, nil
	})
	pc := &pluginCache{
		preRender:  pre,
		postRender: post,
		cache:      &cache.Cache{},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	out, err := pc.Changed(logger, map[string]any{"a": 1})
	if err != nil || out == nil {
		t.Fatalf("expected first call to produce output")
	}
	out, err = pc.Changed(logger, map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output for unchanged data")
	}
	if postCalls != 1 {
		t.Fatalf("expected post render to be called once, got %d", postCalls)
	}
}

func TestPluginCacheChangedStopsOnPreRenderError(t *testing.T) {
	pre := plugin.PreRenderFunc(func(_ *slog.Logger, _ map[string]any) (map[string]any, error) {
		return nil, errors.New("pre-render failed")
	})
	postCalls := 0
	post := plugin.PostRenderFunc(func(_ *slog.Logger, value []byte) ([]byte, error) {
		postCalls++
		return value, nil
	})
	pc := &pluginCache{
		preRender:  pre,
		postRender: post,
		cache:      &cache.Cache{},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, err := pc.Changed(logger, map[string]any{"a": 1}); err == nil {
		t.Fatalf("expected error")
	}
	if postCalls != 0 {
		t.Fatalf("expected post render not to be called")
	}
}

func TestBuildPreRenderInvalidPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := buildPreRender([]string{missing}, nil, ssl.Certificate{}, time.Second); err == nil {
		t.Fatalf("expected error for invalid path")
	}
}
