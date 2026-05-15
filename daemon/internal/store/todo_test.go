// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTodoTestDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestAddTodo(t *testing.T) {
	db := setupTodoTestDB(t)
	defer db.Close()

	todo, err := db.AddTodo("default", "Test todo item", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}
	if todo.Id == "" {
		t.Error("Expected todo ID to be set")
	}
	if todo.Content != "Test todo item" {
		t.Errorf("Expected content 'Test todo item', got %s", todo.Content)
	}
	if todo.Priority != 3 {
		t.Errorf("Expected priority 3, got %d", todo.Priority)
	}
	if todo.Completed {
		t.Error("New todo should not be completed")
	}
	if todo.SourceAgent != "test-agent" {
		t.Errorf("Expected source_agent 'test-agent', got %s", todo.SourceAgent)
	}
	if todo.CreatedAt == 0 {
		t.Error("Expected created_at to be set")
	}
	if todo.CompletedAt != 0 {
		t.Error("New todo should have completed_at = 0")
	}
}

func TestListTodos(t *testing.T) {
	db := setupTodoTestDB(t)
	defer db.Close()

	// Add some todos
	_, err := db.AddTodo("default", "First todo", 5, "agent-1")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}
	_, err = db.AddTodo("default", "Second todo", 3, "agent-1")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}

	// New todos should show as incomplete by default
	todos, err := db.ListTodos("default", true, "")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) != 2 {
		t.Errorf("Expected 2 todos (include completed), got %d", len(todos))
	}

	// Filter by agent
	todos, err = db.ListTodos("default", true, "agent-1")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) != 2 {
		t.Errorf("Expected 2 todos for agent-1, got %d", len(todos))
	}

	// Filter by wrong agent
	todos, err = db.ListTodos("default", true, "wrong-agent")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) != 0 {
		t.Errorf("Expected 0 todos for wrong agent, got %d", len(todos))
	}
}

func TestCompleteTodo(t *testing.T) {
	db := setupTodoTestDB(t)
	defer db.Close()

	todo, err := db.AddTodo("default", "Complete me", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}

	// Complete it
	completed, err := db.CompleteTodo("default", todo.Id)
	if err != nil {
		t.Fatalf("CompleteTodo failed: %v", err)
	}
	if !completed.Completed {
		t.Error("Expected todo to be completed")
	}
	if completed.CompletedAt == 0 {
		t.Error("Expected completed_at to be set")
	}

	// Verify it shows as completed
	incomplete, _ := db.ListTodos("default", false, "")
	if len(incomplete) != 0 {
		t.Errorf("Expected 0 incomplete todos, got %d", len(incomplete))
	}

	complete, _ := db.ListTodos("default", true, "")
	if len(complete) != 1 {
		t.Errorf("Expected 1 total todo, got %d", len(complete))
	}
}

func TestCompleteTodoNotFound(t *testing.T) {
	db := setupTodoTestDB(t)
	defer db.Close()

	_, err := db.CompleteTodo("default", "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent todo ID")
	}
}

func TestRemoveTodo(t *testing.T) {
	db := setupTodoTestDB(t)
	defer db.Close()

	todo, err := db.AddTodo("default", "Remove me", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}

	err = db.RemoveTodo("default", todo.Id)
	if err != nil {
		t.Fatalf("RemoveTodo failed: %v", err)
	}

	// Verify it's gone
	todos, _ := db.ListTodos("default", true, "")
	if len(todos) != 0 {
		t.Errorf("Expected 0 todos after removal, got %d", len(todos))
	}
}

func TestRemoveTodoNotFound(t *testing.T) {
	db := setupTodoTestDB(t)
	defer db.Close()

	err := db.RemoveTodo("default", "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent todo ID")
	}
}

func TestTodoPriorityOrdering(t *testing.T) {
	db := setupTodoTestDB(t)
	defer db.Close()

	// Add todos with different priorities
	db.AddTodo("default", "Low priority", 1, "agent-1")
	db.AddTodo("default", "High priority", 5, "agent-1")
	db.AddTodo("default", "Medium priority", 3, "agent-1")

	// List should be ordered by priority DESC
	todos, err := db.ListTodos("default", true, "")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) != 3 {
		t.Fatalf("Expected 3 todos, got %d", len(todos))
	}
	if todos[0].Priority != 5 {
		t.Errorf("Expected first todo to have priority 5, got %d", todos[0].Priority)
	}
	if todos[1].Priority != 3 {
		t.Errorf("Expected second todo to have priority 3, got %d", todos[1].Priority)
	}
	if todos[2].Priority != 1 {
		t.Errorf("Expected third todo to have priority 1, got %d", todos[2].Priority)
	}
}

func TestTodoDefaultPriority(t *testing.T) {
	db := setupTodoTestDB(t)
	defer db.Close()

	// Add a todo with priority 3 (minimum valid priority)
	todo, err := db.AddTodo("default", "Default priority todo", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}
	if todo.Priority != 3 {
		t.Errorf("Expected priority 3, got %d", todo.Priority)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
