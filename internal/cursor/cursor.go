package cursor

import (
	"context"
	"iter"
	"sync"
)

// Cursor interface - turns a channel into an iterator
type Cursor[T any] <-chan T

// Owned cursor - grants the owner the means to close it
type Owned[T any] struct {
	Cursor[T]
	Close func()
}

// Channel facilitiates inferencing a Cursor from a channel
func Channel[T any](c <-chan T) Cursor[T] {
	return Cursor[T](c)
}

// Implements Cursor API
func (c Cursor[T]) Next(ctx context.Context) (zero T, ok bool) {
	select {
	case <-ctx.Done():
		return zero, false
	case zero, ok = <-c:
		return zero, ok
	}
}

func All[T any](ctx context.Context, c Cursor[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for {
			if v, ok := c.Next(ctx); ok {
				if yield(v) {
					continue
				}
			}
			return
		}
	}
}

type Lock struct {
	sync.Mutex
}

// Generic interface providing channel-based subscription facilities
type Watcher[T any] struct {
	Lock
	subs  map[chan T]string
	topic func(T) string
}

// Creates a cursor from the given watcher
func New[T any](topic func(T) string) *Watcher[T] {
	w := &Watcher[T]{}
	w.Embedded(topic)
	return w
}

// Initializes an embedded instance of a Watcher
// Use it when embedding a Watcher[T] instead of a *Watcher[T]
func (c *Watcher[T]) Embedded(topic func(T) string) {
	c.subs = make(map[chan T]string)
	c.topic = topic
}

// Creates a cursor from the given watcher
func (c *Watcher[T]) Watch(buffer int, topic string) Owned[T] {
	events := make(chan T, buffer)
	c.WithLock(func() {
		c.subs[events] = topic
	})
	return Owned[T]{
		Cursor: Channel(events),
		Close: func() {
			c.WithLock(func() {
				delete(c.subs, events)
			})
			close(events)
		},
	}
}

// Notify all watchers while holding the lock
func (c *Watcher[T]) NotifyAllLocked(ctx context.Context, v T) {
	var topic string
	if c.topic != nil {
		topic = c.topic(v)
	}
	for sub, req := range c.subs {
		if topic == "" || req == "" || topic == req {
			select {
			case <-ctx.Done():
				return
			case sub <- v:
			// Non - blocking: slow watchers will miss items.
			default:
			}
		}
	}
}

// Grab the lock and notify all holders
func (c *Watcher[T]) NotifyAll(ctx context.Context, v T) {
	c.WithLock(func() {
		c.NotifyAllLocked(ctx, v)
	})
}

// Runs the dispatch loop of a watcher.
// Unless you run a loop on the watcher, it won't trigger subscriptions.
func Loop[T any](ctx context.Context, w *Watcher[T], events Cursor[T]) {
	for v := range All(ctx, events) {
		w.WithLock(func() {
			w.NotifyAll(ctx, v)
		})
	}
}

func (l *Lock) WithLock(closure func()) {
	l.Lock()
	defer l.Unlock()
	closure()
}

// Turn a channel-based suscription into an iterable.
// I do not use iter.Seq because I intend to return three
// values: the iteration element, an optional error, and
// a flag to indicate that iteration was cancelled.
type watcherCursor[T any] struct {
	watcher *Watcher[T]
	fanout  chan T
}

// Unsubscribes from the watcher
func (c watcherCursor[T]) Close() {
	c.watcher.WithLock(func() {
		delete(c.watcher.subs, c.fanout)
	})
	close(c.fanout)
}

// Iterates over the next element.
// "cancelled" might be returned when context is cancelled,
// or something else prevents the iteration (e.g. the channel
// has been closed)
func (c watcherCursor[T]) Next(ctx context.Context) (zero T, ok bool) {
	return Channel(c.fanout).Next(ctx)
}
