package plugin

import (
	"errors"
	"log/slog"
	"testing"
)

type stubPlugin struct {
	calls int
	err   error
}

func (s *stubPlugin) Update(_ *slog.Logger, value map[string]any) (map[string]any, error) {
	s.calls++
	return value, s.err
}

func TestChainEmpty(t *testing.T) {
	var chain PreRenderChain
	input := map[string]any{"a": 1}
	output, err := chain.Update(slog.Default(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["a"] != 1 {
		t.Fatalf("expected output unchanged")
	}
}

func TestChainStopsOnError(t *testing.T) {
	first := &stubPlugin{}
	second := &stubPlugin{err: errTest}
	third := &stubPlugin{}
	chain := PreRenderChain{first, second, third}

	_, err := chain.Update(slog.Default(), map[string]any{"a": 1})
	if err == nil {
		t.Fatalf("expected error")
	}
	if first.calls != 1 || second.calls != 1 || third.calls != 0 {
		t.Fatalf("unexpected call counts: %d %d %d", first.calls, second.calls, third.calls)
	}
}

var errTest = errors.New("plugin error")

type stubPostPlugin struct {
	calls int
	err   error
}

func (s *stubPostPlugin) Update(_ *slog.Logger, value []byte) ([]byte, error) {
	s.calls++
	return value, s.err
}

func TestPostRenderChainStopsOnError(t *testing.T) {
	first := &stubPostPlugin{}
	second := &stubPostPlugin{err: errTest}
	third := &stubPostPlugin{}
	chain := PostRenderChain{first, second, third}

	_, err := chain.Update(slog.Default(), []byte("data"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if first.calls != 1 || second.calls != 1 || third.calls != 0 {
		t.Fatalf("unexpected call counts: %d %d %d", first.calls, second.calls, third.calls)
	}
}

func TestPostRenderFunc(t *testing.T) {
	calls := 0
	fn := PostRenderFunc(func(_ *slog.Logger, value []byte) ([]byte, error) {
		calls++
		return append(value, 'x'), nil
	})

	out, err := fn.Update(slog.Default(), []byte("data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "datax" || calls != 1 {
		t.Fatalf("unexpected result: %q calls=%d", out, calls)
	}
}

func TestPreRenderFunc(t *testing.T) {
	calls := 0
	fn := PreRenderFunc(func(_ *slog.Logger, value map[string]any) (map[string]any, error) {
		calls++
		value["b"] = 2
		return value, nil
	})

	out, err := fn.Update(slog.Default(), map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["b"] != 2 || calls != 1 {
		t.Fatalf("unexpected output: %#v calls=%d", out, calls)
	}
}
