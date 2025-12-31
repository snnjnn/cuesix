package cursor

import (
	"context"
	"errors"
)

// Generic interface providing channel-based subscription facilities
type Watcher[T any] interface {
	Subscribe(buffer int) chan T
	Unsubscribe(chan T)
}

// Turn a channel-based suscription into an iterable.
// I do not use iter.Seq because I intend to return three
// values: the iteration element, an optional error, and
// a flag to indicate that iteration was cancelled.
type Cursor[T any] struct {
	watcher Watcher[T]
	events  chan T
}

// Creates a cursor from the given watcher
func New[T any](watcher Watcher[T], buffer int) Cursor[T] {
	events := watcher.Subscribe(buffer)
	return Cursor[T]{
		watcher: watcher,
		events:  events,
	}
}

// Unsubscribes from the watcher
func (c Cursor[T]) Close() {
	c.watcher.Unsubscribe(c.events)
}

// Iterates over the next element.
// "cancelled" might be returned when context is cancelled,
// or something else prevents the iteration (e.g. the channel
// has been closed)
func (c Cursor[T]) Next(ctx context.Context) (cancelled bool, zero T, err error) {
	select {
	case <-ctx.Done():
		return true, zero, nil
	case v, ok := <-c.events:
		if !ok {
			return true, zero, errors.New("watcher closed")
		}
		return false, v, nil
	}
}
