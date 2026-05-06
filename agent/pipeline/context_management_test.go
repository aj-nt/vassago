// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

func TestNewContextManagementStage(t *testing.T) {
	stage := NewContextManagementStage(1000)
	if stage == nil {
		t.Fatal("NewContextManagementStage should return a non-nil stage")
	}
	if stage.MaxContextTokens != 1000 {
		t.Errorf("Expected MaxContextTokens to be 1000, got %d", stage.MaxContextTokens)
	}
}

func TestContextManagementStageProcess_YieldsOnNoToolCalls(t *testing.T) {
	stage := NewContextManagementStage(0) // no token limit
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 5,
		Iteration:     0,
	}
	parsed := agentctx.ParsedResponse{
		Text:         "Hello there",
		ToolCalls:    nil, // no tool calls
		FinishReason: "stop",
	}

	action := stage.Process(convCtx, nil, parsed)

	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD when no tool calls, got %v", action)
	}
}

func TestContextManagementStageProcess_ContinuesOnToolCalls(t *testing.T) {
	stage := NewContextManagementStage(0) // no token limit
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "search memory"}},
		MaxIterations: 5,
		Iteration:     0,
	}
	args, _ := json.Marshal(map[string]interface{}{"query": "test"})
	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call1", Name: "SearchMemories", Arguments: args}},
		FinishReason: "tool_use",
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "call1", Name: "SearchMemories", Result: "found 3 memories", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE when tool calls present, got %v", action)
	}
	// Verify messages were appended
	if len(convCtx.Messages) != 3 {
		t.Errorf("Expected 3 messages (1 user + 1 assistant + 1 tool), got %d", len(convCtx.Messages))
	}
}

func TestContextManagementStageProcess_TerminatesOnMaxIterations(t *testing.T) {
	stage := NewContextManagementStage(0)
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 2,
		Iteration:     2, // at limit
	}
	// Tool calls present — model wants to loop but we've hit the limit
	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call1", Name: "SearchMemories", Arguments: args}},
		FinishReason: "tool_use",
	}

	action := stage.Process(convCtx, nil, parsed)

	if action != agentctx.StageActionTERMINATE {
		t.Errorf("Expected TERMINATE when max iterations reached with tool calls, got %v", action)
	}
}

func TestContextManagementStageProcess_TerminatesOnTokenLimit(t *testing.T) {
	stage := NewContextManagementStage(10) // very low limit
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "this is a very long message that should exceed the token limit"},
		},
		MaxIterations: 100,
		Iteration:      0,
	}
	// Tool calls present — model wants to loop but tokens exceed limit
	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call1", Name: "SearchMemories", Arguments: args}},
		FinishReason: "tool_use",
	}

	action := stage.Process(convCtx, nil, parsed)

	if action != agentctx.StageActionTERMINATE {
		t.Errorf("Expected TERMINATE when token limit exceeded, got %v", action)
	}
}

func TestContextManagementStageProcess_AppendsErrorToolResults(t *testing.T) {
	stage := NewContextManagementStage(0)
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "test"}},
		MaxIterations: 5,
		Iteration:     0,
	}
	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call1", Name: "BadTool", Arguments: args}},
		FinishReason: "tool_use",
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "call1", Name: "BadTool", Result: "something failed", IsError: true},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE even with error tool result, got %v", action)
	}
	// Error tool results should be prefixed with "Error:"
	found := false
	for _, m := range convCtx.Messages {
		if m.Role == "tool" && m.Content == "Error: something failed" {
			found = true
		}
	}
	if !found {
		t.Error("Expected error tool result to be prefixed with 'Error:'")
	}
}

func TestContextManagementStageProcess_CompactsOldToolResults(t *testing.T) {
	// Build a context with old tool calls and enough content to exceed a low token limit
	args, _ := json.Marshal(map[string]interface{}{})

	longToolResult := strings.Repeat("data ", 100) // > 500 chars → ~125 tokens

	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: strings.Repeat("question ", 50)}, // long enough to contribute
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "SearchMemories", Arguments: args}}},
			{Role: "tool", Content: longToolResult, ToolCallID: "1", Name: "SearchMemories"},
			{Role: "assistant", Content: "Your name is AJ."},
			{Role: "user", Content: "What's the weather?"},
			{Role: "assistant", Content: "I don't have weather data."},
		},
		MaxIterations: 10,
		Iteration:     0,
	}

	// Trigger compaction with a very low token limit → the long tool result forces compaction
	convCtx.Compact(50)

	// The old tool result should be removed (it's before the yield point "Your name is AJ.")
	toolCount := 0
	for _, m := range convCtx.Messages {
		if m.Role == "tool" {
			toolCount++
		}
	}
	if toolCount != 0 {
		t.Errorf("Expected old tool results to be removed, found %d", toolCount)
	}

	// The old assistant+tool_call should be summarized
	found := false
	for _, m := range convCtx.Messages {
		if m.Role == "assistant" && m.Content == "[Called SearchMemories]" {
			found = true
		}
	}
	if !found {
		t.Error("Expected old assistant+tool_call to be summarized as '[Called SearchMemories]'")
	}
}

func TestContextManagementStageProcess_CompactionContinuesInsteadOfTerminate(t *testing.T) {
	// If compaction brings the context under the token limit, we should CONTINUE
	stage := NewContextManagementStage(200) // moderate limit

	args, _ := json.Marshal(map[string]interface{}{})
	// Start with a large context from old tool calls
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "Tell me everything"}, // 19 chars /4 = ~5 tokens
			{Role: "assistant", Content: "Let me search", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "SearchMemories", Arguments: args}}},
			{Role: "tool", Content: "Here is a very long result that contains lots of information about many different topics and should consume significant token budget space in the conversation context window for testing purposes", ToolCallID: "1", Name: "SearchMemories"}, // ~200 chars = 50 tokens
			{Role: "assistant", Content: "Based on my search..."}, // short
			{Role: "user", Content: "Thanks, now something else"},  // short
		},
		MaxIterations: 10,
		Iteration:      0,
	}

	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "2", Name: "GetHotBlock", Arguments: args}},
		FinishReason: "tool_use",
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "2", Name: "GetHotBlock", Result: "hot block data", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)
	// If compaction removes the long tool result, it should be able to continue
	if action == agentctx.StageActionTERMINATE {
		t.Error("Expected CONTINUE after compaction, got TERMINATE")
	}
}

// --- Same-tool loop detection ---

func TestIsRepetitiveToolLoop_NoHistory(t *testing.T) {
	// First call — no history, never a loop
	currentCalls := []agentctx.ToolCall{
		{ID: "1", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
	}
	if isRepetitiveToolLoop(nil, currentCalls, 3) {
		t.Error("First tool call should not be detected as a loop")
	}
}

func TestIsRepetitiveToolLoop_TwoCalls(t *testing.T) {
	// Two consecutive same-tool calls — not yet a loop (need 3+)
	messages := []agentctx.Message{
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "hot block data", ToolCallID: "1"},
	}
	currentCalls := []agentctx.ToolCall{
		{ID: "2", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
	}
	if isRepetitiveToolLoop(messages, currentCalls, 3) {
		t.Error("Two consecutive same-tool calls should not be detected as a loop")
	}
}

func TestIsRepetitiveToolLoop_ThreeCalls_BreaksLoop(t *testing.T) {
	// Three+ consecutive same-tool calls — IS a loop
	messages := []agentctx.Message{
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "hot block data 1", ToolCallID: "1"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "2", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "hot block data 2", ToolCallID: "2"},
	}
	currentCalls := []agentctx.ToolCall{
		{ID: "3", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
	}
	if !isRepetitiveToolLoop(messages, currentCalls, 3) {
		t.Error("Three consecutive same-tool calls should be detected as a loop")
	}
}

func TestIsRepetitiveToolLoop_TextBreaksLoop(t *testing.T) {
	// Text-only assistant response (no tool calls) between iterations breaks the loop.
	// The model gave a real answer, then later decided to use a tool again — that's fine.
	messages := []agentctx.Message{
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "hot block data 1", ToolCallID: "1"},
		// Text-only response BREAKS the loop — model answered before calling tool again
		{Role: "assistant", Content: "Here's what I found:", ToolCalls: nil},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "2", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "hot block data 2", ToolCallID: "2"},
	}
	currentCalls := []agentctx.ToolCall{
		{ID: "3", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
	}
	// Count walks backwards: finds #2 in messages, then hits text-only assistant
	// → breaks. Count = 2 (current + #2), NOT 3+ → not a loop.
	if isRepetitiveToolLoop(messages, currentCalls, 3) {
		t.Error("Text-only response between iterations should break the loop counter")
	}
}

func TestIsRepetitiveToolLoop_TextWithToolDoesNotBreakLoop(t *testing.T) {
	// Assistant message with BOTH text and tool calls does NOT break the loop.
	// This is the actual loop pattern from production: model says something,
	// calls GetHotBlock, gets result, says something slightly different,
	// calls GetHotBlock again.
	messages := []agentctx.Message{
		{Role: "assistant", Content: "Let me check.", ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "hot block data 1", ToolCallID: "1"},
		{Role: "assistant", Content: "Sharp.", ToolCalls: []agentctx.ToolCall{
			{ID: "2", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "hot block data 2", ToolCallID: "2"},
	}
	currentCalls := []agentctx.ToolCall{
		{ID: "3", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
	}
	// 3 consecutive GetHotBlock calls (with text between) = loop detected
	if !isRepetitiveToolLoop(messages, currentCalls, 3) {
		t.Error("Text+tool pattern should still be detected as a repeating loop")
	}
}

func TestContextManagement_BreaksRepetitiveLoop(t *testing.T) {
	stage := NewContextManagementStage(0)
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "check memory"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{
				{ID: "1", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
			}},
			{Role: "tool", Content: "hot block data", ToolCallID: "1"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{
				{ID: "2", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
			}},
			{Role: "tool", Content: "hot block data", ToolCallID: "2"},
		},
		MaxIterations: 20,
		Iteration:     3,
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "3", Name: "GetHotBlock", Arguments: json.RawMessage(`{}`)},
		},
		FinishReason: "tool_use",
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "3", Name: "GetHotBlock", Result: "hot block data", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD (break loop), got %v", action)
	}
}

// --- truncateToolResult tests ---

func TestTruncateToolResult_ShortResult(t *testing.T) {
	shortResult := "This is a short result\nwith only a few lines\nnot enough to trigger truncation"
	
	result := agentctx.TruncateToolResult(shortResult, false)
	if result != shortResult {
		t.Errorf("Expected short result to remain unchanged, got:\n%s", result)
	}
}

func TestTruncateToolResult_LongResult(t *testing.T) {
	// Build a long result with more than 100 lines
	var lines []string
	for i := 0; i < 250; i++ {
		lines = append(lines, fmt.Sprintf("Line %d: This is a line of output", i+1))
	}
	longResult := strings.Join(lines, "\n")
	
	result := agentctx.TruncateToolResult(longResult, false)
	
	// Result should have head + tail format
	resultLines := strings.Split(result, "\n")
	
	// Check that the result contains the summary line
	hasSummary := false
	for _, line := range resultLines {
		if strings.HasPrefix(line, "... (truncated:") {
			hasSummary = true
		}
	}
	if !hasSummary {
		t.Errorf("Expected result to contain summary line with truncation info")
	}
	
	// Check that the first headCount lines are preserved
	headCount := 100 / 2 // 50
	for i := 0; i < headCount; i++ {
		if resultLines[i] != lines[i] {
			t.Errorf("Line %d mismatch:\nExpected: %s\nGot: %s", i, lines[i], resultLines[i])
		}
	}
	
	// Check that tail lines are preserved
	tailCount := 100 / 4 // 25
	tailStart := len(lines) - tailCount
	// Find the tail section (after the summary)
	foundTailStart := -1
	for i, line := range resultLines {
		if line == lines[tailStart] {
			foundTailStart = i
			break
		}
	}
	if foundTailStart == -1 {
		t.Errorf("Could not find tail section start in result")
	}
}

func TestTruncateToolResult_ErrorResult(t *testing.T) {
	errorResult := "This is an error message\nthat should not be truncated"
	
	result := agentctx.TruncateToolResult(errorResult, true)
	if result != errorResult {
		t.Errorf("Expected error result to remain unchanged, got:\n%s", result)
	}
}

func TestTruncateToolResult_ExactFormat(t *testing.T) {
	// Build a result with exactly 101 lines to test the truncation
	var lines []string
	for i := 0; i < 101; i++ {
		lines = append(lines, fmt.Sprintf("Line %d", i+1))
	}
	input := strings.Join(lines, "\n")
	
	result := agentctx.TruncateToolResult(input, false)
	
	// Check that result contains head (50 lines) + summary + tail (25 lines)
	resultLines := strings.Split(result, "\n")
	
	// Check that the summary line is present
	hasSummary := false
	for _, line := range resultLines {
		if strings.HasPrefix(line, "... (truncated:") {
			hasSummary = true
		}
	}
	if !hasSummary {
		t.Errorf("Expected result to contain summary line")
	}
	
	// Check that the first headCount lines are preserved
	headCount := 100 / 2
	for i := 0; i < headCount; i++ {
		if resultLines[i] != lines[i] {
			t.Errorf("Line %d mismatch:\nExpected: %s\nGot: %s", i, lines[i], resultLines[i])
		}
	}
}

func TestIsConsecutiveToolIterationLoop_DifferentTools(t *testing.T) {
	// 4+ consecutive iterations with different tools — IS a loop
	// Threshold is 4 to allow legitimate 3-step workflows (read→search→edit)
	messages := []agentctx.Message{
		{Role: "user", Content: "check this"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "FileSearch", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "found files", ToolCallID: "1"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "2", Name: "FileRead", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "file contents", ToolCallID: "2"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "3", Name: "Execute", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "command output", ToolCallID: "3"},
	}
	currentCalls := []agentctx.ToolCall{
		{ID: "4", Name: "FileSearch", Arguments: json.RawMessage(`{}`)},
	}
	if !isConsecutiveToolIterationLoop(messages, currentCalls, 4) {
		t.Error("4+ consecutive tool iterations with different tools should be detected as a loop")
	}
}

func TestIsConsecutiveToolIterationLoop_ThreeIterationsAllowed(t *testing.T) {
	// 3 consecutive tool iterations should still be allowed (legitimate workflow)
	messages := []agentctx.Message{
		{Role: "user", Content: "check this"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "FileSearch", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "found files", ToolCallID: "1"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "2", Name: "FileRead", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "file contents", ToolCallID: "2"},
	}
	currentCalls := []agentctx.ToolCall{
		{ID: "3", Name: "Execute", Arguments: json.RawMessage(`{}`)},
	}
	if isConsecutiveToolIterationLoop(messages, currentCalls, 4) {
		t.Error("3 consecutive tool iterations should be allowed (threshold is 4)")
	}
}

func TestIsConsecutiveToolLoop_TextBreaksLoop(t *testing.T) {
	// Test that a text-only response should reset the counter
	messages := []agentctx.Message{
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "FileSearch", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "search results", Name: "FileSearch", ToolCallID: "1"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "2", Name: "FileRead", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Content: "file content", Name: "FileRead", ToolCallID: "2"},
		{Role: "assistant", Content: "Here's what I found: ...", ToolCalls: nil}, // Text-only breaks the loop
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{
			{ID: "3", Name: "Execute", Arguments: json.RawMessage(`{}`)},
		}},
	}

	currentCalls := []agentctx.ToolCall{
		{ID: "4", Name: "FileSearch", Arguments: json.RawMessage(`{}`)},
	}

	// Should NOT detect a loop because text response broke it
	if isConsecutiveToolIterationLoop(messages, currentCalls, 4) {
		t.Error("Expected text-only response to break the loop")
	}
}

// --- Stuck detection tests ---

func TestIsStuck_TwoErrors(t *testing.T) {
	// Two consecutive error results should return true
	results := []agentctx.ToolResult{
		{CallID: "1", Name: "Tool1", Result: "error 1", IsError: true},
		{CallID: "2", Name: "Tool2", Result: "error 2", IsError: true},
	}

	if !isStuck(results) {
		t.Error("Expected isStuck to return true for two consecutive errors")
	}
}

func TestIsStuck_TwoNotFound(t *testing.T) {
	// Two "not found" results should return true
	results := []agentctx.ToolResult{
		{CallID: "1", Name: "Tool1", Result: "file not found", IsError: false},
		{CallID: "2", Name: "Tool2", Result: "no such file or directory", IsError: false},
	}

	if !isStuck(results) {
		t.Error("Expected isStuck to return true for two 'not found' messages")
	}
}

func TestIsStuck_SuccessAfterError(t *testing.T) {
	// One error then one success should return false
	results := []agentctx.ToolResult{
		{CallID: "1", Name: "Tool1", Result: "error 1", IsError: true},
		{CallID: "2", Name: "Tool2", Result: "success", IsError: false},
	}

	if isStuck(results) {
		t.Error("Expected isStuck to return false when one error is followed by success")
	}
}

func TestIsStuck_InsufficientResults(t *testing.T) {
	// Less than 2 results should return false
	results := []agentctx.ToolResult{
		{CallID: "1", Name: "Tool1", Result: "error 1", IsError: true},
	}

	if isStuck(results) {
		t.Error("Expected isStuck to return false when there are less than 2 results")
	}
}

func TestIsStuckFromHistory_TwoErrors(t *testing.T) {
	messages := []agentctx.Message{
		{Role: "user", Content: "find something"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FindTool"}}},
		{Role: "tool", Content: "Error: permission denied", ToolCallID: "1", Name: "FindTool"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "FindTool"}}},
		{Role: "tool", Content: "Error: still no access", ToolCallID: "2", Name: "FindTool"},
	}
	if !isStuckFromHistory(messages, 3, 2) {
		t.Error("Expected isStuckFromHistory to return true for two consecutive errors")
	}
}

func TestIsStuckFromHistory_TwoNotFound(t *testing.T) {
	messages := []agentctx.Message{
		{Role: "user", Content: "find something"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FileSearch"}}},
		{Role: "tool", Content: "not found", ToolCallID: "1", Name: "FileSearch"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "FileSearch"}}},
		{Role: "tool", Content: "no results", ToolCallID: "2", Name: "FileSearch"},
	}
	if !isStuckFromHistory(messages, 3, 2) {
		t.Error("Expected isStuckFromHistory to return true for two 'not found' results")
	}
}

func TestIsStuckFromHistory_SuccessAfterError(t *testing.T) {
	messages := []agentctx.Message{
		{Role: "user", Content: "find something"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FileSearch"}}},
		{Role: "tool", Content: "not found", ToolCallID: "1", Name: "FileSearch"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "FileSearch"}}},
		{Role: "tool", Content: "found: 3 files", ToolCallID: "2", Name: "FileSearch"},
	}
	if isStuckFromHistory(messages, 3, 2) {
		t.Error("Expected isStuckFromHistory to return false when success follows error")
	}
}

func TestIsStuckFromHistory_InsufficientHistory(t *testing.T) {
	// Only one tool result in history — not enough to detect stuck
	messages := []agentctx.Message{
		{Role: "user", Content: "find something"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FileSearch"}}},
		{Role: "tool", Content: "Error: not found", ToolCallID: "1", Name: "FileSearch"},
	}
	if isStuckFromHistory(messages, 3, 2) {
		t.Error("Expected isStuckFromHistory to return false with insufficient history")
	}
}

// --- Stuck detection integration test (via Process) ---

func TestContextManagement_BreaksStuckLoop_ViaToolResults(t *testing.T) {
	// Test that stuck detection via toolResults parameter causes YIELD
	// with two consecutive error tool results and no history
	stage := NewContextManagementStage(0)
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "find something"}},
		MaxIterations: 20,
		Iteration:     2,
	}

	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "3", Name: "FindTool", Arguments: args},
		},
		FinishReason: "tool_use",
	}

	// Two consecutive error results — stuck
	toolResults := []agentctx.ToolResult{
		{CallID: "1", Name: "FindTool", Result: "not found", IsError: true},
		{CallID: "2", Name: "FindTool", Result: "file not found", IsError: true},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	// Should YIELD with helpful message instead of continuing
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD when stuck (two consecutive errors), got %v", action)
	}

	// Should add helpful message
	lastMessage := convCtx.Messages[len(convCtx.Messages)-1]
	if !strings.Contains(lastMessage.Content, "wasn't able to find") {
		t.Errorf("Expected helpful message about not finding results, got: %s", lastMessage.Content)
	}
}

func TestContextManagement_BreaksStuckLoop_ViaHistory(t *testing.T) {
	// Test that stuck detection via conversation history causes YIELD
	// even when toolResults parameter is empty (loop detection fires before
	// tool results are appended)
	stage := NewContextManagementStage(0)
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "find something"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FindTool"}}},
			{Role: "tool", Content: "Error: not found", ToolCallID: "1", Name: "FindTool"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "FindTool"}}},
			{Role: "tool", Content: "Error: file not found", ToolCallID: "2", Name: "FindTool"},
		},
		MaxIterations: 20,
		Iteration:     3,
	}

	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "3", Name: "FindTool", Arguments: args},
		},
		FinishReason: "tool_use",
	}

	// Pass nil toolResults — stuck detection should use history instead
	action := stage.Process(convCtx, nil, parsed)

	// Should YIELD with helpful message instead of continuing
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD when stuck (from history), got %v", action)
	}

	lastMessage := convCtx.Messages[len(convCtx.Messages)-1]
	if !strings.Contains(lastMessage.Content, "wasn't able to find") {
		t.Errorf("Expected helpful message about not finding results, got: %s", lastMessage.Content)
	}
}

// --- Aggressive compaction tests ---

func TestCompactContext_HighIteration(t *testing.T) {
	// Test that iteration 5+ strips long tool results when over the token limit
	args, _ := json.Marshal(map[string]interface{}{})

	// Create a context with a long tool result and enough total content to exceed a low limit
	longResult := "This is a very long tool result that contains lots of information " +
		"and should be stripped down to a summary during aggressive compaction for " +
		"high iteration counts. This text needs to be over 200 characters to trigger " +
		"the aggressive compaction logic that replaces long tool results with summaries."

	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: strings.Repeat("question ", 100)}, // force over limit
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "SearchMemories", Arguments: args}}},
			{Role: "tool", Content: longResult, ToolCallID: "1", Name: "SearchMemories"},
		},
		MaxIterations: 10,
		Iteration:     5, // High iteration should trigger aggressive compaction
	}

	// Trigger compaction with a low limit so it actually compacts
	convCtx.Compact(50)

	// The long tool result should be replaced with a summary (by Phase 0 aggressive stripping)
	foundSummary := false
	for _, m := range convCtx.Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "[tool result:") {
			foundSummary = true
			// Verify the format
			expectedFormat := fmt.Sprintf("[tool result: %s (%d chars)]", m.Name, len(longResult))
			if m.Content != expectedFormat {
				t.Errorf("Expected summary format: %s, got: %s", expectedFormat, m.Content)
			}
		}
	}

	if !foundSummary {
		t.Error("Expected long tool result to be replaced with summary during aggressive compaction")
	}
}

func TestIsResultNotFound(t *testing.T) {
	tests := []struct {
		content  string
		expected bool
	}{
		{"file not found", true},
		{"No such file or directory", true},
		{"Path does not exist", true},
		{"no results found", true},
		{"No matches for query", true},
		{"0 files found", true},
		{"Empty result set", true},
		{"found 3 files", false},
		{"success", false},
		{"here is the content", false},
	}

	for _, tt := range tests {
		result := agentctx.IsResultNotFound(tt.content)
		if result != tt.expected {
			t.Errorf("IsResultNotFound(%q) = %v, expected %v", tt.content, result, tt.expected)
		}
	}
}