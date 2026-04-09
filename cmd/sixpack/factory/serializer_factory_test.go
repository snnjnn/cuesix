package factory

import (
	"context"
	"log/slog"
	"maps"
	"testing"
	"time"

	"github.com/warpcondev/cuesix/internal/compiler"
	"github.com/warpcondev/cuesix/internal/plugin/ssl"
)

// mockTracker only implements the methods used by serializer_factory loops.
type mockTracker struct {
	commitCalls []struct {
		ctx   context.Context
		grace time.Duration
		count int
	}
}

func (m *mockTracker) Commit(ctx context.Context, _ *slog.Logger, committed map[ssl.Tracking]time.Time, grace time.Duration) int {
	m.commitCalls = append(m.commitCalls, struct {
		ctx   context.Context
		grace time.Duration
		count int
	}{ctx: ctx, grace: grace, count: len(committed)})
	return len(committed)
}

func TestCommitLoopRunsWhenNoCommittedCerts(t *testing.T) {
	t.Parallel()
	tracker := &mockTracker{}
	factory := SerializerFactory{
		sslSetup: SSLSetup{
			AcmeTracker: (*ssl.Tracker)(nil),
		},
		scheduler: NewScheduler(),
		instances: make(map[string]*SerializerInstance),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Inject mock tracker for the test
	factory.sslSetup.AcmeTracker = nil
	// Swap tracker by shadowing CommitLoop invocation
	go func() {
		factory.loop(ctx, 10*time.Millisecond, func() {
			tracker.Commit(ctx, slog.Default(), factory.allCommittedCerts(), time.Second)
		})
	}()
	// Allow one loop tick.
	time.Sleep(20 * time.Millisecond)
	cancel()
	if len(tracker.commitCalls) == 0 {
		t.Fatalf("expected CommitLoop to invoke tracker.Commit when no committed certs")
	}
	if tracker.commitCalls[0].grace != time.Second {
		t.Fatalf("unexpected grace period %v", tracker.commitCalls[0].grace)
	}
}

func TestCommitLoopRunsWhenCommittedCertsPresent(t *testing.T) {
	t.Parallel()
	tracker := &mockTracker{}
	factory := SerializerFactory{
		sslSetup: SSLSetup{
			AcmeTracker: (*ssl.Tracker)(nil),
		},
		scheduler: NewScheduler(),
		instances: map[string]*SerializerInstance{
			compiler.DEFAULT_VIRTUALGW: &SerializerInstance{
				CommittedCerts: map[ssl.Tracking]time.Time{{Provider: "p", Identity: "a"}: time.Now()},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		factory.loop(ctx, 10*time.Millisecond, func() {
			tracker.Commit(ctx, slog.Default(), factory.allCommittedCerts(), time.Second)
		})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if len(tracker.commitCalls) == 0 {
		t.Fatalf("expected CommitLoop to invoke tracker.Commit when committed certs exist")
	}
	if tracker.commitCalls[0].count != 1 {
		t.Fatalf("expected commit count 1, got %d", tracker.commitCalls[0].count)
	}
}

func TestAllCommittedCertsUnionsAcrossVirtualGateways(t *testing.T) {
	t.Parallel()

	now := time.Now()
	shared := ssl.Tracking{Provider: "p", Identity: "shared.example"}
	leftOnly := ssl.Tracking{Provider: "p", Identity: "left.example"}
	rightOnly := ssl.Tracking{Provider: "p", Identity: "right.example"}
	leftTime := now.Add(-time.Minute)
	rightTime := now

	factory := SerializerFactory{
		instances: map[string]*SerializerInstance{
			compiler.DEFAULT_VIRTUALGW: {
				CommittedCerts: map[ssl.Tracking]time.Time{
					shared:   leftTime,
					leftOnly: leftTime,
				},
			},
			"secondary": {
				CommittedCerts: map[ssl.Tracking]time.Time{
					shared:    rightTime,
					rightOnly: rightTime,
				},
			},
		},
	}

	got := factory.allCommittedCerts()
	if len(got) != 3 {
		t.Fatalf("expected 3 committed certs, got %d", len(got))
	}
	if !maps.Equal(got, map[ssl.Tracking]time.Time{
		shared:    rightTime,
		leftOnly:  leftTime,
		rightOnly: rightTime,
	}) {
		t.Fatalf("unexpected committed cert union: %#v", got)
	}
}
