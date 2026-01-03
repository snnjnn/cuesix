package factory

import (
	"context"
	"sync"
)

type Scheduler struct {
	token sync.Mutex
}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Must(ctx context.Context, task func()) {
	s.token.Lock()
	defer s.token.Unlock()
	task()
}

func (s *Scheduler) Might(ctx context.Context, task func()) (executed bool) {
	if !s.token.TryLock() {
		return false
	}
	defer s.token.Unlock()
	task()
	return true
}
