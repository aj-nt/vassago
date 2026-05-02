package pipeline

import (
	"encoding/json"
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
	// Build a context with old tool calls that should be compacted
	args, _ := json.Marshal(map[string]interface{}{})

	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "What's my name?"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "SearchMemories", Arguments: args}}},
			{Role: "tool", Content: "Found: AJ", ToolCallID: "1", Name: "SearchMemories"},
			{Role: "assistant", Content: "Your name is AJ."},
			{Role: "user", Content: "What's the weather?"},
			{Role: "assistant", Content: "I don't have weather data."},
		},
		MaxIterations: 10,
		Iteration:      0,
	}

	// Trigger compaction with a low token limit
	compactContext(convCtx, 50) // very low limit

	// The old tool result should be removed
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
	if isRepetitiveToolLoop(nil, currentCalls) {
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
	if isRepetitiveToolLoop(messages, currentCalls) {
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
	if !isRepetitiveToolLoop(messages, currentCalls) {
		t.Error("Three consecutive same-tool calls should be detected as a loop")
	}
}

func TestIsRepetitiveToolLoop_TextBreaksLoop(t *testing.T) {
	// Text-only assistant response (no tool calls) between iterations breaks the loop.
	// The model gave a real answer, then later decided to use the tool again — that's fine.
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
	if isRepetitiveToolLoop(messages, currentCalls) {
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
	if !isRepetitiveToolLoop(messages, currentCalls) {
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
	if !isConsecutiveToolIterationLoop(messages, currentCalls) {
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
	if isConsecutiveToolIterationLoop(messages, currentCalls) {
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
	if isConsecutiveToolIterationLoop(messages, currentCalls) {
		t.Error("Expected text-only response to break the loop")
	}
}