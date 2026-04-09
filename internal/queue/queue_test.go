package queue

import (
	"container/heap"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
)

func newTestQueue() *Queue {
	return New(func() backoff.BackOff {
		return backoff.NewConstantBackOff(time.Millisecond)
	})
}

func pushTestTasks(q *Queue, tasks ...TaskRetry) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, task := range tasks {
		heap.Push(&q.tasks, task)
	}
}

func queueLen(q *Queue) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

func TestQueuePopOrder(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, q *Queue)
		want  []string
	}{
		{
			name: "orders by deadline then priority then id",
			setup: func(t *testing.T, q *Queue) {
				t.Helper()
				now := time.Now()
				pushTestTasks(q,
					TaskRetry{ID: "b", Priority: 1, Deadline: now},
					TaskRetry{ID: "a", Priority: 1, Deadline: now},
					TaskRetry{ID: "high", Priority: 10, Deadline: now},
					TaskRetry{ID: "later", Priority: 100, Deadline: now.Add(10 * time.Millisecond)},
				)
			},
			want: []string{"high", "a", "b", "later"},
		},
		{
			name: "earlier deadline beats higher priority",
			setup: func(t *testing.T, q *Queue) {
				t.Helper()
				now := time.Now()
				pushTestTasks(q,
					TaskRetry{ID: "urgent-late", Priority: 100, Deadline: now.Add(10 * time.Millisecond)},
					TaskRetry{ID: "sooner", Priority: 1, Deadline: now},
				)
			},
			want: []string{"sooner", "urgent-late"},
		},
		{
			name: "higher priority wins on same deadline",
			setup: func(t *testing.T, q *Queue) {
				t.Helper()
				now := time.Now()
				pushTestTasks(q,
					TaskRetry{ID: "low", Priority: 1, Deadline: now},
					TaskRetry{ID: "high", Priority: 2, Deadline: now},
					TaskRetry{ID: "mid", Priority: 1, Deadline: now},
				)
			},
			want: []string{"high", "low", "mid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newTestQueue()
			tt.setup(t, q)

			ctx := context.Background()
			for i, want := range tt.want {
				got, err := q.Pop(ctx)
				if err != nil {
					t.Fatalf("pop %d: %v", i, err)
				}
				if got.ID != want {
					t.Fatalf("pop %d: got %q want %q", i, got.ID, want)
				}
			}
		})
	}
}

func TestQueuePopWakeupAndTermination(t *testing.T) {
	tests := []struct {
		name  string
		run   func(t *testing.T, q *Queue)
		check func(t *testing.T, task TaskRetry, err error, elapsed time.Duration)
	}{
		{
			name: "wakes when earlier task arrives",
			run: func(t *testing.T, q *Queue) {
				t.Helper()
				if err := q.Batch([]string{"late"}, 1, 200*time.Millisecond); err != nil {
					t.Fatalf("batch late: %v", err)
				}
				time.Sleep(20 * time.Millisecond)
				if err := q.Batch([]string{"early"}, 1, 30*time.Millisecond); err != nil {
					t.Fatalf("batch early: %v", err)
				}
			},
			check: func(t *testing.T, task TaskRetry, err error, elapsed time.Duration) {
				t.Helper()
				if err != nil {
					t.Fatalf("pop failed: %v", err)
				}
				if task.ID != "early" {
					t.Fatalf("got %q want early", task.ID)
				}
				if elapsed >= 150*time.Millisecond {
					t.Fatalf("pop returned too late: %v", elapsed)
				}
			},
		},
		{
			name: "returns context cancellation while idle",
			run: func(t *testing.T, q *Queue) {
				t.Helper()
			},
			check: func(t *testing.T, task TaskRetry, err error, elapsed time.Duration) {
				t.Helper()
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("got err %v want %v", err, context.DeadlineExceeded)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newTestQueue()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			if tt.name == "returns context cancellation while idle" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
			}
			defer cancel()

			taskCh := make(chan TaskRetry, 1)
			errCh := make(chan error, 1)
			start := time.Now()

			go func() {
				task, err := q.Pop(ctx)
				if err != nil {
					errCh <- err
					return
				}
				taskCh <- task
			}()

			tt.run(t, q)

			select {
			case task := <-taskCh:
				tt.check(t, task, nil, time.Since(start))
			case err := <-errCh:
				tt.check(t, TaskRetry{}, err, time.Since(start))
			case <-ctx.Done():
				select {
				case err := <-errCh:
					tt.check(t, TaskRetry{}, err, time.Since(start))
				case <-time.After(50 * time.Millisecond):
					t.Fatalf("timed out waiting for pop result: %v", ctx.Err())
				}
			}
		})
	}
}

func TestQueueCleanup(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, q *Queue)
		wantLen     int
		doubleClean bool
	}{
		{
			name: "removes all pending tasks",
			setup: func(t *testing.T, q *Queue) {
				t.Helper()
				if err := q.Batch([]string{"keep-1", "drop-1", "keep-2", "drop-2"}, 1, time.Second); err != nil {
					t.Fatalf("batch: %v", err)
				}
			},
		},
		{
			name:    "empty queue is a no-op",
			wantLen: 0,
		},
		{
			name: "cleanup can be called repeatedly",
			setup: func(t *testing.T, q *Queue) {
				t.Helper()
				if err := q.Batch([]string{"a", "b"}, 1, time.Second); err != nil {
					t.Fatalf("batch: %v", err)
				}
			},
			doubleClean: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newTestQueue()
			if tt.setup != nil {
				tt.setup(t, q)
			}

			q.Cleanup()
			if got := queueLen(q); got != tt.wantLen {
				t.Fatalf("len after cleanup = %d want %d", got, tt.wantLen)
			}
			if tt.doubleClean {
				q.Cleanup()
				if got := queueLen(q); got != 0 {
					t.Fatalf("len after second cleanup = %d want 0", got)
				}
			}
		})
	}
}

func TestQueueCleanupClearsPendingTasksForWaitingPop(t *testing.T) {
	q := newTestQueue()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := q.Batch([]string{"first"}, 1, 200*time.Millisecond); err != nil {
		t.Fatalf("batch first: %v", err)
	}
	if err := q.Batch([]string{"second"}, 1, 40*time.Millisecond); err != nil {
		t.Fatalf("batch second: %v", err)
	}

	taskCh := make(chan TaskRetry, 1)
	errCh := make(chan error, 1)
	start := time.Now()

	go func() {
		task, err := q.Pop(ctx)
		if err != nil {
			errCh <- err
			return
		}
		taskCh <- task
	}()

	time.Sleep(20 * time.Millisecond)
	q.Cleanup()

	if err := q.Batch([]string{"replacement"}, 1, 30*time.Millisecond); err != nil {
		t.Fatalf("batch replacement: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("pop failed: %v", err)
	case task := <-taskCh:
		if task.ID != "replacement" {
			t.Fatalf("got %q want replacement", task.ID)
		}
		if elapsed := time.Since(start); elapsed >= 150*time.Millisecond {
			t.Fatalf("pop returned too late: %v", elapsed)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for pop result: %v", ctx.Err())
	}
}

func TestQueueBatch(t *testing.T) {
	tests := []struct {
		name      string
		prepare   func(t *testing.T, q *Queue)
		ids       []string
		priority  int
		delay     time.Duration
		wantLen   int
		wantOrder []string
	}{
		{
			name:      "schedules ids with shared deadline and priority",
			ids:       []string{"c", "a", "b"},
			priority:  5,
			delay:     0,
			wantLen:   3,
			wantOrder: []string{"a", "b", "c"},
		},
		{
			name: "existing earlier task stays ahead of batch",
			prepare: func(t *testing.T, q *Queue) {
				t.Helper()
				pushTestTasks(q, TaskRetry{ID: "earlier", Priority: 1, Deadline: time.Now()})
			},
			ids:       []string{"later-b", "later-a"},
			priority:  10,
			delay:     time.Second,
			wantLen:   3,
			wantOrder: []string{"earlier", "later-a", "later-b"},
		},
		{
			name:     "empty batch is a no-op",
			ids:      nil,
			priority: 1,
			delay:    0,
			wantLen:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newTestQueue()
			if tt.prepare != nil {
				tt.prepare(t, q)
			}

			if err := q.Batch(tt.ids, tt.priority, tt.delay); err != nil {
				t.Fatalf("batch error: %v", err)
			}
			if got := queueLen(q); got != tt.wantLen {
				t.Fatalf("len after batch = %d want %d", got, tt.wantLen)
			}

			ctx := context.Background()
			for i, want := range tt.wantOrder {
				got, err := q.Pop(ctx)
				if err != nil {
					t.Fatalf("pop %d: %v", i, err)
				}
				if got.ID != want {
					t.Fatalf("pop %d: got %q want %q", i, got.ID, want)
				}
			}
		})
	}
}

func TestQueueRetry(t *testing.T) {
	tests := []struct {
		name    string
		queue   *Queue
		task    TaskRetry
		wantErr error
		wantLen int
	}{
		{
			name:    "uses queue factory when backoff is nil",
			queue:   newTestQueue(),
			task:    TaskRetry{ID: "a", Priority: 1},
			wantLen: 1,
		},
		{
			name:    "returns retries exceeded on stop",
			queue:   newTestQueue(),
			task:    TaskRetry{ID: "a", Priority: 1, Backoff: backoff.NewConstantBackOff(backoff.Stop)},
			wantErr: ErrRetriesExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.queue.Retry(tt.task)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("retry error = %v want %v", err, tt.wantErr)
			}
			if got := queueLen(tt.queue); got != tt.wantLen {
				t.Fatalf("len after retry = %d want %d", got, tt.wantLen)
			}
		})
	}
}
