package plugin_test

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/warpcomdev/cuesix/internal/plugin"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestPreRenderChain(t *testing.T) {
	t.Parallel()
	var calls []string
	chain := plugin.PreRenderChain{
		plugin.PreRenderFunc(func(_ *slog.Logger, v map[string]any) (map[string]any, error) {
			calls = append(calls, "a")
			v["a"] = 1
			return v, nil
		}),
		nil,
		plugin.PreRenderFunc(func(_ *slog.Logger, v map[string]any) (map[string]any, error) {
			calls = append(calls, "b")
			v["b"] = 2
			return v, nil
		}),
	}
	out, err := chain.Update(testutil.Logger(), map[string]any{"start": true})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if len(calls) != 2 || out["a"] != 1 || out["b"] != 2 {
		t.Fatalf("unexpected result: calls=%v out=%v", calls, out)
	}
}

func TestPreRenderChainError(t *testing.T) {
	t.Parallel()
	chain := plugin.PreRenderChain{
		plugin.PreRenderFunc(func(_ *slog.Logger, v map[string]any) (map[string]any, error) {
			return nil, errors.New("boom")
		}),
		plugin.PreRenderFunc(func(_ *slog.Logger, v map[string]any) (map[string]any, error) {
			return v, nil
		}),
	}
	if _, err := chain.Update(testutil.Logger(), map[string]any{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestPostRenderChain(t *testing.T) {
	t.Parallel()
	var calls []string
	chain := plugin.PostRenderChain{
		plugin.PostRenderFunc(func(_ *slog.Logger, v []byte) ([]byte, error) {
			calls = append(calls, "a")
			return append(v, 'a'), nil
		}),
		nil,
		plugin.PostRenderFunc(func(_ *slog.Logger, v []byte) ([]byte, error) {
			calls = append(calls, "b")
			return append(v, 'b'), nil
		}),
	}
	out, err := chain.Update(testutil.Logger(), []byte("x"))
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if string(out) != "xab" {
		t.Fatalf("unexpected output %q", out)
	}
	if len(calls) != 2 {
		t.Fatalf("expected two calls, got %d", len(calls))
	}
}

func TestPostRenderChainError(t *testing.T) {
	t.Parallel()
	chain := plugin.PostRenderChain{
		plugin.PostRenderFunc(func(_ *slog.Logger, v []byte) ([]byte, error) {
			return nil, errors.New("boom")
		}),
	}
	if _, err := chain.Update(testutil.Logger(), []byte("x")); err == nil {
		t.Fatalf("expected error")
	}
}
