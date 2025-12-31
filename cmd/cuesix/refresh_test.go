package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

type fakeDispatcher struct {
	notified int
}

func (f *fakeDispatcher) Notify() {
	f.notified++
}

type fakeReloader struct {
	err   error
	calls int
}

func (f *fakeReloader) Apply(_ context.Context, _ *slog.Logger, _ []byte, _ bool) error {
	f.calls++
	return f.err
}

func TestRefreshManagerNotifyClearsActive(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	manager := newRefreshManager(&fakeReloader{})
	manager.realDispatcher = dispatcher
	manager.active.Store(true)

	manager.Notify()

	if manager.active.Load() {
		t.Fatalf("expected active to be false")
	}
	if dispatcher.notified != 1 {
		t.Fatalf("expected dispatcher notify to be called once")
	}
}

func TestRefreshManagerApplyMarksReady(t *testing.T) {
	reloader := &fakeReloader{}
	manager := newRefreshManager(reloader)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if manager.Ready() {
		t.Fatalf("expected ready to be false at start")
	}

	if err := manager.Apply(context.Background(), logger, []byte("payload"), true); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !manager.active.Load() {
		t.Fatalf("expected active to be true after successful apply")
	}
	if !manager.Ready() {
		t.Fatalf("expected ready to be true after successful apply")
	}
	if reloader.calls != 1 {
		t.Fatalf("expected reloader apply to be called once")
	}
}

func TestRefreshManagerApplyErrorDoesNotMarkReady(t *testing.T) {
	reloader := &fakeReloader{err: errors.New("boom")}
	manager := newRefreshManager(reloader)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := manager.Apply(context.Background(), logger, []byte("payload"), true); err == nil {
		t.Fatalf("expected error")
	}
	if manager.active.Load() {
		t.Fatalf("expected active to remain false")
	}
	if manager.Ready() {
		t.Fatalf("expected ready to remain false")
	}
}

func TestRefreshManagerWatchTriggersWhenActive(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	manager := newRefreshManager(&fakeReloader{})
	manager.realDispatcher = dispatcher
	manager.active.Store(true)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	calls := 0
	watcher := func(_ context.Context) bool {
		calls++
		return calls > 1
	}

	manager.Watch(context.Background(), logger, watcher)

	if dispatcher.notified != 1 {
		t.Fatalf("expected dispatcher notify to be called once")
	}
	if manager.active.Load() {
		t.Fatalf("expected active to be false after watch trigger")
	}
}

func TestRefreshManagerWatchIgnoresWhenInactive(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	manager := newRefreshManager(&fakeReloader{})
	manager.realDispatcher = dispatcher
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	calls := 0
	watcher := func(_ context.Context) bool {
		calls++
		return calls > 1
	}

	manager.Watch(context.Background(), logger, watcher)

	if dispatcher.notified != 0 {
		t.Fatalf("expected dispatcher notify to not be called")
	}
}
