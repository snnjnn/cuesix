package cursor_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/warpcondev/cuesix/internal/cursor"
)

func TestChannelNextAndAll(t *testing.T) {
	t.Parallel()
	ch := make(chan int, 2)
	ch <- 1
	ch <- 2
	close(ch)
	c := cursor.Channel(ch)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, ok := c.Next(ctx)
	if !ok || v != 1 {
		t.Fatalf("expected first value 1, got %v ok=%v", v, ok)
	}
	seen := []int{v}
	for v := range cursor.All(ctx, c) {
		seen = append(seen, v)
	}
	if len(seen) != 2 || seen[1] != 2 {
		t.Fatalf("unexpected values %v", seen)
	}
}

func TestWatcherNotify(t *testing.T) {
	t.Parallel()
	w := cursor.New(func(s string) string { return s })
	all := w.Watch(2, "")
	defer all.Close()
	onlyA := w.Watch(1, "a")
	defer onlyA.Close()
	w.NotifyAllLocked(context.Background(), "a")
	w.NotifyAllLocked(context.Background(), "b")

	select {
	case v := <-all.Cursor:
		if v != "a" {
			t.Fatalf("expected a, got %s", v)
		}
	default:
		t.Fatalf("expected value on catch-all watcher")
	}
	select {
	case v := <-all.Cursor:
		if v != "b" {
			t.Fatalf("expected b, got %s", v)
		}
	default:
		t.Fatalf("expected second value on catch-all watcher")
	}
	select {
	case v := <-onlyA.Cursor:
		if v != "a" {
			t.Fatalf("expected only a, got %s", v)
		}
	default:
		t.Fatalf("expected value on topic watcher")
	}
}

func TestNotifyAllUsesLock(t *testing.T) {
	t.Parallel()
	w := cursor.New(func(s string) string { return s })
	events := w.Watch(1, "")
	defer events.Close()
	w.NotifyAll(context.Background(), "ping")
	select {
	case v := <-events.Cursor:
		if v != "ping" {
			t.Fatalf("expected ping, got %s", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected notification via NotifyAll")
	}
}

func TestLoopDispatchesAndStops(t *testing.T) {
	t.Parallel()
	w := cursor.New(func(s string) string { return s })
	sub := w.Watch(2, "")

	events := make(chan string, 2)
	var doneWG sync.WaitGroup
	doneWG.Go(func() {
		cursor.Loop(context.Background(), w, cursor.Channel(events))
	})

	events <- "first"
	events <- "second"
	close(events)

	doneWG.Wait()
	sub.Close()

	got := []string{}
	for v := range sub.Cursor {
		got = append(got, v)
	}
	if got[0] != "first" || got[1] != "second" {
		t.Fatalf("unexpected values %v", got)
	}
}
