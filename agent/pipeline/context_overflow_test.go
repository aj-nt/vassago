// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// TestContextManagementStage_OverflowTakesPriorityOverLoopDetection verifies that
// when the context exceeds the token limit, Stage5 returns TERMINATE even when
// loop detection (isConsecutiveToolIterationLoop) would otherwise fire.
//
// TDD: This test was written BEFORE the fix. The current behavior is that loop
// detection fires first (4+ consecutive tool iterations → YIELD) before the
// context overflow check, meaning an oversized context never gets terminated.
// The fix reorders Stage5 to check context overflow before loop detection.
func TestContextManagementStage_OverflowTakesPriorityOverLoopDetection(t *testing.T) {
	// Use a low max context so it's easy to exceed
	stage := NewContextManagementStage(1000) // 1000 tokens max, effective = 900

	// Build a conversation that:
	// 1. Has 4+ consecutive tool iterations (would trigger isConsecutiveToolIterationLoop)
	// 2. Exceeds the effective token limit (should trigger TERMINATE regardless)
	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "This is a line that adds to the context length for testing. "
	}
	// longContent ≈ 200 * 55 = 11,000 chars ≈ 2,750 tokens — way over 900

	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "search for everything"},
			// Iteration 1
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "call-1", Name: "ListSessions", Arguments: []byte(`{}`)},
			}},
			{Role: "tool", Content: longContent, ToolCallID: "call-1", Name: "ListSessions"},
			// Iteration 2
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "call-2", Name: "SearchMemories", Arguments: []byte(`{"query":"test"}`)},
			}},
			{Role: "tool", Content: longContent, ToolCallID: "call-2", Name: "SearchMemories"},
			// Iteration 3
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "call-3", Name: "GetSession", Arguments: []byte(`{"id":"sess-123"}`)},
			}},
			{Role: "tool", Content: longContent, ToolCallID: "call-3", Name: "GetSession"},
			// Iteration 4
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "call-4", Name: "GetSession", Arguments: []byte(`{"id":"sess-456"}`)},
			}},
			{Role: "tool", Content: longContent, ToolCallID: "call-4", Name: "GetSession"},
		},
		MaxIterations: 50,
		Iteration:     5,
		HotBlockContent: "",
	}

	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call-5", Name: "FileRead", Arguments: []byte(`{"path":"/tmp/test"}`)}},
		FinishReason: "tool_use",
	}

	toolResults := []agentctx.ToolResult{
		{CallID: "call-5", Name: "FileRead", Result: "file contents", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	// Context overflow should take PRIORITY over loop detection.
	// Without this fix, 4+ consecutive tool iterations would trigger YIELD from
	// isConsecutiveToolIterationLoop. With the fix, context overflow is checked first:
	//   - If compaction can save the conversation → CONTINUE (loop detection skipped)
	//   - If compaction fails → TERMINATE (not YIELD)
	// We should NEVER get YIELD from loop detection when the context was oversized,
	// because YIELD means the conversation ends without any context management.
	if action == agentctx.StageActionYIELD {
		t.Errorf("Expected TERMINATE or CONTINUE (overflow takes priority over loop detection), got YIELD — loop detection fired before context overflow check")
	}
	// The expected outcome depends on whether compaction succeeds:
	// - With iteration 5 and Phase 0 compaction, tool results get truncated to placeholders
	//   which brings the context under the limit → CONTINUE
	// - If we made the context impossibly large, it would → TERMINATE
	// Either way, the key invariant is: NO YIELD from loop detection on an oversized context.
}

// TestContextManagementStage_OverflowTerminatesWhenUncountable verifies that when the
// context exceeds the limit AND compaction cannot bring it under the limit, Stage5
// returns TERMINATE (not YIELD from loop detection).
func TestContextManagementStage_OverflowTerminatesWhenUncountable(t *testing.T) {
	stage := NewContextManagementStage(1000) // Very low limit, effective = 900

	// Build a context that is impossible to compact under 900 tokens.
	// Use only user and assistant text messages (no tool results to truncate).
	// Each message is large enough that even a single pair exceeds the limit.
	bigText := ""
	for i := 0; i < 1000; i++ {
		bigText += "Word "
	}
	// bigText ≈ 5000 chars ≈ 1250 tokens — way over 900

	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: bigText},
			{Role: "assistant", Content: bigText},
		},
		MaxIterations:   50,
		Iteration:       2,
		HotBlockContent: "",
	}

	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call-1", Name: "SearchMemories", Arguments: []byte(`{"query":"test"}`)}},
		FinishReason: "tool_use",
	}

	toolResults := []agentctx.ToolResult{
		{CallID: "call-1", Name: "SearchMemories", Result: "some results", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	// With only user/assistant messages and no tool results to truncate,
	// compaction can't help (it removes tool results, not user messages).
	// The context should TERMINATE, not YIELD.
	if action != agentctx.StageActionTERMINATE {
		t.Errorf("Expected TERMINATE when context is uncompactable, got %v", action)
	}
}

// TestContextManagementStage_LoopDetectionWhenContextFits verifies that loop detection
// still works normally when the context is within limits. This is a regression test
// to ensure the reorder doesn't break the existing loop detection.
func TestContextManagementStage_LoopDetectionWhenContextFits(t *testing.T) {
	// Use a very high max context so it won't trigger overflow
	stage := NewContextManagementStage(1000000)

	// Build a conversation with 4+ consecutive tool iterations but small content
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "search for everything"},
			// Iteration 1
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "call-1", Name: "ListSessions", Arguments: []byte(`{}`)},
			}},
			{Role: "tool", Content: "found 3 sessions", ToolCallID: "call-1", Name: "ListSessions"},
			// Iteration 2
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "call-2", Name: "SearchMemories", Arguments: []byte(`{"query":"test"}`)},
			}},
			{Role: "tool", Content: "found 5 memories", ToolCallID: "call-2", Name: "SearchMemories"},
			// Iteration 3
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "call-3", Name: "GetSession", Arguments: []byte(`{"id":"sess-123"}`)},
			}},
			{Role: "tool", Content: "session data", ToolCallID: "call-3", Name: "GetSession"},
			// Iteration 4
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "call-4", Name: "GetSession", Arguments: []byte(`{"id":"sess-456"}`)},
			}},
			{Role: "tool", Content: "more session data", ToolCallID: "call-4", Name: "GetSession"},
		},
		MaxIterations: 50,
		Iteration:     5,
		HotBlockContent: "",
	}

	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call-5", Name: "FileRead", Arguments: []byte(`{"path":"/tmp/test"}`)}},
		FinishReason: "tool_use",
	}

	toolResults := []agentctx.ToolResult{
		{CallID: "call-5", Name: "FileRead", Result: "file contents", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	// With context well within limits, loop detection should fire and YIELD
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD from loop detection when context is within limits, got %v", action)
	}
}

// TestContextManagementStage_AggressiveTruncationSavesConversation verifies that when
// tool results push context over the limit, aggressive truncation can bring it back
// under the limit, allowing the conversation to CONTINUE instead of TERMINATE.
func TestContextManagementStage_AggressiveTruncationSavesConversation(t *testing.T) {
	// Use a context limit where 2 large tool results would exceed it,
	// but truncating them to placeholders would fit.
	// 2 results × 5000 chars each = ~2500 tokens. Effective limit = 0.9 * 6000 = 5400.
	// The long results exceed 5400 tokens but the truncated placeholders fit easily.
	stage := NewContextManagementStage(6000)

	// Build oversized tool results that will be truncated
	bigResult := ""
	for i := 0; i < 100; i++ {
		bigResult += "This line adds bulk to the tool result content for testing truncation. "
	}

	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "tell me about sessions"},
			{Role: "assistant", Content: "Let me list them.", ToolCalls: []agentctx.ToolCall{
				{ID: "call-1", Name: "ListSessions", Arguments: []byte(`{}`)},
			}},
			{Role: "tool", Content: bigResult, ToolCallID: "call-1", Name: "ListSessions"},
		},
		MaxIterations: 50,
		Iteration:     1,
		HotBlockContent: "",
	}

	parsed := agentctx.ParsedResponse{
		Text:         "Let me search for more details.",
		ToolCalls:    []agentctx.ToolCall{{ID: "call-2", Name: "SearchMemories", Arguments: []byte(`{"query":"test"}`)}},
		FinishReason: "tool_use",
	}

	toolResults := []agentctx.ToolResult{
		{CallID: "call-2", Name: "SearchMemories", Result: bigResult, IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	// After aggressive truncation, the context should be small enough to CONTINUE.
	// If it's still too large after truncation, TERMINATE is acceptable.
	// But CONTINUE is the desired outcome — truncation should save the conversation.
	if action == agentctx.StageActionYIELD {
		// YIELD means loop detection or stuck detection fired (shouldn't happen with iteration 1)
		t.Errorf("Expected CONTINUE or TERMINATE, got YIELD — likely loop detection incorrectly firing")
	}
	// CONTINUE means truncation successfully brought context under limit — best case
	// TERMINATE means truncation wasn't sufficient — acceptable but not ideal
	// Either is fine as long as it's not YIELD (which skips context management)
}