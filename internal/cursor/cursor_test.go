package cursor_test

import (
	"context"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/cursor"
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
