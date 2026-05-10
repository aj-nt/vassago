// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aj-nt/mnemosyne/client"
)

// RegisterSavedToolTools adds saved tool management handlers to the registry.
// The saved tools themselves (dynamic handlers) are registered separately
// by the agent's RegisterSavedToolHandlers() which refreshes per-turn.
func RegisterSavedToolTools(registry *ToolRegistry, mnemo client.MnemoClient, agentID string) {
	registry.Register("CreateTool", makeCreateTool(mnemo, agentID), CreateToolSchema)
	registry.Register("GetTool", makeGetTool(mnemo), GetToolSchema)
	registry.Register("ListTools", makeListTools(mnemo), ListToolsSchema)
	registry.Register("SearchTools", makeSearchTools(mnemo), SearchToolsSchema)
	registry.Register("UpdateTool", makeUpdateTool(mnemo), UpdateToolSchema)
	registry.Register("RemoveTool", makeRemoveTool(mnemo), RemoveToolSchema)
}

func makeCreateTool(mnemo client.MnemoClient, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		name := strArg(args, "name", "")
		description := strArg(args, "description", "")
		toolType := strArg(args, "type", "script")

		if name == "" || description == "" || args["parameters"] == nil || strArg(args, "script", "") == "" {
			return "name, description, parameters, and script are required", true
		}

		if toolType != "script" {
			return "type must be 'script' (only supported type in v1)", true
		}

		// Marshal parameters — handle both string and map
		parameters := ""
		switch p := args["parameters"].(type) {
		case string:
			if !json.Valid([]byte(p)) {
				return "parameters must be valid JSON", true
			}
			parameters = p
		default:
			paramsJSON, err := json.Marshal(p)
			if err != nil {
				return fmt.Sprintf("parameters marshal failed: %v", err), true
			}
			parameters = string(paramsJSON)
		}

		script := strArg(args, "script", "")
		category := strArg(args, "category", "")
		triggers := listArg(args, "triggers", []string{})
		priority := intArg(args, "priority", 3)

		// Validate name: alphanumeric + underscores, 2-64 chars
		if len(name) < 2 || len(name) > 64 {
			return "tool name must be between 2 and 64 characters", true
		}
		for _, c := range name {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
				return "tool name must contain only alphanumeric characters and underscores", true
			}
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		entry, err := mnemo.AddSavedTool(ctx, name, description, toolType, parameters, script, category, triggers, agentID, int32(priority))
		if err != nil {
			return fmt.Sprintf("CreateTool failed: %v", err), true
		}
		return fmt.Sprintf("Tool created: [%s] %s (v%d, priority %d). Available next turn.", entry.Category, entry.Name, entry.Version, entry.Priority), false
	}
}

func makeGetTool(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		id := strArg(args, "id", "")
		if id == "" {
			return "id is required", true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		entry, err := mnemo.GetSavedTool(ctx, id)
		if err != nil {
			return fmt.Sprintf("GetTool failed: %v", err), true
		}

		triggersJSON, _ := json.Marshal(entry.Triggers)
		return fmt.Sprintf("Tool: %s\nType: %s\nDescription: %s\nParameters: %s\nScript: %s\nTriggers: %s\nCategory: %s\nVersion: %d\nApproval Every Call: %v\nPriority: %d",
			entry.Name, entry.Type, entry.Description, entry.Parameters, truncate(entry.Script, 200),
			string(triggersJSON), entry.Category, entry.Version, entry.RequiresApprovalEveryCall, entry.Priority), false
	}
}

func makeListTools(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		category := strArg(args, "category", "")
		author := strArg(args, "author", "")
		limit := intArg(args, "limit", 20)

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		entries, err := mnemo.ListSavedTools(ctx, category, author, int32(limit))
		if err != nil {
			return fmt.Sprintf("ListTools failed: %v", err), true
		}

		if len(entries) == 0 {
			return "No saved tools found.", false
		}

		result := fmt.Sprintf("Found %d saved tools:\n", len(entries))
		for _, e := range entries {
			triggersJSON, _ := json.Marshal(e.Triggers)
			result += fmt.Sprintf("[%s] %s: %s (v%d, triggers: %s)\n", e.Category, e.Name, truncate(e.Description, 80), e.Version, string(triggersJSON))
		}
		return result, false
	}
}

func makeSearchTools(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		query := strArg(args, "query", "")
		category := strArg(args, "category", "")
		limit := intArg(args, "limit", 20)

		if query == "" {
			return "query is required", true
		}

		const maxQueryLen = 4096
		if len(query) > maxQueryLen {
			return fmt.Sprintf("Query too long (%d chars, max %d).", len(query), maxQueryLen), true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		entries, err := mnemo.SearchSavedTools(ctx, query, category, int32(limit))
		if err != nil {
			return fmt.Sprintf("SearchTools failed: %v", err), true
		}

		if len(entries) == 0 {
			return "No saved tools found.", false
		}

		result := fmt.Sprintf("Found %d saved tools:\n", len(entries))
		for _, e := range entries {
			triggersJSON, _ := json.Marshal(e.Triggers)
			result += fmt.Sprintf("[%s] %s: %s (v%d, triggers: %s)\n", e.Category, e.Name, truncate(e.Description, 80), e.Version, string(triggersJSON))
		}
		return result, false
	}
}

func makeUpdateTool(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		id := strArg(args, "id", "")
		if id == "" {
			return "id is required", true
		}

		updates := make(map[string]interface{})
		if v, ok := args["name"].(string); ok && v != "" {
			updates["name"] = v
		}
		if v, ok := args["description"].(string); ok && v != "" {
			updates["description"] = v
		}
		if v, ok := args["parameters"]; ok {
			switch val := v.(type) {
			case string:
				if !json.Valid([]byte(val)) {
					return "parameters must be valid JSON", true
				}
				updates["parameters"] = val
			default:
				paramsJSON, _ := json.Marshal(val)
				updates["parameters"] = string(paramsJSON)
			}
		}
		if v, ok := args["script"].(string); ok && v != "" {
			updates["script"] = v
		}
		if v, ok := args["category"].(string); ok && v != "" {
			updates["category"] = v
		}
		if v, ok := args["triggers"]; ok {
			switch triggers := v.(type) {
			case []string:
				updates["triggers"] = triggers
			case []interface{}:
				strs := make([]string, len(triggers))
				for i, t := range triggers {
					strs[i] = fmt.Sprint(t)
				}
				updates["triggers"] = strs
			}
		}
		if v, ok := args["priority"].(float64); ok {
			updates["priority"] = int(v)
		}
		if v, ok := args["requires_approval_every_call"].(bool); ok {
			updates["requires_approval_every_call"] = v
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		entry, err := mnemo.UpdateSavedTool(ctx, id, updates)
		if err != nil {
			return fmt.Sprintf("UpdateTool failed: %v", err), true
		}
		return fmt.Sprintf("Tool updated: [%s] %s (now v%d). Changes take effect next turn.", entry.Category, entry.Name, entry.Version), false
	}
}

func makeRemoveTool(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		id := strArg(args, "id", "")
		if id == "" {
			return "id is required", true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		removed, err := mnemo.RemoveSavedTool(ctx, id)
		if err != nil {
			return fmt.Sprintf("RemoveTool failed: %v", err), true
		}

		if removed {
			return fmt.Sprintf("Tool %s removed. Changes take effect next turn.", id), false
		}
		return fmt.Sprintf("Tool %s not found.", id), false
	}
}

// JSON Schema definitions

var CreateToolSchema = map[string]interface{}{
	"name":        "CreateTool",
	"description": "Create a new saved tool that can be called in future sessions. Script-type tools run commands via the existing Execute handler with all security checks. Requires user approval.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name":        map[string]interface{}{"type": "string", "description": "Unique tool name (alphanumeric + underscores, 2-64 chars)"},
			"description": map[string]interface{}{"type": "string", "description": "What this tool does, for the LLM to understand when to call it"},
			"type":        map[string]interface{}{"type": "string", "enum": []string{"script"}, "description": "Tool type — 'script' only in v1"},
			"parameters":  map[string]interface{}{"description": "JSON Schema for the tool's input parameters"},
			"script":      map[string]interface{}{"type": "string", "description": "Executable content with {{param}} placeholders"},
			"category":    map[string]interface{}{"type": "string", "description": "Category (e.g., 'devops', 'astrology')"},
			"triggers":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Keywords for discovery"},
			"priority":    map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 5, "description": "Priority 1-5"},
		},
		"required": []string{"name", "description", "parameters", "script"},
	},
}

var GetToolSchema = map[string]interface{}{
	"name":        "GetTool",
	"description": "Retrieve a saved tool by ID.",
	"parameters": map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"id": map[string]interface{}{"type": "string"}},
		"required":   []string{"id"},
	},
}

var ListToolsSchema = map[string]interface{}{
	"name":        "ListTools",
	"description": "List saved tools, optionally filtered.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"category": map[string]interface{}{"type": "string"},
			"author":   map[string]interface{}{"type": "string"},
			"limit":    map[string]interface{}{"type": "integer"},
		},
	},
}

var SearchToolsSchema = map[string]interface{}{
	"name":        "SearchTools",
	"description": "Search saved tools by FTS5.",
	"parameters": map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{
			"query":    map[string]interface{}{"type": "string"},
			"category": map[string]interface{}{"type": "string"},
			"limit":    map[string]interface{}{"type": "integer"},
		},
		"required": []string{"query"},
	},
}

var UpdateToolSchema = map[string]interface{}{
	"name":        "UpdateTool",
	"description": "Update a saved tool. Increments version. Requires approval.",
	"parameters": map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{
			"id":                           map[string]interface{}{"type": "string"},
			"name":                         map[string]interface{}{"type": "string"},
			"description":                   map[string]interface{}{"type": "string"},
			"parameters":                    map[string]interface{}{"description": "New parameter schema"},
			"script":                        map[string]interface{}{"type": "string"},
			"category":                      map[string]interface{}{"type": "string"},
			"triggers":                      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"priority":                      map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 5},
			"requires_approval_every_call":   map[string]interface{}{"type": "boolean"},
		},
		"required": []string{"id"},
	},
}

var RemoveToolSchema = map[string]interface{}{
	"name":        "RemoveTool",
	"description": "Remove a saved tool. Requires approval.",
	"parameters": map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"id": map[string]interface{}{"type": "string"}},
		"required":   []string{"id"},
	},
}

// Ensure strings import is used (for template param validation in agent/saved_tools.go)
var _ = strings.Contains
