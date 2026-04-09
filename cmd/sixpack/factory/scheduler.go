package factory

import (
	"context"
	"sync"
)

type Scheduler struct {
	sync.Mutex
}

// NewScheduler creates a mutex-backed scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{}
}

// Must runs task while holding the scheduler lock, blocking until available.
func (s *Scheduler) Must(ctx context.Context, task func()) {
	s.Lock()
	defer s.Unlock()
	task()
}

// Might runs task only if the lock is immediately available.
func (s *Scheduler) Might(ctx context.Context, task func()) (executed bool) {
	if !s.TryLock() {
		return false
	}
	defer s.Unlock()
	task()
	return true
}
