package factory

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/warpcomdev/sixpack/internal/plugin/ssl"
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
		scheduler:      NewScheduler(),
		CommittedCerts: make(map[ssl.Tracking]time.Time),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Inject mock tracker for the test
	factory.sslSetup.AcmeTracker = nil
	// Swap tracker by shadowing CommitLoop invocation
	go func() {
		factory.loop(ctx, 10*time.Millisecond, func() {
			tracker.Commit(ctx, slog.Default(), factory.CommittedCerts, time.Second)
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
		scheduler:      NewScheduler(),
		CommittedCerts: map[ssl.Tracking]time.Time{{Provider: "p", Identity: "a"}: time.Now()},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		factory.loop(ctx, 10*time.Millisecond, func() {
			tracker.Commit(ctx, slog.Default(), factory.CommittedCerts, time.Second)
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
