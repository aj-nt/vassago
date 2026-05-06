// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	memoclient "github.com/aj-nt/mnemosyne/client"
	pb "github.com/aj-nt/mnemosyne/proto"
)

// Task is a scheduled periodic task.
type Task struct {
	Name     string        // Human-readable name for logging
	Interval time.Duration // How often to run
	Fn       TaskFunc      // The function to execute
}

// TaskFunc is the signature for scheduled task functions.
type TaskFunc func(ctx context.Context) error

// Scheduler runs periodic tasks at configured intervals.
type Scheduler struct {
	tasks     []Task
	mnemo memoclient.MnemoClient
	mu        sync.Mutex
	running   bool
	cancellers []context.CancelFunc
}

// NewScheduler creates a new scheduler with the daemon client.
func NewScheduler(mnemo memoclient.MnemoClient) *Scheduler {
	return &Scheduler{
		mnemo: mnemo,
	}
}

// AddTask registers a periodic task.
func (s *Scheduler) AddTask(task Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, task)
}

// MnemoTasks returns the built-in scheduled tasks for Mnemosyne.
// These use the scheduler's daemon client for consolidation operations.
func (s *Scheduler) MnemoTasks() []Task {
	if s.mnemo == nil {
		return nil
	}

	return []Task{
		{
			Name:     "consolidate-observations",
			Interval: 1 * time.Hour,
			Fn: func(ctx context.Context) error {
				resp, err := s.mnemo.Consolidate(ctx, pb.ConsolidateScope_CONSOLIDATE_SCOPE_OBSERVATIONS, 50)
				if err != nil {
					return fmt.Errorf("consolidate observations: %w", err)
				}
				if resp.Removed > 0 || resp.Merged > 0 {
					log.Printf("Scheduler: consolidated observations: removed=%d merged=%d %s",
						resp.Removed, resp.Merged, resp.Summary)
				}
				return nil
			},
		},
		{
			Name:     "consolidate-expired",
			Interval: 6 * time.Hour,
			Fn: func(ctx context.Context) error {
				resp, err := s.mnemo.Consolidate(ctx, pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED, 100)
				if err != nil {
					return fmt.Errorf("consolidate expired: %w", err)
				}
				if resp.Removed > 0 || resp.Merged > 0 {
					log.Printf("Scheduler: consolidated expired: removed=%d merged=%d %s",
						resp.Removed, resp.Merged, resp.Summary)
				}
				return nil
			},
		},
	}
}

// Start begins running all scheduled tasks. Each task runs in its own goroutine
// at the configured interval. The first run happens after the interval elapses.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	for _, task := range s.tasks {
		taskCtx, cancel := context.WithCancel(ctx)
		s.mu.Lock()
		s.cancellers = append(s.cancellers, cancel)
		s.mu.Unlock()

		go s.runTask(taskCtx, task)
		log.Printf("Scheduler: started task %q (interval: %s)", task.Name, task.Interval)
	}
}

// Stop cancels all running tasks and waits for them to finish.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cancel := range s.cancellers {
		cancel()
	}
	s.cancellers = nil
	s.running = false
	log.Printf("Scheduler: stopped all tasks")
}

// TaskCount returns the number of registered tasks.
func (s *Scheduler) TaskCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tasks)
}

// runTask executes a task at its configured interval.
func (s *Scheduler) runTask(ctx context.Context, task Task) {
	ticker := time.NewTicker(task.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := task.Fn(ctx); err != nil {
				log.Printf("Scheduler: task %q failed: %v", task.Name, err)
			} else {
				log.Printf("Scheduler: task %q completed", task.Name)
			}
		}
	}
}