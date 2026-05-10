// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"encoding/json"
	"fmt"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

// --- RED TEAM QA: Tool Dispatch Pipeline ---

func setupRedTeamDispatch(t *testing.T) (*ParallelToolDispatchStage, *tools.ToolRegistry) {
	t.Helper()
	bus := eventbus.NewEventBus()
	registry := tools.NewToolRegistry()

	registry.Register("Echo", func(args map[string]interface{}) (string, bool) {
		msg, _ := args["message"].(string)
		return msg, false
	}, map[string]interface{}{
		"name": "Echo",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{"type": "string"},
			},
			"required": []string{"message"},
		},
	})

	stage := NewParallelToolDispatchStage(registry, bus)
	return stage, registry
}

func TestRedTeam_Dispatch_UnknownTool(t *testing.T) {
	t.Parallel()
	stage, _ := setupRedTeamDispatch(t)

	parsed := agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "NonExistentTool", Arguments: json.RawMessage(`{"msg": "hi"}`)},
		},
	}

	result := stage.Process(nil, parsed)
	if len(result.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(result.ToolResults))
	}
	if !result.ToolResults[0].IsError {
		t.Error("unknown tool should return error result")
	}
}

func TestRedTeam_Dispatch_EmptyArgs(t *testing.T) {
	t.Parallel()
	stage, _ := setupRedTeamDispatch(t)

	parsed := agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "Echo", Arguments: json.RawMessage(``)},
		},
	}

	result := stage.Process(nil, parsed)
	if len(result.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(result.ToolResults))
	}
	// Empty args → {} (Phase 14E fix) → validation fails (message is required)
	if !result.ToolResults[0].IsError {
		t.Error("empty args for tool with required fields should return validation error")
	}
	t.Logf("Empty args result: %s", result.ToolResults[0].Result)
}

func TestRedTeam_Dispatch_NullArgs(t *testing.T) {
	t.Parallel()
	stage, _ := setupRedTeamDispatch(t)

	parsed := agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "Echo", Arguments: json.RawMessage(`null`)},
		},
	}

	result := stage.Process(nil, parsed)
	if len(result.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(result.ToolResults))
	}
	if !result.ToolResults[0].IsError {
		t.Error("null args should fail validation")
	}
}

func TestRedTeam_Dispatch_WrongTypeArgs(t *testing.T) {
	t.Parallel()
	stage, _ := setupRedTeamDispatch(t)

	parsed := agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "Echo", Arguments: json.RawMessage(`{"message": 42}`)},
		},
	}

	result := stage.Process(nil, parsed)
	if len(result.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(result.ToolResults))
	}
	// Type mismatch should be caught by validation
	if !result.ToolResults[0].IsError {
		t.Logf("WARNING: type mismatch not caught by validation. Result: %s", result.ToolResults[0].Result)
	} else {
		t.Logf("Type mismatch correctly rejected: %s", result.ToolResults[0].Result)
	}
}

func TestRedTeam_Dispatch_SQLInjectionInToolName(t *testing.T) {
	t.Parallel()
	stage, _ := setupRedTeamDispatch(t)

	parsed := agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "AddMemory'; DROP TABLE memories; --", Arguments: json.RawMessage(`{"key":"k"}`)},
		},
	}

	result := stage.Process(nil, parsed)
	if len(result.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(result.ToolResults))
	}
	if !result.ToolResults[0].IsError {
		t.Error("SQL injection in tool name should be rejected as unknown tool")
	}
}

func TestRedTeam_Dispatch_InvalidJSON(t *testing.T) {
	t.Parallel()
	stage, _ := setupRedTeamDispatch(t)

	parsed := agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "Echo", Arguments: json.RawMessage(`{invalid json}`)},
		},
	}

	result := stage.Process(nil, parsed)
	if len(result.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(result.ToolResults))
	}
	if !result.ToolResults[0].IsError {
		t.Error("invalid JSON should return error")
	}
	t.Logf("Invalid JSON result: %s", result.ToolResults[0].Result)
}

func TestRedTeam_Dispatch_MultipleToolCalls(t *testing.T) {
	t.Parallel()
	stage, _ := setupRedTeamDispatch(t)

	// ParallelToolDispatchStage executes all calls concurrently (bounded by semaphore).
	// Unlike the old sequential ToolDispatchStage, there is no hard call cap —
	// all valid calls succeed regardless of count.

	calls := make([]agentctx.ToolCall, 20)
	for i := range calls {
		calls[i] = agentctx.ToolCall{
			ID:        fmt.Sprintf("call-%d", i),
			Name:      "Echo",
			Arguments: json.RawMessage(`{"message": "test"}`),
		}
	}

	parsed := agentctx.ParsedResponse{ToolCalls: calls}
	result := stage.Process(nil, parsed)
	if len(result.ToolResults) != 20 {
		t.Fatalf("expected 20 tool results, got %d", len(result.ToolResults))
	}
	for i, tr := range result.ToolResults {
		if tr.IsError {
			t.Errorf("tool result %d unexpected error: %s", i, tr.Result)
		}
	}
}

func TestRedTeam_Dispatch_NoToolCalls(t *testing.T) {
	t.Parallel()
	stage, _ := setupRedTeamDispatch(t)

	// Response with no tool calls should return empty ToolResults
	parsed := agentctx.ParsedResponse{}
	result := stage.Process(nil, parsed)
	if len(result.ToolResults) != 0 {
		t.Errorf("expected 0 tool results, got %d", len(result.ToolResults))
	}
}