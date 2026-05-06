// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/client"
	pb "github.com/aj-nt/mnemosyne/proto"
)

// RegisterMemoryTools adds all memory tool handlers to the registry.
// If mutationLimit is not nil, AddMemory calls are rate-limited per session.
func RegisterMemoryTools(registry *ToolRegistry, mnemo client.MnemoClient, agentID string, mutationLimit *MutationRateLimit) {
	registry.Register("AddMemory", makeAddMemory(mnemo, agentID, mutationLimit), AddMemorySchema)
	registry.Register("SearchMemories", makeSearchMemories(mnemo), SearchMemoriesSchema)
	registry.Register("ReplaceMemory", makeReplaceMemory(mnemo, agentID), ReplaceMemorySchema)
	registry.Register("RemoveMemory", makeRemoveMemory(mnemo), RemoveMemorySchema)
	registry.Register("Consolidate", makeConsolidate(mnemo, agentID), ConsolidateSchema)
	registry.Register("GetHotBlock", makeGetHotBlock(mnemo, agentID), GetHotBlockSchema)
}

func makeAddMemory(mnemo client.MnemoClient, agentID string, mutationLimit *MutationRateLimit) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		// Check mutation rate limit first
		if mutationLimit != nil && !mutationLimit.Allow() {
			return fmt.Sprintf("AddMemory rate limit reached (%d mutations this session). Consolidate or wait for next session.", mutationLimit.Count()), true
		}

		target := strArg(args, "target", "memory")
		category := strArg(args, "category", "environment")
		key := strArg(args, "key", "")
		content := strArg(args, "content", "")
		priority := intArg(args, "priority", 3)

		if key == "" || content == "" {
			return "key and content are required", true
		}

		// Reject oversized content — prevents database bloat from accidental large writes
		const maxContentLen = 100000 // 100KB
		if len(content) > maxContentLen {
			return fmt.Sprintf("Content too large (%d chars, max %d). Break into smaller entries.", len(content), maxContentLen), true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		entry, err := mnemo.AddMemory(ctx, target, category, key, content, int32(priority), agentID)
		if err != nil {
			return fmt.Sprintf("AddMemory failed: %v", err), true
		}
		return fmt.Sprintf("Memory saved: [%s] %s: %s (priority %d)", entry.Category, entry.Key, truncate(entry.Content, 80), entry.Priority), false
	}
}

func makeSearchMemories(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		query := strArg(args, "query", "")
		target := strArg(args, "target", "")
		limit := intArg(args, "limit", 20)

		if query == "" {
			return "query is required", true
		}

		// Reject oversized queries — prevents DoS via huge FTS5 MATCH input
		const maxQueryLen = 4096
		if len(query) > maxQueryLen {
			return fmt.Sprintf("Query too long (%d chars, max %d). Use more specific search terms.", len(query), maxQueryLen), true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
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

func makeReplaceMemory(mnemo client.MnemoClient, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		target := strArg(args, "target", "memory")
		category := strArg(args, "category", "environment")
		key := strArg(args, "key", "")
		content := strArg(args, "content", "")
		priority := intArg(args, "priority", 3)

		if key == "" || content == "" {
			return "key and content are required", true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		entry, err := mnemo.AddMemory(ctx, target, category, key, content, int32(priority), agentID)
		if err != nil {
			return fmt.Sprintf("ReplaceMemory failed: %v", err), true
		}
		return fmt.Sprintf("Memory updated: [%s] %s", entry.Category, entry.Key), false
	}
}

func makeRemoveMemory(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		id := strArg(args, "id", "")
		if id == "" {
			return "id is required", true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		if err := mnemo.RemoveMemory(ctx, id); err != nil {
			return fmt.Sprintf("RemoveMemory failed: %v", err), true
		}
		return "Memory removed.", false
	}
}

func makeConsolidate(mnemo client.MnemoClient, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		scopeStr := strArg(args, "scope", "expired")
		batchSize := int32(intArg(args, "batch_size", 100))

		var scope pb.ConsolidateScope
		switch scopeStr {
		case "all":
			scope = pb.ConsolidateScope_CONSOLIDATE_SCOPE_ALL
		case "observations":
			scope = pb.ConsolidateScope_CONSOLIDATE_SCOPE_OBSERVATIONS
		case "category":
			scope = pb.ConsolidateScope_CONSOLIDATE_SCOPE_CATEGORY
		case "expired":
			scope = pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED
		default:
			scope = pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED
		}

		ctx, cancel := withTimeout(LongTimeout)
		defer cancel()
		resp, err := mnemo.Consolidate(ctx, scope, batchSize)
		if err != nil {
			return fmt.Sprintf("Consolidate failed: %v", err), true
		}
		return fmt.Sprintf("Consolidated: removed %d, merged %d. %s", resp.GetRemoved(), resp.GetMerged(), resp.GetSummary()), false
	}
}

func makeGetHotBlock(mnemo client.MnemoClient, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		target := strArg(args, "target", "memory")
		charLimit := int32(intArg(args, "char_limit", 4000))

		// Use a timeout instead of context.Background() — the SSH tunnel or
		// daemon can stall, and without a deadline the tool handler blocks
		// forever while the CLI's turn-level cancel can't reach it.
		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()

		hotBlock, err := mnemo.GetHotBlock(ctx, target, agentID, charLimit, nil, nil, 0)
		if err != nil {
			return fmt.Sprintf("GetHotBlock failed: %v", err), true
		}
		return fmt.Sprintf("Hot block retrieved (target: %s, char limit: %d): %s", target, charLimit, truncate(hotBlock, 100)), false
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
		"required": []string{"key", "content", "category"},
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

var ConsolidateSchema = map[string]interface{}{
	"name":        "Consolidate",
	"description": "Consolidate expired or idle memories to free up space",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"scope":      map[string]interface{}{"type": "string", "enum": []string{"expired", "observations", "category", "all"}, "default": "expired"},
			"batch_size": map[string]interface{}{"type": "integer", "default": 100},
		},
	},
}

var GetHotBlockSchema = map[string]interface{}{
	"name":        "GetHotBlock",
	"description": "Get the current hot block of high-priority memories",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target":     map[string]interface{}{"type": "string", "enum": []string{"memory", "user"}, "default": "memory"},
			"char_limit": map[string]interface{}{"type": "integer", "default": 4000},
		},
	},
}

// Ensure agentctx.Message is available
var _ = agentctx.Message{}
