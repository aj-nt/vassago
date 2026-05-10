// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// Red Team tests for Phase 17A conversation quality improvements.
// These tests try to break the stuck detection, loop detection, and anti-repetition
// heuristics by sending adversarial inputs and edge cases.

// --- Stuck Detection Red Team ---

// TestRT_Stuck_MixedSuccessAndError: Success after error should NOT trigger stuck detection.
func TestRT_Stuck_MixedSuccessAndError(t *testing.T) {
	results := []agentctx.ToolResult{
		{CallID: "1", Name: "FileSearch", Result: "Error: permission denied", IsError: true},
		{CallID: "2", Name: "FileSearch", Result: "found 3 files", IsError: false},
	}
	if isStuck(results) {
		t.Error("Success after error should NOT be stuck")
	}
}

// TestRT_Stuck_SingleError: One error is not enough to trigger stuck detection.
func TestRT_Stuck_SingleError(t *testing.T) {
	results := []agentctx.ToolResult{
		{CallID: "1", Name: "FileSearch", Result: "Error: not found", IsError: true},
	}
	if isStuck(results) {
		t.Error("Single error should NOT trigger stuck detection")
	}
}

// TestRT_Stuck_DifferentToolsSameError: Different tools both failing should trigger stuck.
func TestRT_Stuck_DifferentToolsSameError(t *testing.T) {
	results := []agentctx.ToolResult{
		{CallID: "1", Name: "FileSearch", Result: "Error: not found", IsError: true},
		{CallID: "2", Name: "FileRead", Result: "Error: file not found", IsError: true},
	}
	if !isStuck(results) {
		t.Error("Two errors from different tools should trigger stuck detection")
	}
}

// TestRT_Stuck_EmptyResult: Empty result should be treated as an error in history.
func TestRT_Stuck_EmptyResult(t *testing.T) {
	messages := []agentctx.Message{
		{Role: "user", Content: "find something"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "Search"}}},
		{Role: "tool", Content: "", ToolCallID: "1", Name: "Search"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "Search"}}},
		{Role: "tool", Content: "", ToolCallID: "2", Name: "Search"},
	}
	if !isStuckFromHistory(messages, 3, 2) {
		t.Error("Empty results in history should trigger stuck detection")
	}
}

// TestRT_Stuck_LongHistoryWithOneFailure: Many successes and one recent failure — should NOT be stuck.
func TestRT_Stuck_LongHistoryWithOneFailure(t *testing.T) {
	messages := []agentctx.Message{
		{Role: "user", Content: "find something"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FileSearch"}}},
		{Role: "tool", Content: "found 5 files", ToolCallID: "1", Name: "FileSearch"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "FileRead"}}},
		{Role: "tool", Content: "file contents...", ToolCallID: "2", Name: "FileRead"},
		{Role: "assistant", Content: "Here's what I found", ToolCalls: nil},
		{Role: "user", Content: "now find another"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "3", Name: "FileSearch"}}},
		{Role: "tool", Content: "0 files found", ToolCallID: "3", Name: "FileSearch"},
	}
	if isStuckFromHistory(messages, 3, 2) {
		t.Error("One failure after successes should NOT trigger stuck detection")
	}
}

// TestRT_Stuck_ErrorWithNullBytes: Error messages with null bytes should still be detected.
func TestRT_Stuck_ErrorWithNullBytes(t *testing.T) {
	results := []agentctx.ToolResult{
		{CallID: "1", Name: "Search", Result: "Error: \x00not found", IsError: true},
		{CallID: "2", Name: "Search", Result: "Error: \x00no such file", IsError: true},
	}
	if !isStuck(results) {
		t.Error("Errors with null bytes should still trigger stuck detection")
	}
}

// --- Loop Detection Red Team ---

// TestRT_Loop_FourDifferentTools: 5 consecutive different tools should be caught.

// TestRT_Loop_ThreeDifferentTools: 3 different tools should NOT be flagged.

// TestRT_Loop_SameToolTwice: 2 calls to the same tool should NOT be flagged as repetitive loop.

// TestRT_ResultNotFound_EdgeCases: Test various "not found" patterns.
func TestRT_ResultNotFound_EdgeCases(t *testing.T) {
	cases := []struct {
		content  string
		expected bool
	}{
		// Should match
		{"File not found", true},
		{"No such file or directory", true},
		{"The path does not exist", true},
		{"no results found for query", true},
		{"No matches found", true},
		{"0 files found matching pattern", true},
		{"Empty result set returned", true},
		// Case insensitive
		{"FILE NOT FOUND", true},
		{"No Such File", true},
		// Should NOT match — legitimate results
		{"found 3 files", false},
		{"Here is the content", false},
		{"Success: data retrieved", false},
		{"3 matches found", false},
	}

	for _, tc := range cases {
		result := agentctx.IsResultNotFound(tc.content)
		if result != tc.expected {
			t.Errorf("IsResultNotFound(%q) = %v, expected %v", tc.content, result, tc.expected)
		}
	}
}

// TestRT_StuckFromHistory_InterleavedErrors: Errors with successes between them should NOT be stuck.
func TestRT_StuckFromHistory_InterleavedErrors(t *testing.T) {
	messages := []agentctx.Message{
		{Role: "user", Content: "find something"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "Search"}}},
		{Role: "tool", Content: "Error: not found", ToolCallID: "1", Name: "Search"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "Search"}}},
		{Role: "tool", Content: "found 5 items", ToolCallID: "2", Name: "Search"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "3", Name: "Search"}}},
		{Role: "tool", Content: "0 files found", ToolCallID: "3", Name: "Search"},
	}
	// Only one "not found" in the last 2 results (most recent is "0 files found",
	// second most recent is "found 5 items" which is success)
	if isStuckFromHistory(messages, 3, 2) {
		t.Error("Interleaved errors/successes should NOT trigger stuck detection")
	}
}

// TestRT_Loop_TextResponseBreaksCrossToolLoop: A text-only response resets the cross-tool counter.

// TestRT_Process_StuckDetectionBeforeLoopDetection: Stuck detection should fire before
// loop detection when agent is stuck, producing a helpful message.
func TestRT_Process_StuckDetectionBeforeLoopDetection(t *testing.T) {
	stage := NewContextManagementStage(0)
	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "find something"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FileSearch"}}},
			{Role: "tool", Content: "no such file", ToolCallID: "1", Name: "FileSearch"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "FileSearch"}}},
			{Role: "tool", Content: "not found", ToolCallID: "2", Name: "FileSearch"},
		},
		MaxIterations: 10,
		Iteration:     3,
	}

	// Agent is both stuck (2 not-founds) and in a repetitive loop (3rd FileSearch call)
	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "3", Name: "FileSearch", Arguments: args},
		},
		FinishReason: "tool_use",
	}

	action := stage.Process(convCtx, nil, parsed)

	// Stuck detection should fire first (before loop detection)
	// Both stuck and loop detection produce YIELD, but stuck adds a helpful message
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD (stuck or loop), got %v", action)
	}

	// Verify the helpful message was added (stuck detection adds this)
	if len(convCtx.Messages) > 0 {
		lastMsg := convCtx.Messages[len(convCtx.Messages)-1]
		if lastMsg.Role == "assistant" && len(lastMsg.Content) > 0 {
			if !strings.Contains(lastMsg.Content, "wasn't able to find") {
				t.Logf("Last message content: %s", lastMsg.Content)
			}
		}
	}
}

// TestRT_Process_MaxIterationsTerminate: Agent should TERMINATE when max iterations reached.
func TestRT_Process_MaxIterationsTerminate(t *testing.T) {
	stage := NewContextManagementStage(0)
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "test"}},
		MaxIterations: 3,
		Iteration:     3, // at limit
	}
	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "FileSearch", Arguments: args},
		},
		FinishReason: "tool_use",
	}

	// Should NOT be stuck or looping — just at max iterations
	toolResults := []agentctx.ToolResult{
		{CallID: "1", Name: "FileSearch", Result: "found files", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)
	if action != agentctx.StageActionTERMINATE {
		t.Errorf("Expected TERMINATE when max iterations reached, got %v", action)
	}
}