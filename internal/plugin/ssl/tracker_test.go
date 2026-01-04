package ssl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/cursor"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestTrackerRequestCertificateNew(t *testing.T) {
	t.Parallel()
	provider := &mockACMEProvider{
		name: "p1",
		BestMatchForFunc: func(sni string) (Certificate, bool) {
			return sslCert(time.Now().Add(time.Hour)), true
		},
	}
	manager := mockACMEManager{providers: map[string]Provider{"p1": provider}}
	tracker, err := NewTracker(testutil.Logger(), manager)
	if err != nil {
		t.Fatalf("NewTracker returned error: %v", err)
	}
	key, err := tracker.RequestCertificate(context.Background(), "p1", "example.com")
	if err != nil {
		t.Fatalf("RequestCertificate returned error: %v", err)
	}
	if key.Provider != "p1" || key.Identity != "example.com" {
		t.Fatalf("unexpected key: %+v", key)
	}
	if len(provider.requestCalls) != 1 {
		t.Fatalf("expected provider request once, got %d", len(provider.requestCalls))
	}
	tracker.WithLock(func() {
		tc, ok := tracker.track[key]
		if !ok || tc.NotAfter.IsZero() {
			t.Fatalf("expected tracked certificate to be populated")
		}
	})
}

func TestTrackerRequestCertificateAlreadyTrackedNotifies(t *testing.T) {
	t.Parallel()
	provider := &mockACMEProvider{name: "p1"}
	manager := mockACMEManager{providers: map[string]Provider{"p1": provider}}
	tracker, _ := NewTracker(testutil.Logger(), manager)
	key := Tracking{Provider: "p1", Identity: "example.com"}
	tracked := trackedCertificate{Certificate: sslCert(time.Now().Add(time.Hour))}
	tracker.WithLock(func() {
		tracker.track[key] = tracked
	})
	events := tracker.Watch(1, "")
	defer events.Close()
	_, err := tracker.RequestCertificate(context.Background(), "p1", "example.com")
	if err != nil {
		t.Fatalf("RequestCertificate returned error: %v", err)
	}
	select {
	case cert := <-events.Cursor:
		if cert.Identity != key.Identity || cert.Provider != key.Provider {
			t.Fatalf("unexpected notification %+v", cert)
		}
	default:
		t.Fatalf("expected notification for tracked certificate")
	}
	if len(provider.requestCalls) != 0 {
		t.Fatalf("provider should not be called for tracked cert")
	}
}

func TestTrackerCommitAndUnmanage(t *testing.T) {
	t.Parallel()
	provider := &mockACMEProvider{
		name: "p1",
		BestMatchForFunc: func(sni string) (Certificate, bool) {
			return sslCert(time.Now().Add(2 * time.Hour)), true
		},
	}
	manager := mockACMEManager{providers: map[string]Provider{"p1": provider}}
	tracker, _ := NewTracker(testutil.Logger(), manager)
	keyTracked := Tracking{Provider: "p1", Identity: "tracked.example"}
	keyStale := Tracking{Provider: "p1", Identity: "stale.example"}
	now := time.Now()
	tracker.WithLock(func() {
		tracker.track[keyTracked] = trackedCertificate{
			Certificate: Certificate{NotAfter: now.Add(time.Hour)},
			TrackedAt:   now.Add(-time.Hour),
		}
		tracker.track[keyStale] = trackedCertificate{
			Certificate: Certificate{NotAfter: now.Add(30 * time.Minute)},
			TrackedAt:   now.Add(-2 * time.Hour),
		}
	})
	committed := map[Tracking]time.Time{
		keyTracked: time.Time{},
	}
	updates := tracker.Commit(context.Background(), testutil.Logger(), committed, time.Hour)
	if updates != 1 {
		t.Fatalf("expected 1 update, got %d", updates)
	}
	if committed[keyTracked].IsZero() {
		t.Fatalf("expected committed map updated with notAfter")
	}
	if len(provider.removeManagedCalls) != 1 || provider.removeManagedCalls[0].SNI != keyStale.Identity {
		t.Fatalf("expected RemoveManaged called for stale cert, calls: %+v", provider.removeManagedCalls)
	}
}

func TestUpdateLoopBroadcasts(t *testing.T) {
	t.Parallel()
	cert := sslCert(time.Now().Add(time.Hour))
	provider := &mockACMEProvider{
		name: "p1",
		BestMatchForFunc: func(sni string) (Certificate, bool) {
			return cert, true
		},
	}
	manager := mockACMEManager{providers: map[string]Provider{"p1": provider}}
	tracker, _ := NewTracker(testutil.Logger(), manager)
	events := make(chan Tracking, 1)
	events <- Tracking{Provider: "p1", Identity: "example.com"}
	close(events)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watch := tracker.Watch(1, "")
	defer watch.Close()
	UpdateLoop(ctx, testutil.Logger(), tracker, cursor.Channel(events))
	select {
	case update := <-watch.Cursor:
		if update.Provider != "p1" || update.Identity != "example.com" {
			t.Fatalf("unexpected update %+v", update)
		}
	default:
		t.Fatalf("expected notification for cert event")
	}
}

func TestProviderForCaches(t *testing.T) {
	t.Parallel()
	manager := mockACMEManager{providers: map[string]Provider{}}
	tracker, _ := NewTracker(testutil.Logger(), manager)
	cache := make(map[string]Provider)
	if _, err := tracker.providerFor("missing", cache); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected propagated error, got %v", err)
	}
}
