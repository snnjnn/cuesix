package certmagicmgr

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

type fakeProvider struct {
	name string
	cert ssl.Certificate
}

func (f fakeProvider) Name() string {
	return f.name
}

func (f fakeProvider) BestMatchFor(_ string, _ *slog.Logger) (ssl.Certificate, bool) {
	return f.cert, true
}

func TestWatcherSubscribeAndUnsubscribe(t *testing.T) {
	events := make(chan CertEvent, 1)
	watcher, err := NewWatcher(nil, events)
	if err != nil {
		t.Fatalf("NewWatcher returned error: %v", err)
	}
	ch := watcher.Subscribe(1)
	watcher.withLock(func() {
		watcher.notifyAllLocked(Notification{Provider: "p1", SNI: "example.com"})
	})

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("expected notification")
	}

	watcher.Unsubscribe(ch)
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected channel to be closed")
		}
	default:
		t.Fatalf("expected closed channel")
	}
}

func TestWatcherRunWatchEmitsNotifications(t *testing.T) {
	events := make(chan CertEvent, 1)
	watcher, err := NewWatcher(nil, events)
	if err != nil {
		t.Fatalf("NewWatcher returned error: %v", err)
	}
	ch := watcher.Subscribe(1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{}, 1)
	go func() {
		watcher.RunWatch(ctx, logger, time.Hour)
		done <- struct{}{}
	}()

	events <- CertEvent{
		sni: "example.com",
		provider: fakeProvider{
			name: "p1",
			cert: ssl.Certificate{NotAfter: time.Now().UTC().Add(time.Hour)},
		},
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected notification")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected RunWatch to exit")
	}
}

func TestWatcherClearTrackingNoEntries(t *testing.T) {
	events := make(chan CertEvent, 1)
	watcher, err := NewWatcher(nil, events)
	if err != nil {
		t.Fatalf("NewWatcher returned error: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	watcher.ClearTracking(logger)
	if len(watcher.track) != 0 {
		t.Fatalf("expected empty tracking map")
	}
}
