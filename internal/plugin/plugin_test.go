package plugin

import (
	"errors"
	"testing"
)

type stubPlugin struct {
	calls int
	err   error
}

func (s *stubPlugin) Update(value map[string]any) (map[string]any, error) {
	s.calls++
	return value, s.err
}

func TestChainEmpty(t *testing.T) {
	var chain Chain
	input := map[string]any{"a": 1}
	output, err := chain.Update(input)
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
	chain := Chain{first, second, third}

	_, err := chain.Update(map[string]any{"a": 1})
	if err == nil {
		t.Fatalf("expected error")
	}
	if first.calls != 1 || second.calls != 1 || third.calls != 0 {
		t.Fatalf("unexpected call counts: %d %d %d", first.calls, second.calls, third.calls)
	}
}

var errTest = errors.New("plugin error")
