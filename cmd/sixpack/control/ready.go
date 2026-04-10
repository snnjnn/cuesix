package control

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/cursor"
)

// Dispatcher is the expected API of the process that performs the actual work.
type Dispatcher interface {
	Notify()
}

// Reloader is the expected API of the process that applies the work result.
type Reloader interface {
	Apply(context.Context, string, []byte) error
}

// Scheduler is the API for scheduling tasks that must run.
type Scheduler interface {
	Must(context.Context, func())
}

// Watch is a source of events that trigger work execution.
// Returns true to break the loop
// (that is, when the context is canceled)
type Watch func(ctx context.Context) (cancelled bool)

// ReadyReloader wraps a reloader to track readiness.
type ReadyReloader struct {
	cursor.Lock
	Reloader
	maxGatewayDepth int
	ready           map[string]bool
}

// NewReadyReloader builds a readiness-tracking reloader.
func NewReadyReloader(reloader Reloader, maxGatewayDepth int) ReadyReloader {
	return ReadyReloader{
		Reloader:        reloader,
		maxGatewayDepth: maxGatewayDepth,
		ready:           make(map[string]bool),
	}
}

// ReadyManager coordinates reload readiness and cert-driven refreshes.
type ReadyManager struct {
	active atomic.Bool
	ReadyReloader
	realScheduler  Scheduler
	realDispatcher Dispatcher
}

// Notify implements listener.Notify
func (r *ReadyManager) Notify() {
	r.realDispatcher.Notify()
}

// Must implements dispatcher.Scheduler
func (r *ReadyManager) Must(ctx context.Context, task func()) {
	r.active.Store(true)
	defer r.active.Store(false)
	r.realScheduler.Must(ctx, task)
}

// Apply implements dispatcher.Reloader and marks readiness on successful reload.
func (r *ReadyReloader) Apply(ctx context.Context, virtualgw string, payload []byte) error {
	err := r.Reloader.Apply(ctx, virtualgw, payload)
	if err != nil {
		return err
	}
	// If apply worked, we become active and ready
	r.WithLock(func() {
		if r.ready == nil {
			r.ready = make(map[string]bool)
		}
		r.ready[virtualgw] = true
	})
	return nil
}

// Loop subscribes to certificate updates and triggers the dispatcher when active.
func (r *ReadyManager) Loop(ctx context.Context, logger *slog.Logger, watcher Watch) {
	for !watcher(ctx) {
		if !r.active.Load() {
			r.active.Store(true)
			logger.Info("triggering reload by watcher")
			r.realDispatcher.Notify()
		}
	}
}

// Ready reports whether a successful reload has completed.
func (r *ReadyReloader) Ready() bool {
	// At least one virtual gateway must be ready
	var anyReady bool
	r.WithLock(func() {
		for virtualgw, ready := range r.ready {
			// Only consider gateways at or above the configured separator depth.
			// Deeper virtual gateways may represent partial config fragments.
			if strings.Count(virtualgw, compiler.VIRTUALGW_SEP) <= r.maxGatewayDepth {
				if ready {
					anyReady = true
					return
				}
			}
		}
	})
	return anyReady
}

// SetDispatcher injects the dispatcher dependency.
func (r *ReadyManager) SetDispatcher(d Dispatcher) {
	r.realDispatcher = d
}

// NewReadyManager builds a refresh manager around the reloader.
// The dispatcher cannot be provided yet because the readyManager
// is actually a parameter in the Dispatcher's constructor
// (it becomes the Reloader that the dispatcher uses).
// So we need to add the dispatcher afterwards.
// amazonq-ignore-next-line
func NewReadyManager(reloader Reloader, scheduler Scheduler, maxGatewayDepth int) *ReadyManager {
	r := &ReadyManager{
		realScheduler: scheduler,
		ReadyReloader: NewReadyReloader(reloader, maxGatewayDepth),
	}
	r.active.Store(false)
	return r
}
