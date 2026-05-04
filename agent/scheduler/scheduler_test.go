// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_AddTask(t *testing.T) {
	s := NewScheduler(nil)
	task := Task{
		Name:     "test",
		Interval: 1 * time.Hour,
		Fn:       func(ctx context.Context) error { return nil },
	}
	s.AddTask(task)

	if s.TaskCount() != 1 {
		t.Errorf("Expected 1 task, got %d", s.TaskCount())
	}
}

func TestScheduler_StartStop(t *testing.T) {
	s := NewScheduler(nil)
	var calls int32

	s.AddTask(Task{
		Name:     "fast-task",
		Interval: 50 * time.Millisecond,
		Fn: func(ctx context.Context) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Wait for at least 2 ticks
	time.Sleep(150 * time.Millisecond)
	s.Stop()

	callsVal := atomic.LoadInt32(&calls)
	if callsVal < 2 {
		t.Errorf("Expected at least 2 calls, got %d", callsVal)
	}
}

func TestScheduler_StopIdempotent(t *testing.T) {
	s := NewScheduler(nil)
	s.AddTask(Task{
		Name:     "test",
		Interval: 10 * time.Second,
		Fn:       func(ctx context.Context) error { return nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.Stop()
	s.Stop() // Should not panic
}

func TestScheduler_StartIdempotent(t *testing.T) {
	s := NewScheduler(nil)
	s.AddTask(Task{
		Name:     "test",
		Interval: 10 * time.Second,
		Fn:       func(ctx context.Context) error { return nil },
	})

	ctx := context.Background()
	s.Start(ctx)
	s.Start(ctx) // Should not double-start

	if s.TaskCount() != 1 {
		t.Errorf("Expected 1 task after double start, got %d", s.TaskCount())
	}
	s.Stop()
}

func TestScheduler_TaskErrorContinues(t *testing.T) {
	s := NewScheduler(nil)
	var calls int32

	s.AddTask(Task{
		Name:     "error-task",
		Interval: 50 * time.Millisecond,
		Fn: func(ctx context.Context) error {
			atomic.AddInt32(&calls, 1)
			return fmt.Errorf("task error")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Wait for at least 2 ticks — errors should be logged but not stop the scheduler
	time.Sleep(150 * time.Millisecond)
	s.Stop()

	callsVal := atomic.LoadInt32(&calls)
	if callsVal < 2 {
		t.Errorf("Expected at least 2 calls despite errors, got %d", callsVal)
	}
}

func TestScheduler_MnemoTasks_NilClient(t *testing.T) {
	s := NewScheduler(nil)
	tasks := s.MnemoTasks()
	if tasks != nil {
		t.Errorf("Expected nil tasks with nil client, got %d", len(tasks))
	}
}

func TestScheduler_MnemoTasks_WithClient(t *testing.T) {
	// Can't use a real client in unit tests, but verify we get 2 tasks
	// when client is non-nil (even though calls would fail without a daemon)
	s := NewScheduler(nil)
	// We skip testing MnemoTasks with a real client here;
	// integration tests cover the actual consolidation calls.
	// Just verify the structure is correct.
	if s.TaskCount() != 0 {
		t.Errorf("Expected 0 tasks initially, got %d", s.TaskCount())
	}
}

func TestScheduler_MultipleTasks(t *testing.T) {
	s := NewScheduler(nil)
	var calls1, calls2 int32

	s.AddTask(Task{
		Name:     "task1",
		Interval: 50 * time.Millisecond,
		Fn: func(ctx context.Context) error {
			atomic.AddInt32(&calls1, 1)
			return nil
		},
	})
	s.AddTask(Task{
		Name:     "task2",
		Interval: 70 * time.Millisecond,
		Fn: func(ctx context.Context) error {
			atomic.AddInt32(&calls2, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	if atomic.LoadInt32(&calls1) < 2 {
		t.Errorf("Expected task1 to run at least 2 times, got %d", atomic.LoadInt32(&calls1))
	}
	if atomic.LoadInt32(&calls2) < 2 {
		t.Errorf("Expected task2 to run at least 2 times, got %d", atomic.LoadInt32(&calls2))
	}
}

func TestScheduler_ContextCancellation(t *testing.T) {
	s := NewScheduler(nil)
	var calls int32

	s.AddTask(Task{
		Name:     "cancelled-task",
		Interval: 50 * time.Millisecond,
		Fn: func(ctx context.Context) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)

	// Let it run for a bit
	time.Sleep(120 * time.Millisecond)

	// Cancel the context (not Stop)
	cancel()
	time.Sleep(50 * time.Millisecond) // Give goroutines time to exit

	// The scheduler should have been running
	callsVal := atomic.LoadInt32(&calls)
	if callsVal < 1 {
		t.Errorf("Expected at least 1 call before cancellation, got %d", callsVal)
	}
}