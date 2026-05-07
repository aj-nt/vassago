// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"encoding/json"
	"fmt"

	"github.com/aj-nt/mnemosyne/client"
)

// RegisterSkillTools adds all skill tool handlers to the registry.
func RegisterSkillTools(registry *ToolRegistry, mnemo client.MnemoClient, agentID string) {
	registry.Register("AddSkill", makeAddSkill(mnemo, agentID), AddSkillSchema)
	registry.Register("GetSkill", makeGetSkill(mnemo), GetSkillSchema)
	registry.Register("ListSkills", makeListSkills(mnemo), ListSkillsSchema)
	registry.Register("SearchSkills", makeSearchSkills(mnemo), SearchSkillsSchema)
	registry.Register("RemoveSkill", makeRemoveSkill(mnemo), RemoveSkillSchema)
}

func makeAddSkill(mnemo client.MnemoClient, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		name := strArg(args, "name", "")
		description := strArg(args, "description", "")
		triggers := listArg(args, "triggers", []string{})
		category := strArg(args, "category", "")
		content := strArg(args, "content", "")
		priority := intArg(args, "priority", 3)

		if name == "" || description == "" || len(triggers) == 0 || category == "" || content == "" {
			return "name, description, triggers, category, and content are required", true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		skill, err := mnemo.AddSkill(ctx, name, description, triggers, category, content, agentID, int32(priority))
		if err != nil {
			return fmt.Sprintf("AddSkill failed: %v", err), true
		}
		return fmt.Sprintf("Skill added: [%s] %s (priority %d)", skill.Category, skill.Name, skill.Priority), false
	}
}

func makeGetSkill(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		id := strArg(args, "id", "")
		if id == "" {
			return "id is required", true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		skill, err := mnemo.GetSkill(ctx, id)
		if err != nil {
			return fmt.Sprintf("GetSkill failed: %v", err), true
		}

		triggersJSON, _ := json.Marshal(skill.Triggers)
		return fmt.Sprintf("Skill: %s\nDescription: %s\nTriggers: %s\nCategory: %s\nContent: %s\nPriority: %d",
			skill.Name, skill.Description, string(triggersJSON), skill.Category, truncate(skill.Content, 200), skill.Priority), false
	}
}

func makeListSkills(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		category := strArg(args, "category", "")
		sourceAgent := strArg(args, "source_agent", "")
		limit := intArg(args, "limit", 20)

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		skills, err := mnemo.ListSkills(ctx, category, sourceAgent, int32(limit))
		if err != nil {
			return fmt.Sprintf("ListSkills failed: %v", err), true
		}

		if len(skills) == 0 {
			return "No skills found.", false
		}

		result := fmt.Sprintf("Found %d skills:\n", len(skills))
		for _, s := range skills {
			triggersJSON, _ := json.Marshal(s.Triggers)
			result += fmt.Sprintf("[%s] %s: %s (triggers: %s)\n", s.Category, s.Name, truncate(s.Description, 80), string(triggersJSON))
		}
		return result, false
	}
}

func makeSearchSkills(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		query := strArg(args, "query", "")
		category := strArg(args, "category", "")
		limit := intArg(args, "limit", 20)

		if query == "" {
			return "query is required", true
		}

		// Reject oversized queries
		const maxQueryLen = 4096
		if len(query) > maxQueryLen {
			return fmt.Sprintf("Query too long (%d chars, max %d). Use more specific search terms.", len(query), maxQueryLen), true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		skills, err := mnemo.SearchSkills(ctx, query, category, int32(limit))
		if err != nil {
			return fmt.Sprintf("SearchSkills failed: %v", err), true
		}

		if len(skills) == 0 {
			return "No skills found.", false
		}

		result := fmt.Sprintf("Found %d skills:\n", len(skills))
		for _, s := range skills {
			triggersJSON, _ := json.Marshal(s.Triggers)
			result += fmt.Sprintf("[%s] %s: %s (triggers: %s)\n", s.Category, s.Name, truncate(s.Description, 80), string(triggersJSON))
		}
		return result, false
	}
}

func makeRemoveSkill(mnemo client.MnemoClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		id := strArg(args, "id", "")
		if id == "" {
			return "id is required", true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()
		success, err := mnemo.RemoveSkill(ctx, id)
		if err != nil {
			return fmt.Sprintf("RemoveSkill failed: %v", err), true
		}

		if success {
			return fmt.Sprintf("Skill %s removed successfully.", id), false
		}
		return fmt.Sprintf("Skill %s not found.", id), false
	}
}

// JSON Schema definitions for skill tools

var AddSkillSchema = map[string]interface{}{
	"name":        "AddSkill",
	"description": "Save a skill to persistent storage. Skills are procedural knowledge - reusable approaches for recurring task types.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name":        map[string]interface{}{"type": "string", "description": "Skill name (lowercase, hyphens)"},
			"description": map[string]interface{}{"type": "string", "description": "Skill description"},
			"triggers":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Triggers that activate this skill"},
			"category":    map[string]interface{}{"type": "string", "description": "Skill category (e.g. devops, data-science, mlops)"},
			"content":     map[string]interface{}{"type": "string", "description": "Skill content/implementation"},
			"priority":    map[string]interface{}{"type": "integer", "description": "Priority (1-5, higher = more important)", "minimum": 1, "maximum": 5},
		},
		"required": []string{"name", "description", "triggers", "category", "content"},
	},
}

var GetSkillSchema = map[string]interface{}{
	"name":        "GetSkill",
	"description": "Retrieve a skill by its ID. Returns the full skill content.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "string", "description": "Skill ID"},
		},
		"required": []string{"id"},
	},
}

var ListSkillsSchema = map[string]interface{}{
	"name":        "ListSkills",
	"description": "List skills, optionally filtered by category or source agent.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"category":     map[string]interface{}{"type": "string", "description": "Filter by category"},
			"source_agent":  map[string]interface{}{"type": "string", "description": "Filter by source agent"},
			"limit":         map[string]interface{}{"type": "integer", "description": "Limit results"},
		},
	},
}

var SearchSkillsSchema = map[string]interface{}{
	"name":        "SearchSkills",
	"description": "Search skills by keywords. Use real words, not wildcards.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query":    map[string]interface{}{"type": "string", "description": "Search query"},
			"category": map[string]interface{}{"type": "string", "description": "Filter by category"},
			"limit":    map[string]interface{}{"type": "integer", "description": "Limit results"},
		},
		"required": []string{"query"},
	},
}

var RemoveSkillSchema = map[string]interface{}{
	"name":        "RemoveSkill",
	"description": "Remove a skill by its ID.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "string", "description": "Skill ID to remove"},
		},
		"required": []string{"id"},
	},
}