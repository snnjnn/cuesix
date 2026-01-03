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
	Apply(context.Context, []byte, bool) error
}

type Scheduler interface {
	Must(context.Context, func())
}

// Watch is a source of events that trigger work execution.
// Returns true to break the loop
// (that is, when the context is canceled)
type Watch func(ctx context.Context) (cancelled bool)

// readyManager coordinates reload readiness and cert-driven refreshes.
type readyManager struct {
	active         atomic.Bool
	ready          atomic.Bool
	realScheduler  Scheduler
	realDispatcher Dispatcher
	realReloader   Reloader
}

// Notify implements listener.Notify
func (r *readyManager) Notify() {
	r.realDispatcher.Notify()
}

// Must implements dispatcher.Scheduler
func (r *readyManager) Must(ctx context.Context, task func()) {
	r.active.Store(true)
	defer r.active.Store(false)
	r.realScheduler.Must(ctx, task)
}

// Apply implements dispatcher.Reloader and marks readiness on successful reload.
func (r *readyManager) Apply(ctx context.Context, payload []byte, useApi bool) error {
	err := r.realReloader.Apply(ctx, payload, useApi)
	if err != nil {
		return err
	}
	// If apply worked, we become active and ready
	r.ready.Store(true)
	return err
}

// Loop subscribes to certificate updates and triggers the dispatcher when active.
func (r *readyManager) Loop(ctx context.Context, logger *slog.Logger, watcher Watch) {
	for !watcher(ctx) {
		if !r.active.Load() {
			r.active.Store(true)
			logger.Info("triggering reload by watcher")
			r.realDispatcher.Notify()
		}
	}
}

// Ready reports whether a successful reload has completed.
func (r *readyManager) Ready() bool {
	return r.ready.Load()
}

// newReadyManager builds a refresh manager around the reloader.
func newReadyManager(reloader Reloader, scheduler Scheduler) *readyManager {
	r := &readyManager{
		realScheduler: scheduler,
		realReloader:  reloader,
	}
	r.active.Store(false)
	r.ready.Store(false)
	return r
}
