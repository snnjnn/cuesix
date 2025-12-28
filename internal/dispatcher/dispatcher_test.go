package dispatcher

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"
	"testing/synctest"
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
	data []byte
	err  error
}

func (s *stubCache) Changed(map[string]any) ([]byte, error) {
	return s.data, s.err
}

type stubValidator struct {
	ok    bool
	err   error
	calls int
}

func (s *stubValidator) Validate([]byte, bool) (bool, error) {
	s.calls++
	return s.ok, s.err
}

type stubReloader struct {
	err   error
	calls int
	done  chan struct{}
}

func (s *stubReloader) Apply(context.Context, []byte) error {
	s.calls++
	if s.done != nil {
		s.done <- struct{}{}
	}
	return s.err
}

func TestDispatcherSkipsWhenUnchanged(t *testing.T) {
	comp := &stubCompiler{result: map[string]any{"a": 1}}
	cache := &stubCache{data: nil}
	validator := &stubValidator{ok: true}
	reloader := &stubReloader{}

	disp, err := New(Config{
		Compiler:    comp,
		Cache:       cache,
		Validator:   validator,
		Reloader:    reloader,
		Filesystems: []fs.FS{os.DirFS(".")},
		OutputYAML:  false,
		Cooldown:    0,
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

func TestDispatcherReturnsCompilerError(t *testing.T) {
	comp := &stubCompiler{err: errors.New("compile fail")}
	cache := &stubCache{}
	validator := &stubValidator{}
	reloader := &stubReloader{}

	disp, err := New(Config{
		Compiler:    comp,
		Cache:       cache,
		Validator:   validator,
		Reloader:    reloader,
		Filesystems: []fs.FS{os.DirFS(".")},
		OutputYAML:  false,
		Cooldown:    0,
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
	case runErr := <-errCh:
		if runErr == nil {
			t.Fatalf("expected error")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for error")
	}
}

func TestDispatcherReturnsValidationError(t *testing.T) {
	comp := &stubCompiler{result: map[string]any{"a": 1}}
	cache := &stubCache{data: []byte("data")}
	validator := &stubValidator{ok: false}
	reloader := &stubReloader{}

	disp, err := New(Config{
		Compiler:    comp,
		Cache:       cache,
		Validator:   validator,
		Reloader:    reloader,
		Filesystems: []fs.FS{os.DirFS(".")},
		OutputYAML:  false,
		Cooldown:    0,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	err = disp.handle(context.Background())
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if reloader.calls != 0 {
		t.Fatalf("expected reloader not to be called")
	}
}

func TestWaitForCooldownRespectsLastDequeued(t *testing.T) {
	comp := &stubCompiler{}
	cache := &stubCache{}
	validator := &stubValidator{}
	reloader := &stubReloader{}

	disp, err := New(Config{
		Compiler:    comp,
		Cache:       cache,
		Validator:   validator,
		Reloader:    reloader,
		Filesystems: []fs.FS{os.DirFS(".")},
		OutputYAML:  false,
		Cooldown:    50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	disp.lastDequeued = time.Now().Add(-60 * time.Millisecond)
	if err := disp.waitForCooldown(context.Background()); err != nil {
		t.Fatalf("unexpected cooldown error: %v", err)
	}
}

func TestSleepContextWithSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done := make(chan error, 1)
		go func() {
			done <- sleepContext(context.Background(), 1*time.Second)
		}()

		time.Sleep(500 * time.Millisecond)
		select {
		case <-done:
			t.Fatalf("sleepContext returned too early")
		default:
		}

		time.Sleep(500 * time.Millisecond)
		if err := <-done; err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestWaitAfterDequeueWithSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		disp, err := New(Config{
			Compiler:    &stubCompiler{},
			Cache:       &stubCache{},
			Validator:   &stubValidator{},
			Reloader:    &stubReloader{},
			Filesystems: []fs.FS{os.DirFS(".")},
			OutputYAML:  false,
			Cooldown:    1 * time.Second,
		})
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}

		now := time.Now()
		disp.lastDequeued = now
		dequeuedAt := now.Add(500 * time.Millisecond)

		done := make(chan error, 1)
		go func() {
			done <- disp.waitAfterDequeue(context.Background(), dequeuedAt)
		}()

		time.Sleep(400 * time.Millisecond)
		select {
		case <-done:
			t.Fatalf("waitAfterDequeue returned too early")
		default:
		}

		time.Sleep(100 * time.Millisecond)
		if err := <-done; err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
