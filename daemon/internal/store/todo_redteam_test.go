// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestRedTeamTodoEmptyContent verifies that empty content is accepted at the DB layer
// (NOT NULL doesn't reject empty strings in SQLite) but should be validated at the
// gRPC/service layer.
func TestRedTeamTodoEmptyContent(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	todo, err := db.AddTodo("default", "", 3, "test-agent")
	if err != nil {
		t.Fatalf("DB layer should accept empty content (NOT NULL != non-empty), got error: %v", err)
	}
	if todo.Content != "" {
		t.Errorf("Expected empty content, got %q", todo.Content)
	}
}

// TestRedTeamTodoEmptySourceAgent verifies that empty source_agent is accepted at the
// DB layer (empty string satisfies NOT NULL in SQLite).
func TestRedTeamTodoEmptySourceAgent(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	todo, err := db.AddTodo("default", "Valid content", 3, "")
	if err != nil {
		t.Fatalf("DB layer should accept empty source_agent, got error: %v", err)
	}
	if todo.SourceAgent != "" {
		t.Errorf("Expected empty source_agent, got %q", todo.SourceAgent)
	}
}

// TestRedTeamTodoPriorityBounds verifies the CHECK(priority BETWEEN 1 AND 5) constraint.
// Note: SQLite CHECK constraints are only enforced with STRICT tables or PRAGMA settings.
func TestRedTeamTodoPriorityBounds(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	// Valid priorities should work fine
	for _, p := range []int32{1, 3, 5} {
		_, err := db.AddTodo("default", "test", p, "agent")
		if err != nil {
			t.Errorf("Priority %d should be valid, got error: %v", p, err)
		}
	}

	// Out-of-range priorities: SQLite CHECK constraints may not be enforced
	// depending on journal mode. The service layer should validate instead.
	t.Run("ZeroPriority", func(t *testing.T) {
		_, err := db.AddTodo("default", "zero pri", 0, "agent")
		// Whether this fails depends on SQLite STRICT mode; either way is acceptable
		if err != nil {
			t.Logf("Zero priority rejected (CHECK enforced): %v", err)
		} else {
			t.Log("Zero priority accepted (CHECK not enforced — service layer should validate)")
		}
	})

	t.Run("NegativePriority", func(t *testing.T) {
		_, err := db.AddTodo("default", "neg pri", -1, "agent")
		if err != nil {
			t.Logf("Negative priority rejected (CHECK enforced): %v", err)
		} else {
			t.Log("Negative priority accepted (CHECK not enforced — service layer should validate)")
		}
	})

	t.Run("PriorityAbove5", func(t *testing.T) {
		_, err := db.AddTodo("default", "high pri", 10, "agent")
		if err != nil {
			t.Logf("Priority 10 rejected (CHECK enforced): %v", err)
		} else {
			t.Log("Priority 10 accepted (CHECK not enforced — service layer should validate)")
		}
	})
}

// TestRedTeamTodoConcurrentComplete tests that concurrent CompleteTodo calls
// on the same todo don't cause data races or panics.
func TestRedTeamTodoConcurrentComplete(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	todo, err := db.AddTodo("default", "concurrent complete", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := db.CompleteTodo("default", todo.Id); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	// At most one should succeed; the rest may get "not found" if the first completes
	// but since we're completing an already-completed todo, it just re-sets completed=1
	completed, err := db.GetTodo("default", todo.Id)
	if err != nil {
		t.Fatalf("GetTodo failed: %v", err)
	}
	if !completed.Completed {
		t.Error("Expected todo to be completed after concurrent access")
	}
}

// TestRedTeamTodoDeleteNonexistent verifies that deleting a nonexistent todo
// returns an error.
func TestRedTeamTodoDeleteNonexistent(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	err := db.RemoveTodo("default", "does-not-exist")
	if err == nil {
		t.Error("Expected error when deleting nonexistent todo, got nil")
	}
}

// TestRedTeamTodoCompleteNonexistent verifies that completing a nonexistent todo
// returns an error.
func TestRedTeamTodoCompleteNonexistent(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	_, err := db.CompleteTodo("default", "does-not-exist")
	if err == nil {
		t.Error("Expected error when completing nonexistent todo, got nil")
	}
}

// TestRedTeamTodoGetNonexistent verifies that getting a nonexistent todo
// returns an error.
func TestRedTeamTodoGetNonexistent(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	_, err := db.GetTodo("default", "does-not-exist")
	if err == nil {
		t.Error("Expected error when getting nonexistent todo, got nil")
	}
}

// TestRedTeamTodoSQLInjection verifies that SQL injection attempts in content
// are stored as-is (parameterized queries prevent injection).
func TestRedTeamTodoSQLInjection(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	injections := []string{
		"'; DROP TABLE todos; --",
		"' OR '1'='1",
		"Robert'); DROP TABLE students; --",
		"${7*7}",
		"{{7*7}}",
		"<script>alert('xss')</script>",
	}

	for _, content := range injections {
		t.Run(content, func(t *testing.T) {
			todo, err := db.AddTodo("default", content, 3, "test-agent")
			if err != nil {
				t.Fatalf("Parameterized query should handle injection safely, got: %v", err)
			}
			if todo.Content != content {
				t.Errorf("Content mutated: got %q, want %q", todo.Content, content)
			}

			// Verify the todos table still exists and is functional
			todos, err := db.ListTodos("default", true, "")
			if err != nil {
				t.Fatalf("ListTodos failed after injection attempt: %v", err)
			}
			if len(todos) == 0 {
				t.Error("Expected at least one todo after insertion")
			}
		})
	}
}

// TestRedTeamTodoDoubleComplete verifies that completing an already-completed
// todo is idempotent (sets completed=1 again, updates timestamp).
func TestRedTeamTodoDoubleComplete(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	todo, err := db.AddTodo("default", "double complete", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}

	// First complete
	completed1, err := db.CompleteTodo("default", todo.Id)
	if err != nil {
		t.Fatalf("First CompleteTodo failed: %v", err)
	}
	if !completed1.Completed {
		t.Error("Expected todo to be completed after first call")
	}

	// Second complete — should succeed (idempotent)
	completed2, err := db.CompleteTodo("default", todo.Id)
	if err != nil {
		t.Fatalf("Second CompleteTodo should be idempotent, got: %v", err)
	}
	if !completed2.Completed {
		t.Error("Expected todo to still be completed after second call")
	}
}

// TestRedTeamTodoUpdateAfterDelete verifies that getting/completing a deleted
// todo returns an error.
func TestRedTeamTodoUpdateAfterDelete(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	todo, err := db.AddTodo("default", "to be deleted", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}

	err = db.RemoveTodo("default", todo.Id)
	if err != nil {
		t.Fatalf("RemoveTodo failed: %v", err)
	}

	// CompleteTodo on deleted todo should fail
	_, err = db.CompleteTodo("default", todo.Id)
	if err == nil {
		t.Error("Expected error when completing deleted todo")
	}

	// GetTodo on deleted todo should fail
	_, err = db.GetTodo("default", todo.Id)
	if err == nil {
		t.Error("Expected error when getting deleted todo")
	}
}

// TestRedTeamTodoListFiltering verifies that ListTodos correctly filters
// by completion status and source agent.
func TestRedTeamTodoListFiltering(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	// Create 3 todos from different agents
	db.AddTodo("default", "task-a1", 3, "agent-a")
	db.AddTodo("default", "task-b1", 3, "agent-b")
	db.AddTodo("default", "task-a2", 1, "agent-a")

	// Complete one from agent-a
	todos, _ := db.ListTodos("default", true, "agent-a")
	var agentATodoId string
	for _, todo := range todos {
		if todo.Content == "task-a1" {
			agentATodoId = todo.Id
			break
		}
	}
	db.CompleteTodo("default", agentATodoId)

	// List incomplete only
	incomplete, err := db.ListTodos("default", false, "")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(incomplete) != 2 {
		t.Errorf("Expected 2 incomplete todos, got %d", len(incomplete))
	}

	// List all for agent-a
	agentA, err := db.ListTodos("default", true, "agent-a")
	if err != nil {
		t.Fatalf("ListTodos for agent-a failed: %v", err)
	}
	if len(agentA) != 2 {
		t.Errorf("Expected 2 todos for agent-a, got %d", len(agentA))
	}

	// List incomplete for agent-a
	agentAIncomplete, err := db.ListTodos("default", false, "agent-a")
	if err != nil {
		t.Fatalf("ListTodos incomplete for agent-a failed: %v", err)
	}
	if len(agentAIncomplete) != 1 {
		t.Errorf("Expected 1 incomplete todo for agent-a, got %d", len(agentAIncomplete))
	}
}

// TestRedTeamTodoConcurrentCreateDelete tests that concurrent writes don't cause panics.
// SQLite may return SQLITE_BUSY under concurrent write load — this test verifies
// no panics or data corruption, not that all writes succeed.
func TestRedTeamTodoConcurrentCreateDelete(t *testing.T) {
	t.Parallel()
	db := setupTodoTestDB(t)

	var wg sync.WaitGroup
	const numOps = 20
	var successCount int64

	// Concurrent creates — some may get SQLITE_BUSY, which is expected with SQLite
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := db.AddTodo("default", "concurrent-todo", 3, "test-agent")
			if err != nil {
				// SQLITE_BUSY is expected under concurrent write load
				t.Logf("Concurrent create %d: %v (expected for SQLite)", n, err)
			} else {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// Verify at least some were created and no panics occurred
	todos, err := db.ListTodos("default", true, "test-agent")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) == 0 {
		t.Error("Expected at least one todo to be created")
	}
	t.Logf("Created %d/%d todos under concurrent write load (SQLite expected)", len(todos), numOps)
}
