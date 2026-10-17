// This file is part of Vassago.
// See LICENSE for license information.

package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTaskLifecycle(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	task := &TaskEntry{
		ID: "test-task-1", AgentType: "static_analysis",
		Goal: "Run golangci-lint", Context: `{"repo":"/test"}`,
		Priority: 3, Status: TaskReady, TTLSeconds: 60,
		MaxRetries: 2, CreatedAt: time.Now().Unix(),
	}
	entry, err := s.AddTask(task)
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if entry.ID != "test-task-1" {
		t.Errorf("ID = %q, want test-task-1", entry.ID)
	}
	if entry.Status != TaskReady {
		t.Errorf("status = %s, want ready", entry.Status)
	}

	got, err := s.GetTask("test-task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got == nil {
		t.Fatal("GetTask returned nil")
	}
	if got.Goal != "Run golangci-lint" {
		t.Errorf("goal = %q", got.Goal)
	}
}

func TestClaimTaskAtomic(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	_, err := s.AddTask(&TaskEntry{
		ID: "race-task", AgentType: "builder",
		Goal: "race test", Priority: 3, Status: TaskReady,
		TTLSeconds: 60, MaxRetries: 2, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	claimed, err := s.ClaimTask("race-task", "worker-a")
	if err != nil {
		t.Fatalf("ClaimTask(worker-a): %v", err)
	}
	if claimed == nil {
		t.Fatal("First claim should succeed")
	}
	if claimed.AssignedAgent != "worker-a" {
		t.Errorf("assigned_agent = %q, want worker-a", claimed.AssignedAgent)
	}
	if claimed.Status != TaskClaimed {
		t.Errorf("status = %s, want claimed", claimed.Status)
	}

	second, err := s.ClaimTask("race-task", "worker-b")
	if err != nil {
		t.Fatalf("ClaimTask(worker-b): %v", err)
	}
	if second != nil {
		t.Error("Second claim should return nil (already claimed)")
	}
}

func TestClaimTaskNonexistent(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	claimed, err := s.ClaimTask("no-such-task", "worker-c")
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	if claimed != nil {
		t.Error("Claiming nonexistent task should return nil")
	}
}

func TestCompleteTask(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	_, err := s.AddTask(&TaskEntry{
		ID: "comp-task", AgentType: "scanner",
		Goal: "scan", Priority: 3, Status: TaskReady,
		TTLSeconds: 60, MaxRetries: 2, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	_, err = s.ClaimTask("comp-task", "worker-c")
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	done, err := s.CompleteTask("comp-task", "result-key-123")
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	if done == nil {
		t.Fatal("CompleteTask returned nil")
	}
	if done.Status != TaskDone {
		t.Errorf("status = %s, want done", done.Status)
	}
	if done.ResultKey != "result-key-123" {
		t.Errorf("result_key = %q", done.ResultKey)
	}
	if done.CompletedAt == 0 {
		t.Error("completed_at should be set")
	}
}

func TestFailTaskRetry(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	_, err := s.AddTask(&TaskEntry{
		ID: "fail-task", AgentType: "builder",
		Goal: "test fail", Priority: 3, Status: TaskReady,
		TTLSeconds: 60, MaxRetries: 2, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	_, err = s.ClaimTask("fail-task", "worker-f")
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	failed, err := s.FailTask("fail-task")
	if err != nil {
		t.Fatalf("FailTask: %v", err)
	}
	if failed == nil {
		t.Fatal("FailTask returned nil")
	}
	if failed.Status != TaskReady {
		t.Errorf("status = %s, want ready (retry)", failed.Status)
	}
	if failed.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", failed.RetryCount)
	}
	if failed.AssignedAgent != "" {
		t.Errorf("assigned_agent should be cleared on retry, got %q", failed.AssignedAgent)
	}

	_, err = s.ClaimTask("fail-task", "worker-f2")
	if err != nil {
		t.Fatalf("ClaimTask (retry): %v", err)
	}
	failed2, err := s.FailTask("fail-task")
	if err != nil {
		t.Fatalf("FailTask (dead_letter): %v", err)
	}
	if failed2.Status != TaskDeadLetter {
		t.Errorf("status = %s, want dead_letter", failed2.Status)
	}
	if failed2.RetryCount != 2 {
		t.Errorf("retry_count = %d, want 2", failed2.RetryCount)
	}
}

func TestFindReadyTasks(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	now := time.Now().Unix()
	add := func(id, at string, prio int32) {
		_, err := s.AddTask(&TaskEntry{
			ID: id, AgentType: at, Goal: "test",
			Priority: prio, Status: TaskReady,
			TTLSeconds: 60, MaxRetries: 2, CreatedAt: now,
		})
		if err != nil {
			t.Fatalf("AddTask(%s): %v", id, err)
		}
	}
	add("t1", "static_analysis", 3)
	add("t2", "quality_scan", 5)
	add("t3", "static_analysis", 4)

	_, err := s.ClaimTask("t2", "worker")
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	tasks, err := s.FindReadyTasks("static_analysis", 10)
	if err != nil {
		t.Fatalf("FindReadyTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 ready static_analysis tasks, got %d", len(tasks))
	}
	if tasks[0].Priority < tasks[1].Priority {
		t.Error("tasks should be ordered by priority DESC")
	}
	if tasks[0].ID != "t3" || tasks[1].ID != "t1" {
		t.Errorf("order wrong: %v, %v", tasks[0].ID, tasks[1].ID)
	}
}

func TestFindReadyTasksEmpty(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	tasks, err := s.FindReadyTasks("no-matching", 10)
	if err != nil {
		t.Fatalf("FindReadyTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestResetTimedOutTasks(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	past := time.Now().Unix() - 120

	_, err := s.AddTask(&TaskEntry{
		ID: "timeout-1", AgentType: "worker",
		Goal: "timeout test", Priority: 3, Status: TaskReady,
		TTLSeconds: 60, MaxRetries: 2, CreatedAt: past,
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	_, err = s.ClaimTask("timeout-1", "dead-worker")
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	s.writeMu.Lock()
	_, err = s.db.Exec("UPDATE tasks SET claimed_at = ? WHERE id = ?", past, "timeout-1")
	s.writeMu.Unlock()
	if err != nil {
		t.Fatalf("set old claimed_at: %v", err)
	}

	ids, err := s.ResetTimedOutTasks()
	if err != nil {
		t.Fatalf("ResetTimedOutTasks: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 reset, got %d", len(ids))
	}
	if ids[0] != "timeout-1" {
		t.Errorf("expected timeout-1, got %s", ids[0])
	}

	task, err := s.GetTask("timeout-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != TaskReady {
		t.Errorf("status = %s, want ready", task.Status)
	}
	if task.AssignedAgent != "" {
		t.Errorf("assigned_agent should be cleared, got %q", task.AssignedAgent)
	}
	if task.ClaimedAt != 0 {
		t.Errorf("claimed_at should be reset to 0, got %d", task.ClaimedAt)
	}
}

func TestValidateTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		task  *TaskEntry
		valid bool
	}{
		{"valid", &TaskEntry{ID: "t1", AgentType: "worker", Goal: "test", Priority: 3, TTLSeconds: 300, Status: TaskReady}, true},
		{"no_id", &TaskEntry{AgentType: "worker", Goal: "test", Priority: 3, TTLSeconds: 300}, false},
		{"no_goal", &TaskEntry{ID: "t2", AgentType: "worker", Priority: 3, TTLSeconds: 300}, false},
		{"no_agent_type", &TaskEntry{ID: "t3", Goal: "test", Priority: 3, TTLSeconds: 300}, false},
		{"bad_status", &TaskEntry{ID: "t4", AgentType: "w", Goal: "t", Priority: 3, Status: "bogus", TTLSeconds: 300}, false},
		{"default_ttl", &TaskEntry{ID: "t5", AgentType: "w", Goal: "t", Priority: 3}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.task.Validate()
			if tt.valid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestDeleteTask(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	_, err := s.AddTask(&TaskEntry{
		ID: "del-me", AgentType: "test",
		Goal: "test", Priority: 3, Status: TaskReady,
		TTLSeconds: 60, MaxRetries: 2, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	deleted, err := s.DeleteTask("del-me")
	if err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if !deleted {
		t.Error("DeleteTask should return true")
	}

	got, err := s.GetTask("del-me")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got != nil {
		t.Error("GetTask should return nil after delete")
	}
}

func TestDeleteNonexistentTask(t *testing.T) {
	t.Parallel()
	s := newTaskTestStore(t)
	defer s.Close()

	deleted, err := s.DeleteTask("no-such")
	if err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if deleted {
		t.Error("DeleteTask should return false for nonexistent")
	}
}

func newTaskTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}
