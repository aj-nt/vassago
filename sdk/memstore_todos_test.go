// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package sdk

import (
	"context"
	"testing"
)

// --- Todos ---

func TestMemStoreAddTodo(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	todo, err := store.AddTodo(ctx, "Write SDK tests", 5)
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}
	if todo.ID == "" {
		t.Error("AddTodo: ID is empty")
	}
	if todo.Content != "Write SDK tests" {
		t.Errorf("AddTodo: content = %q", todo.Content)
	}
	if todo.Priority != 5 {
		t.Errorf("AddTodo: priority = %d, want 5", todo.Priority)
	}
	if todo.SourceAgent != "test-agent" {
		t.Errorf("AddTodo: SourceAgent = %q, want %q", todo.SourceAgent, "test-agent")
	}
	if todo.Completed {
		t.Error("AddTodo: new todo should not be completed")
	}
}

func TestMemStoreListTodos(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	store.AddTodo(ctx, "task 1", 3)
	t2, _ := store.AddTodo(ctx, "task 2", 1)
	store.AddTodo(ctx, "task 3", 2)

	// All incomplete.
	all, err := store.ListTodos(ctx, false)
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListTodos: got %d, want 3", len(all))
	}

	// Complete task 2.
	_, err = store.CompleteTodo(ctx, t2.ID)
	if err != nil {
		t.Fatalf("CompleteTodo: %v", err)
	}

	// Only incomplete now.
	active, err := store.ListTodos(ctx, false)
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("ListTodos after complete: got %d, want 2", len(active))
	}

	// Include completed.
	all, err = store.ListTodos(ctx, true)
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListTodos(includeCompleted): got %d, want 3", len(all))
	}
}

func TestMemStoreCompleteTodo(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	todo, _ := store.AddTodo(ctx, "finish this", 3)

	result, err := store.CompleteTodo(ctx, todo.ID)
	if err != nil {
		t.Fatalf("CompleteTodo: %v", err)
	}
	if !result.Completed {
		t.Error("CompleteTodo: todo not marked completed")
	}
	if result.CompletedAt == 0 {
		t.Error("CompleteTodo: CompletedAt is zero")
	}
}

func TestMemStoreCompleteTodoNonexistent(t *testing.T) {
	store := NewMemStore("test-agent")
	_, err := store.CompleteTodo(context.Background(), "nonexistent")
	if err == nil {
		t.Error("CompleteTodo(nonexistent): expected error")
	}
}

func TestMemStoreRemoveTodo(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	todo, _ := store.AddTodo(ctx, "remove me", 1)

	if err := store.RemoveTodo(ctx, todo.ID); err != nil {
		t.Fatalf("RemoveTodo: %v", err)
	}

	all, _ := store.ListTodos(ctx, false)
	if len(all) != 0 {
		t.Errorf("RemoveTodo: still have %d todos after removing all", len(all))
	}
}

func TestMemStoreRemoveTodoNonexistent(t *testing.T) {
	store := NewMemStore("test-agent")
	// Removing a nonexistent todo should not error.
	if err := store.RemoveTodo(context.Background(), "nonexistent"); err != nil {
		t.Errorf("RemoveTodo(nonexistent): expected nil, got error: %v", err)
	}
}
