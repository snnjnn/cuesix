package factory

import (
	"context"
	"sync"
)

type Scheduler struct {
	sync.Mutex
}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Must(ctx context.Context, task func()) {
	s.Lock()
	defer s.Unlock()
	task()
}

func (s *Scheduler) Might(ctx context.Context, task func()) (executed bool) {
	if !s.TryLock() {
		return false
	}
	defer s.Unlock()
	task()
	return true
}
