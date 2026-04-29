package tools

import (
	"context"
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/client"
)

// RegisterMemoryTools adds all memory tool handlers to the registry.
func RegisterMemoryTools(registry *ToolRegistry, mnemo *client.Client, agentID string) {
	registry.Register("AddMemory", makeAddMemory(mnemo, agentID), AddMemorySchema)
	registry.Register("SearchMemories", makeSearchMemories(mnemo), SearchMemoriesSchema)
	registry.Register("ReplaceMemory", makeReplaceMemory(mnemo, agentID), ReplaceMemorySchema)
	registry.Register("RemoveMemory", makeRemoveMemory(mnemo), RemoveMemorySchema)
}

func makeAddMemory(mnemo *client.Client, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		target := strArg(args, "target", "memory")
		category := strArg(args, "category", "environment")
		key := strArg(args, "key", "")
		content := strArg(args, "content", "")
		priority := intArg(args, "priority", 3)

		if key == "" || content == "" {
			return "key and content are required", true
		}

		ctx := context.Background()
		entry, err := mnemo.AddMemory(ctx, target, category, key, content, int32(priority), agentID)
		if err != nil {
			return fmt.Sprintf("AddMemory failed: %v", err), true
		}
		return fmt.Sprintf("Memory saved: [%s] %s: %s (priority %d)", entry.Category, entry.Key, truncate(entry.Content, 80), entry.Priority), false
	}
}

func makeSearchMemories(mnemo *client.Client) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		query := strArg(args, "query", "")
		target := strArg(args, "target", "")
		limit := intArg(args, "limit", 20)

		if query == "" {
			return "query is required", true
		}

		ctx := context.Background()
		entries, err := mnemo.SearchMemories(ctx, query, target, int32(limit))
		if err != nil {
			return fmt.Sprintf("SearchMemories failed: %v", err), true
		}
		if len(entries) == 0 {
			return "No memories found.", false
		}

		result := fmt.Sprintf("Found %d memories:\n", len(entries))
		for _, e := range entries {
			result += fmt.Sprintf("[%s] %s: %s\n", e.Category, e.Key, truncate(e.Content, 100))
		}
		return result, false
	}
}

func makeReplaceMemory(mnemo *client.Client, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		target := strArg(args, "target", "memory")
		category := strArg(args, "category", "environment")
		key := strArg(args, "key", "")
		content := strArg(args, "content", "")
		priority := intArg(args, "priority", 3)

		if key == "" || content == "" {
			return "key and content are required", true
		}

		ctx := context.Background()
		entry, err := mnemo.AddMemory(ctx, target, category, key, content, int32(priority), agentID)
		if err != nil {
			return fmt.Sprintf("ReplaceMemory failed: %v", err), true
		}
		return fmt.Sprintf("Memory updated: [%s] %s", entry.Category, entry.Key), false
	}
}

func makeRemoveMemory(mnemo *client.Client) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		id := strArg(args, "id", "")
		if id == "" {
			return "id is required", true
		}

		ctx := context.Background()
		if err := mnemo.RemoveMemory(ctx, id); err != nil {
			return fmt.Sprintf("RemoveMemory failed: %v", err), true
		}
		return "Memory removed.", false
	}
}

func strArg(args map[string]interface{}, key, defaultVal string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

func intArg(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Tool schemas for LLM tool definitions (JSON Schema format)
var AddMemorySchema = map[string]interface{}{
	"name":        "AddMemory",
	"description": "Save a memory to persistent storage. Memories survive across sessions.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target":   map[string]interface{}{"type": "string", "enum": []string{"memory", "user"}},
			"category": map[string]interface{}{"type": "string", "enum": []string{"user", "environment", "quirk", "project", "observation"}},
			"key":      map[string]interface{}{"type": "string", "description": "Unique key for upsert"},
			"content":  map[string]interface{}{"type": "string"},
			"priority": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 5, "default": 3},
		},
		"required": []string{"key", "content"},
	},
}

var SearchMemoriesSchema = map[string]interface{}{
	"name":        "SearchMemories",
	"description": "Search memories by keywords. Use real words, not wildcards.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query":   map[string]interface{}{"type": "string"},
			"target":  map[string]interface{}{"type": "string", "enum": []string{"memory", "user", ""}},
			"limit":   map[string]interface{}{"type": "integer", "default": 20},
		},
		"required": []string{"query"},
	},
}

var ReplaceMemorySchema = map[string]interface{}{
	"name":        "ReplaceMemory",
	"description": "Update an existing memory by key. Uses upsert semantics.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target":   map[string]interface{}{"type": "string"},
			"category":  map[string]interface{}{"type": "string"},
			"key":       map[string]interface{}{"type": "string"},
			"content":   map[string]interface{}{"type": "string"},
			"priority":  map[string]interface{}{"type": "integer"},
		},
		"required": []string{"key", "content"},
	},
}

var RemoveMemorySchema = map[string]interface{}{
	"name":        "RemoveMemory",
	"description": "Delete a memory by ID.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "string"},
		},
		"required": []string{"id"},
	},
}

// Ensure agentctx.Message is available
var _ = agentctx.Message{}
