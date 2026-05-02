package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/aj-nt/mnemosyne/client"
)

// RegisterTodoTools adds todo management tool handlers to the registry.
func RegisterTodoTools(registry *ToolRegistry, mnemo *client.Client, agentID string) {
	registry.Register("AddTodo", makeAddTodo(mnemo, agentID), AddTodoSchema)
	registry.Register("ListTodos", makeListTodos(mnemo), ListTodosSchema)
	registry.Register("CompleteTodo", makeCompleteTodo(mnemo), CompleteTodoSchema)
	registry.Register("RemoveTodo", makeRemoveTodo(mnemo), RemoveTodoSchema)
}

func makeAddTodo(mnemo *client.Client, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		content := strArg(args, "content", "")
		priority := intArg(args, "priority", 3)

		if content == "" {
			return "content is required", true
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		todo, err := mnemo.AddTodo(ctx, content, int32(priority), agentID)
		if err != nil {
			return fmt.Sprintf("AddTodo failed: %v", err), true
		}

		completed := ""
		if todo.Completed {
			completed = " ✓"
		}
		return fmt.Sprintf("Todo added: [%s] %s (priority %d)%s", todo.ID, todo.Content, todo.Priority, completed), false
	}
}

func makeListTodos(mnemo *client.Client) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		includeCompleted := boolArg(args, "include_completed", false)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		todos, err := mnemo.ListTodos(ctx, includeCompleted, "")
		if err != nil {
			return fmt.Sprintf("ListTodos failed: %v", err), true
		}
		if len(todos) == 0 {
			return "No todos found.", false
		}

		result := fmt.Sprintf("Found %d todos:\n", len(todos))
		for _, t := range todos {
			status := "☐"
			if t.Completed {
				status = "✓"
			}
			result += fmt.Sprintf("- %s [%s] %s (priority %d)\n", status, t.ID, t.Content, t.Priority)
		}
		return result, false
	}
}

func makeCompleteTodo(mnemo *client.Client) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		id := strArg(args, "id", "")
		if id == "" {
			return "id is required", true
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		todo, err := mnemo.CompleteTodo(ctx, id)
		if err != nil {
			return fmt.Sprintf("CompleteTodo failed: %v", err), true
		}

		return fmt.Sprintf("Todo completed: [%s] %s", todo.ID, todo.Content), false
	}
}

func makeRemoveTodo(mnemo *client.Client) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		id := strArg(args, "id", "")
		if id == "" {
			return "id is required", true
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := mnemo.RemoveTodo(ctx, id); err != nil {
			return fmt.Sprintf("RemoveTodo failed: %v", err), true
		}

		return fmt.Sprintf("Todo removed: [%s]", id), false
	}
}

// --- Todo Schemas ---

var AddTodoSchema = map[string]interface{}{
	"name":        "AddTodo",
	"description": "Add a new todo item. Use this to track tasks or things to remember.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The todo content/task description",
			},
			"priority": map[string]interface{}{
				"type":        "integer",
				"description": "Priority 1-5, higher is more important (default: 3)",
			},
		},
		"required": []string{"content"},
	},
}

var ListTodosSchema = map[string]interface{}{
	"name":        "ListTodos",
	"description": "List todo items. By default shows only incomplete todos.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"include_completed": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to include completed todos (default: false)",
			},
		},
		"required": []string{},
	},
}

var CompleteTodoSchema = map[string]interface{}{
	"name":        "CompleteTodo",
	"description": "Mark a todo item as completed.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The todo ID to complete",
			},
		},
		"required": []string{"id"},
	},
}

var RemoveTodoSchema = map[string]interface{}{
	"name":        "RemoveTodo",
	"description": "Remove a todo item permanently.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The todo ID to remove",
			},
		},
		"required": []string{"id"},
	},
}