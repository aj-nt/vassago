// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// RED TEAM: Agent tool handler adversarial tests
// ============================================================================

// --- CreateTool: Crash it ---

func TestRedTeam_CreateTool_NilArgs(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	// nil map
	result, isErr := handler(nil)
	assert.True(t, isErr)
	assert.Contains(t, result, "required")
}

func TestRedTeam_CreateTool_EmptyArgs(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	result, isErr := handler(map[string]interface{}{})
	assert.True(t, isErr)
	assert.Contains(t, result, "required")
}

// --- CreateTool: Inject it ---

func TestRedTeam_CreateTool_SQLInjectionInName(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	// SQL injection attempt in name
	result, isErr := handler(map[string]interface{}{
		"name":        "x'; DROP TABLE saved_tools; --",
		"description": "test",
		"type":        "script",
		"parameters":  `{"type":"object","properties":{}}`,
		"script":      "echo hi",
	})
	// Should reject: name must be alphanumeric + underscores only
	assert.True(t, isErr)
	assert.Contains(t, result, "alphanumeric")
}

func TestRedTeam_CreateTool_SQLInjectionInCategory(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	// Category is not validated for SQL safety — but it goes through parameterized queries.
	// This should succeed (store uses parameterized queries).
	result, isErr := handler(map[string]interface{}{
		"name":        "safe_name",
		"description": "test",
		"type":        "script",
		"parameters":  `{"type":"object","properties":{}}`,
		"script":      "echo hi",
		"category":    "'); DROP TABLE saved_tools; --",
	})
	assert.False(t, isErr)
	assert.Contains(t, result, "Tool created")
}

func TestRedTeam_CreateTool_NullByteInName(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	result, isErr := handler(map[string]interface{}{
		"name":        "bad\x00name",
		"description": "test",
		"type":        "script",
		"parameters":  `{"type":"object","properties":{}}`,
		"script":      "echo hi",
	})
	assert.True(t, isErr)
	_ = result
}

func TestRedTeam_CreateTool_ScriptInjectionViaPlaceholder(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	// Script with {{param}} inside a quoted string — parameter substitution
	// should be literal, not interpret the value as code.
	result, isErr := handler(map[string]interface{}{
		"name":        "echo_tool",
		"description": "test",
		"type":        "script",
		"parameters":  `{"type":"object","properties":{"msg":{"type":"string"}},"required":["msg"]}`,
		"script":      "echo {{msg}}",
	})
	assert.False(t, isErr)
	assert.Contains(t, result, "Tool created")

	// The handler was registered immediately. Verify it exists.
	_, err := reg.Get("echo_tool")
	require.NoError(t, err)
}

// --- CreateTool: Corrupt it ---

func TestRedTeam_CreateTool_DuplicateNameRace(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	args := map[string]interface{}{
		"name":        "race_tool",
		"description": "test",
		"type":        "script",
		"parameters":  `{"type":"object","properties":{}}`,
		"script":      "echo hi",
	}

	// First creation
	result1, isErr1 := handler(args)
	assert.False(t, isErr1)
	assert.Contains(t, result1, "Tool created")

	// Second creation with same name
	result2, isErr2 := handler(args)
	assert.True(t, isErr2)
	_ = result2
}

// --- CreateTool: Starve it ---

func TestRedTeam_CreateTool_NameLengthBoundaries(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	baseArgs := func(name string) map[string]interface{} {
		return map[string]interface{}{
			"name":        name,
			"description": "test",
			"type":        "script",
			"parameters":  `{"type":"object","properties":{}}`,
			"script":      "echo hi",
		}
	}

	// 1 char — too short
	result, isErr := handler(baseArgs("a"))
	assert.True(t, isErr)
	assert.Contains(t, result, "between 2 and 64")

	// 65 chars — too long
	result, isErr = handler(baseArgs(strings.Repeat("x", 65)))
	assert.True(t, isErr)
	assert.Contains(t, result, "between 2 and 64")

	// Exactly 64 chars — valid
	result, isErr = handler(baseArgs(strings.Repeat("x", 64)))
	assert.False(t, isErr)
	assert.Contains(t, result, "Tool created")
}

// --- CreateTool: Spoof it ---

func TestRedTeam_CreateTool_PriorityBoundaries(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	buildArgs := func(name string, p interface{}) map[string]interface{} {
		return map[string]interface{}{
			"name":        name,
			"description": "test",
			"type":        "script",
			"parameters":  `{"type":"object","properties":{}}`,
			"script":      "echo hi",
			"priority":    p,
		}
	}

	// Priority 0 — now rejected at agent layer
	result, isErr := handler(buildArgs("prio_zero", float64(0)))
	assert.True(t, isErr) // now validated at agent layer
	assert.Contains(t, result, "priority must be between 1 and 5")

	// Priority 6
	result, isErr = handler(buildArgs("prio_six", float64(6)))
	assert.True(t, isErr) // now validated at agent layer
	assert.Contains(t, result, "priority must be between 1 and 5")
}

// --- UpdateTool: Inject it ---

func TestRedTeam_UpdateTool_NonExistentID(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("UpdateTool")

	result, isErr := handler(map[string]interface{}{
		"id":          "does-not-exist",
		"description": "new desc",
	})
	assert.True(t, isErr)
	_ = result
}

func TestRedTeam_UpdateTool_EmptyBody(t *testing.T) {
	mock := newMockSavedToolClient()
	mock.AddSavedTool(context.Background(), "empty_update", "desc", "script",
		`{"type":"object","properties":{}}`, "echo hi", "test", nil, "agent-1", 3)

	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("UpdateTool")

	result, isErr := handler(map[string]interface{}{
		"id": "tool-empty_update",
	})
	// Should succeed — no-op update is valid
	assert.False(t, isErr)
	assert.Contains(t, result, "v2") // Note: agent layer always increments version even for no-ops
}

// --- RemoveTool: Crash it ---

func TestRedTeam_RemoveTool_NilArgs(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("RemoveTool")

	result, isErr := handler(nil)
	assert.True(t, isErr)
	assert.Contains(t, result, "id is required")
}

func TestRedTeam_RemoveTool_NonExistent(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("RemoveTool")

	result, isErr := handler(map[string]interface{}{"id": "does-not-exist"})
	assert.False(t, isErr) // not an error, just not found
	assert.Contains(t, result, "not found")
}

// --- SearchTools: Inject it ---

func TestRedTeam_SearchTools_FTS5Injection(t *testing.T) {
	mock := newMockSavedToolClient()
	mock.AddSavedTool(context.Background(), "foo", "Foo tool", "script",
		`{"type":"object","properties":{}}`, "echo foo", "test", nil, "agent-1", 3)

	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("SearchTools")

	// FTS5 query injection — the store passes raw queries to MATCH.
	// This is dangerous if not sanitized. The mock bypasses the store,
	// so we test the agent-level validation only (query length check).
	query := "foo OR 1=1"
	result, isErr := handler(map[string]interface{}{
		"query": query,
	})
	assert.False(t, isErr) // mock doesn't do FTS5, just returns all

	// Very long query
	longQuery := strings.Repeat("x", 5000)
	result, isErr = handler(map[string]interface{}{
		"query": longQuery,
	})
	assert.True(t, isErr)
	assert.Contains(t, result, "too long")
}

// --- GetTool: Inject it ---

func TestRedTeam_GetTool_EmptyID(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("GetTool")

	result, isErr := handler(map[string]interface{}{"id": ""})
	assert.True(t, isErr)
	assert.Contains(t, result, "id is required")
}

// --- ListTools: Starve it ---

func TestRedTeam_ListTools_LargeLimit(t *testing.T) {
	mock := newMockSavedToolClient()
	// Add many tools
	for i := 0; i < 100; i++ {
		mock.AddSavedTool(context.Background(),
			"tool_"+string(rune('a'+i%26))+string(rune('0'+i/26)),
			"desc", "script", `{"type":"object","properties":{}}`,
			"echo hi", "test", nil, "agent-1", 3)
	}

	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("ListTools")

	result, isErr := handler(map[string]interface{}{
		"limit": float64(10000),
	})
	assert.False(t, isErr)
	// Should return all tools (100 of them)
	assert.Contains(t, result, "Found")
}

// --- Handler name collision ---

func TestRedTeam_HandlerNameCollision(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()

	// Register a compiled tool with the same name a saved tool would use
	reg.Register("Execute", func(args map[string]interface{}) (string, bool) {
		return "executed", false
	}, map[string]interface{}{"name": "Execute"})

	// Now try to create a saved tool named "Execute"
	RegisterSavedToolTools(reg, mock, "agent-1")
	handler, _ := reg.Get("CreateTool")

	result, isErr := handler(map[string]interface{}{
		"name":        "Execute",
		"description": "test",
		"type":        "script",
		"parameters":  `{"type":"object","properties":{}}`,
		"script":      "echo hi",
	})
	assert.False(t, isErr)
	assert.Contains(t, result, "Tool created")

	// Now both "Execute" handlers exist — the saved tool one shadowed the compiled one.
	// This is a known behavior: the registry overwrites on Register().
	// The create-then-register-immediately pattern means if you create a saved tool
	// with the same name as a compiled tool, the saved tool wins.
}

// --- Script handler: parameter injection ---

func TestRedTeam_InlineScriptHandler_ShellInjection(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()

	// Register a fake Execute that captures what it receives
	var capturedCmd string
	reg.Register("Execute", func(args map[string]interface{}) (string, bool) {
		capturedCmd = args["command"].(string)
		return "ran: " + capturedCmd, false
	}, map[string]interface{}{"name": "Execute", "description": "run"})

	// Register saved tool tools so inline handler can resolve Execute
	RegisterSavedToolTools(reg, mock, "agent-1")

	// Create a saved tool
	handler, _ := reg.Get("CreateTool")
	_, _ = handler(map[string]interface{}{
		"name":        "shell_test",
		"description": "test",
		"type":        "script",
		"parameters":  `{"type":"object","properties":{"msg":{"type":"string"}},"required":["msg"]}`,
		"script":      "echo {{msg}}",
	})

	// Now call the saved tool with a shell injection payload
	toolHandler, err := reg.Get("shell_test")
	require.NoError(t, err)

	result, isErr := toolHandler(map[string]interface{}{
		"msg": "$(rm -rf /)",
	})
	assert.False(t, isErr)
	// The substituted command should have the literal string "$(rm -rf /)",
	// not the executed result. The shell injection protection depends on
	// the Execute handler's security (path validation, command validation).
	assert.Contains(t, capturedCmd, "$(rm -rf /)")
	// result contains ran: echo $(rm -rf /) — it wasn't executed as shell
	assert.Contains(t, result, "ran:")
}

func TestRedTeam_InlineScriptHandler_UnresolvedPlaceholder(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()

	reg.Register("Execute", func(args map[string]interface{}) (string, bool) {
		return "ran", false
	}, map[string]interface{}{"name": "Execute", "description": "run"})

	RegisterSavedToolTools(reg, mock, "agent-1")

	// Create tool with required param
	handler, _ := reg.Get("CreateTool")
	_, _ = handler(map[string]interface{}{
		"name":        "unresolved_test",
		"description": "test",
		"type":        "script",
		"parameters":  `{"type":"object","properties":{"env":{"type":"string"}},"required":["env"]}`,
		"script":      "deploy.sh --env {{env}}",
	})

	toolHandler, _ := reg.Get("unresolved_test")

	// Call without the required param
	result, isErr := toolHandler(map[string]interface{}{})
	assert.True(t, isErr)
	assert.Contains(t, result, "Unresolved parameters")
	assert.Contains(t, result, "env")
}
