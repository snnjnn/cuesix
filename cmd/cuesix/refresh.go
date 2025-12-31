package main

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// Representa la API del proceso que se encarga realmente del trabajo.
type Dispatcher interface {
	Notify()
}

// Representa la API del proceso que aplica el resultado del trabajo
type Reloader interface {
	Apply(context.Context, *slog.Logger, []byte, bool) error
}

// Representa una fuenta de eventos que provocan la ejecución del trabajo.
// Devuelve "false" para interrumpir el bucle
// (es decir, cuando el contexto se cancela)
type Watch func(ctx context.Context) (canceled bool)

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
	for watcher(ctx) {
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
