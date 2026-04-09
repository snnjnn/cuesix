package queue

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// Error type for this module
type Error string

func (err Error) Error() string {
	return string(err)
}

const (
	ErrRetriesExceeded = Error("retries exceeded")
)

// TaskRetry is a scheduled retry attempt.
type TaskRetry struct {
	Backoff  backoff.BackOff
	Deadline time.Time
	Priority int
	ID       string
}

// Queue orders tasks by deadline asc, priority desc, and ID asc.
type Queue struct {
	mu      sync.Mutex
	factory func() backoff.BackOff
	tasks   taskHeap
	wakeCh  chan struct{}
}

// New creates an empty priority queue
func New(factory func() backoff.BackOff) *Queue {
	return &Queue{
		factory: factory,
		wakeCh:  make(chan struct{}, 1),
	}
}

// Batch schedules a set of task IDs with the same priority and deadline.
func (q *Queue) Batch(ids []string, priority int, delay time.Duration) error {
	if len(ids) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	deadline := time.Now().Add(delay)
	shouldWake := len(q.tasks) == 0
	for _, id := range ids {
		task := TaskRetry{
			Backoff:  nil,
			Deadline: deadline,
			Priority: priority,
			ID:       id,
		}
		if !shouldWake && lessTask(task, q.tasks[0]) {
			shouldWake = true
		}
		heap.Push(&q.tasks, task)
	}
	if shouldWake {
		q.notifyLocked()
	}
	return nil
}

// Retry schedules a single task for retry
func (q *Queue) Retry(task TaskRetry) error {
	if task.Backoff == nil {
		task.Backoff = q.factory()
		task.Backoff.Reset()
	}
	d := task.Backoff.NextBackOff()
	if d == backoff.Stop {
		return ErrRetriesExceeded
	}
	task.Deadline = time.Now().Add(d)
	q.mu.Lock()
	defer q.mu.Unlock()
	shouldWake := len(q.tasks) == 0 || lessTask(task, q.tasks[0])
	heap.Push(&q.tasks, task)
	if shouldWake {
		q.notifyLocked()
	}
	return nil
}

// Cleanup removes all pending tasks
func (q *Queue) Cleanup() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.tasks) > 0 {
		q.tasks = q.tasks[:0]
		q.notifyLocked()
	}
}

// Pop waits for the next task to be ready for execution.
func (q *Queue) Pop(ctx context.Context) (TaskRetry, error) {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		task, wait, err := q.nextReady()
		if err != nil {
			return TaskRetry{}, err
		}
		if wait <= 0 {
			return task, nil
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(wait)
		select {
		case <-ctx.Done():
			return TaskRetry{}, ctx.Err()
		case <-q.wakeCh:
		case <-timer.C:
		}
	}
}

func (q *Queue) nextReady() (TaskRetry, time.Duration, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.tasks) == 0 {
		return TaskRetry{}, time.Hour, nil
	}
	next := q.tasks[0]
	wait := time.Until(next.Deadline)
	if wait > 0 {
		return TaskRetry{}, wait, nil
	}
	return heap.Pop(&q.tasks).(TaskRetry), 0, nil
}

func (q *Queue) notifyLocked() {
	select {
	case q.wakeCh <- struct{}{}:
	default:
	}
}

type taskHeap []TaskRetry

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool {
	return lessTask(h[i], h[j])
}

func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *taskHeap) Push(x any) {
	*h = append(*h, x.(TaskRetry))
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func lessTask(a, b TaskRetry) bool {
	if !a.Deadline.Equal(b.Deadline) {
		return a.Deadline.Before(b.Deadline)
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	return a.ID < b.ID
}
