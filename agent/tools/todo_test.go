package tools

import (
	"testing"
)

func TestRegisterTodoTools(t *testing.T) {
	registry := NewToolRegistry()
	RegisterTodoTools(registry, nil, "test-agent")

	for _, name := range []string{"AddTodo", "ListTodos", "CompleteTodo", "RemoveTodo"} {
		handler, err := registry.Get(name)
		if err != nil {
			t.Errorf("Expected %s to be registered: %v", name, err)
		}
		if handler == nil {
			t.Errorf("Expected %s handler to not be nil", name)
		}
	}
}

func TestAddTodoSchema(t *testing.T) {
	name, ok := AddTodoSchema["name"]
	if !ok || name != "AddTodo" {
		t.Error("AddTodoSchema should have name 'AddTodo'")
	}

	params, ok := AddTodoSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("AddTodoSchema should have parameters")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("AddTodoSchema parameters should have required")
	}
	if len(required) != 1 || required[0] != "content" {
		t.Errorf("AddTodoSchema should require 'content', got %v", required)
	}
}

func TestListTodosSchema(t *testing.T) {
	name, ok := ListTodosSchema["name"]
	if !ok || name != "ListTodos" {
		t.Error("ListTodosSchema should have name 'ListTodos'")
	}
}

func TestCompleteTodoSchema(t *testing.T) {
	params, ok := CompleteTodoSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("CompleteTodoSchema should have parameters")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("CompleteTodoSchema parameters should have required")
	}
	if len(required) != 1 || required[0] != "id" {
		t.Errorf("CompleteTodoSchema should require 'id', got %v", required)
	}
}

func TestRemoveTodoSchema(t *testing.T) {
	params, ok := RemoveTodoSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("RemoveTodoSchema should have parameters")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("RemoveTodoSchema parameters should have required")
	}
	if len(required) != 1 || required[0] != "id" {
		t.Errorf("RemoveTodoSchema should require 'id', got %v", required)
	}
}