package dispatcher

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"time"
)

type stubCompiler struct {
	result map[string]any
	err    error
	calls  int
}

func (s *stubCompiler) Compile(...fs.FS) (map[string]any, error) {
	s.calls++
	return s.result, s.err
}

type stubCache struct {
	path string
	err  error
}

func (s *stubCache) Changed(map[string]any) (string, error) {
	return s.path, s.err
}

type stubValidator struct {
	ok    bool
	err   error
	calls int
}

func (s *stubValidator) Validate(string) (bool, error) {
	s.calls++
	return s.ok, s.err
}

type stubReloader struct {
	err   error
	calls int
	done  chan struct{}
}

func (s *stubReloader) Apply(context.Context, string) error {
	s.calls++
	if s.done != nil {
		s.done <- struct{}{}
	}
	return s.err
}

func TestDispatcherRemovesTempFile(t *testing.T) {
	tempDir := t.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "cache-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	comp := &stubCompiler{result: map[string]any{"a": 1}}
	cache := &stubCache{path: tempPath}
	validator := &stubValidator{ok: true}
	done := make(chan struct{}, 1)
	reloader := &stubReloader{done: done}

	disp, err := New(Config{
		Compiler:   comp,
		Cache:      cache,
		Validator:  validator,
		Reloader:   reloader,
		Filesystems: []fs.FS{os.DirFS(tempDir)},
		Cooldown:   0,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- disp.Run(ctx)
	}()

	disp.Notify()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for reload")
	}
	cancel()
	err = <-errCh
	if err != nil && err != context.Canceled {
		t.Fatalf("unexpected Run error: %v", err)
	}

	for start := time.Now(); time.Since(start) < 2*time.Second; {
		_, statErr := os.Stat(tempPath)
		if os.IsNotExist(statErr) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected temp file to be removed")
}

func TestDispatcherSkipsWhenUnchanged(t *testing.T) {
	comp := &stubCompiler{result: map[string]any{"a": 1}}
	cache := &stubCache{path: ""}
	validator := &stubValidator{ok: true}
	reloader := &stubReloader{}

	disp, err := New(Config{
		Compiler:   comp,
		Cache:      cache,
		Validator:  validator,
		Reloader:   reloader,
		Filesystems: []fs.FS{os.DirFS(".")},
		Cooldown:   0,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- disp.Run(ctx)
	}()

	disp.Notify()

	time.Sleep(50 * time.Millisecond)

	if validator.calls != 0 {
		t.Fatalf("expected validator to be skipped")
	}
	if reloader.calls != 0 {
		t.Fatalf("expected reloader to be skipped")
	}
	cancel()
	err = <-errCh
	if err != nil && err != context.Canceled {
		t.Fatalf("unexpected Run error: %v", err)
	}
}
