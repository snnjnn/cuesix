package main

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// Dispatcher is the expected API of the process that performs the actual work.
type Dispatcher interface {
	Notify()
}

// Reloader is the expected API of the process that applies the work result.
type Reloader interface {
	Apply(context.Context, *slog.Logger, []byte, bool) error
}

// Watch is a source of events that trigger work execution.
// Returns true to break the loop
// (that is, when the context is canceled)
type Watch func(ctx context.Context) (cancelled bool)

// refreshManager coordinates reload readiness and cert-driven refreshes.
type refreshManager struct {
	active         atomic.Bool
	ready          atomic.Bool
	realDispatcher Dispatcher
	realReloader   Reloader
}

// Notify implements listener.Notifier and marks the manager inactive until the next reload.
func (r *refreshManager) Notify() {
	r.active.Store(false)
	r.realDispatcher.Notify()
}

// Apply implements dispatcher.Reloader and marks readiness on successful reload.
func (r *refreshManager) Apply(ctx context.Context, logger *slog.Logger, payload []byte, useApi bool) error {
	err := r.realReloader.Apply(ctx, logger, payload, useApi)
	if err != nil {
		return err
	}
	// If apply worked, we become active and ready
	r.active.Store(true)
	r.ready.Store(true)
	return err
}

// Watch subscribes to certificate updates and triggers the dispatcher when active.
func (r *refreshManager) Watch(ctx context.Context, logger *slog.Logger, watcher Watch) {
	for !watcher(ctx) {
		if r.active.Load() {
			logger.Info("triggering reload by watcher")
			r.active.Store(false)
			r.realDispatcher.Notify()
		}
	}
}

// Ready reports whether a successful reload has completed.
func (r *refreshManager) Ready() bool {
	return r.ready.Load()
}

// newRefreshManager builds a refresh manager around the reloader.
func newRefreshManager(reloader Reloader) *refreshManager {
	r := &refreshManager{
		realReloader: reloader,
	}
	r.active.Store(false)
	r.ready.Store(false)
	return r
}
