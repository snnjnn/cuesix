package dispatcher_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"iter"

	"github.com/warpcondev/cuesix/internal/compiler"
	"github.com/warpcondev/cuesix/internal/dispatcher"
	"github.com/warpcondev/cuesix/internal/testutil"
)

type mockFetcher struct {
	snippets []compiler.Snippet
	err      error
}

func (m mockFetcher) Fetch(roots ...compiler.InputRoot) iter.Seq2[compiler.Snippet, error] {
	return func(yield func(compiler.Snippet, error) bool) {
		if m.err != nil {
			yield(compiler.Snippet{}, m.err)
			return
		}
		for _, s := range m.snippets {
			if !yield(s, nil) {
				return
			}
		}
	}
}

type mockMerger struct {
	ResetCount  int
	CommitCount int
	outputs     []map[string]any
	errs        []error
	pos         int
}

func (m *mockMerger) Reset()  { m.ResetCount++ }
func (m *mockMerger) Commit() { m.CommitCount++ }
func (m *mockMerger) Merge(iter.Seq[compiler.Snippet]) (map[string]any, error) {
	defer func() { m.pos++ }()
	if m.pos < len(m.errs) && m.errs[m.pos] != nil {
		return nil, m.errs[m.pos]
	}
	if m.pos < len(m.outputs) {
		return m.outputs[m.pos], nil
	}
	return map[string]any{}, nil
}

type mockSerializer struct {
	ResetCount  int
	CommitCount int
	outputs     [][]byte
	errs        []error
	pos         int
}

func (m *mockSerializer) Reset()  { m.ResetCount++ }
func (m *mockSerializer) Commit() { m.CommitCount++ }
func (m *mockSerializer) Serialize(value map[string]any) ([]byte, error) {
	defer func() { m.pos++ }()
	if m.pos < len(m.errs) && m.errs[m.pos] != nil {
		return nil, m.errs[m.pos]
	}
	if m.pos < len(m.outputs) {
		return m.outputs[m.pos], nil
	}
	return []byte("default"), nil
}

type mockValidator struct {
	ResetCount  int
	CommitCount int
	results     []struct {
		ok  bool
		err error
	}
	pos int
}

func (m *mockValidator) Reset()  { m.ResetCount++ }
func (m *mockValidator) Commit() { m.CommitCount++ }
func (m *mockValidator) Validate(candidate []byte, isYAML bool) (bool, error) {
	defer func() { m.pos++ }()
	if m.pos < len(m.results) {
		return m.results[m.pos].ok, m.results[m.pos].err
	}
	return true, nil
}

type mockReloader struct {
	payloads [][]byte
	errs     []error
}

func (m *mockReloader) Apply(ctx context.Context, virtualgw string, payload []byte) error {
	m.payloads = append(m.payloads, payload)
	idx := len(m.payloads) - 1
	if idx < len(m.errs) && m.errs[idx] != nil {
		return m.errs[idx]
	}
	return nil
}

type mockMergerFactory struct {
	instance dispatcher.Merger
}

func (m mockMergerFactory) Instance(string) dispatcher.Merger {
	return m.instance
}

type mockSerializerFactory struct {
	instance dispatcher.Serializer
}

func (m mockSerializerFactory) Instance(string) dispatcher.Serializer {
	return m.instance
}

type mockValidatorFactory struct {
	instance dispatcher.Validator
}

func (m mockValidatorFactory) Instance(string) dispatcher.Validator {
	return m.instance
}

func newDispatcherSuite(fetcher mockFetcher, merger *mockMerger, serializer *mockSerializer, validator *mockValidator, reloader *mockReloader, runs int) (*dispatcher.Dispatcher, chan struct{}) {
	done := make(chan struct{}, runs)
	cfg := dispatcher.Config{
		Fetcher:           fetcher,
		MergerFactory:     mockMergerFactory{instance: merger},
		SerializerFactory: mockSerializerFactory{instance: serializer},
		ValidatorFactory:  mockValidatorFactory{instance: validator},
		Reloader:          reloader,
		Scheduler: func(_ context.Context, action func()) {
			action()
			done <- struct{}{}
		},
		Filesystems: []compiler.InputRoot{{Name: "test"}},
	}
	d, err := dispatcher.New(testutil.Logger(), cfg)
	if err != nil {
		panic(err)
	}
	return d, done
}

func runDispatcher(t *testing.T, d *dispatcher.Dispatcher, done chan struct{}, runs int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() {
		_ = d.Run(ctx)
	})
	for i := range runs {
		d.Notify()
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatalf("dispatcher did not complete run %d: %v", i, ctx.Err())
		}
	}
	cancel()
	wg.Wait()
}

func TestResetAndCommitViaRun(t *testing.T) {
	t.Parallel()
	merger := &mockMerger{}
	serializer := &mockSerializer{}
	validator := &mockValidator{}
	reloader := &mockReloader{}
	d, done := newDispatcherSuite(mockFetcher{snippets: []compiler.Snippet{{
		Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
		Data:      map[string]any{"a": 1},
	}}}, merger, serializer, validator, reloader, 1)

	runDispatcher(t, d, done, 1)
	if merger.ResetCount != 1 || serializer.ResetCount != 1 || validator.ResetCount != 1 {
		t.Fatalf("expected one reset per stage, got merger=%d serializer=%d validator=%d", merger.ResetCount, serializer.ResetCount, validator.ResetCount)
	}
	if merger.CommitCount != 1 || serializer.CommitCount != 1 || validator.CommitCount != 1 {
		t.Fatalf("expected commits on success, got merger=%d serializer=%d validator=%d", merger.CommitCount, serializer.CommitCount, validator.CommitCount)
	}

	// Reloader fails: no commits
	merger = &mockMerger{}
	serializer = &mockSerializer{}
	validator = &mockValidator{}
	reloader = &mockReloader{errs: []error{errors.New("boom")}}
	d, done = newDispatcherSuite(mockFetcher{snippets: []compiler.Snippet{{
		Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
		Data:      map[string]any{"a": 1},
	}}}, merger, serializer, validator, reloader, 1)
	runDispatcher(t, d, done, 1)
	if merger.CommitCount != 0 || serializer.CommitCount != 0 || validator.CommitCount != 0 {
		t.Fatalf("expected no commits on reload failure, got merger=%d serializer=%d validator=%d", merger.CommitCount, serializer.CommitCount, validator.CommitCount)
	}
}

func TestCachingSkipsReloadAfterSuccessfulCommit(t *testing.T) {
	t.Parallel()
	merger := &mockMerger{}
	serializer := &mockSerializer{outputs: [][]byte{[]byte("cfg"), nil}}
	validator := &mockValidator{}
	reloader := &mockReloader{}
	d, done := newDispatcherSuite(mockFetcher{snippets: []compiler.Snippet{{
		Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
		Data:      map[string]any{"a": 1},
	}}}, merger, serializer, validator, reloader, 2)

	runDispatcher(t, d, done, 2)
	if len(reloader.payloads) != 1 {
		t.Fatalf("expected one reload, got %d", len(reloader.payloads))
	}
	if validator.ResetCount != 1 {
		t.Fatalf("validator should not run on unchanged second pass")
	}
}

func TestCachingRetriesAfterFailure(t *testing.T) {
	t.Parallel()
	merger := &mockMerger{}
	serializer := &mockSerializer{outputs: [][]byte{[]byte("cfg1"), []byte("cfg2"), nil}}
	validator := &mockValidator{}
	reloader := &mockReloader{errs: []error{nil, errors.New("fail"), nil}}
	d, done := newDispatcherSuite(mockFetcher{snippets: []compiler.Snippet{{
		Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
		Data:      map[string]any{"a": 1},
	}}}, merger, serializer, validator, reloader, 3)

	runDispatcher(t, d, done, 3)
	if len(reloader.payloads) != 3 {
		t.Fatalf("expected three reload attempts, got %d", len(reloader.payloads))
	}
	if string(reloader.payloads[2]) != "cfg1" {
		t.Fatalf("expected retry with last committed payload, got %q", reloader.payloads[2])
	}
	if merger.CommitCount != 2 || serializer.CommitCount != 2 || validator.CommitCount != 2 {
		t.Fatalf("expected commits only on success, got merger=%d serializer=%d validator=%d", merger.CommitCount, serializer.CommitCount, validator.CommitCount)
	}
}
