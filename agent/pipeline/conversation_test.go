// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// Conversation simulation tests validate the complete pipeline behavior
// for common conversation patterns that caused bugs in production.

// TestConversation_SimpleQuestionNoTools simulates a simple question that
// should be answered directly without any tool calls. The agent should
// YIELD immediately with the text response.
func TestConversation_SimpleQuestionNoTools(t *testing.T) {
	stage := NewContextManagementStage(0)
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "What is 2+2?"}},
		MaxIterations: 10,
		Iteration:     1,
	}

	// Model responds with just text, no tool calls
	parsed := agentctx.ParsedResponse{
		Text:         "4",
		FinishReason: "stop",
	}

	action := stage.Process(convCtx, nil, parsed)
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD for simple text response, got %v", action)
	}
}

// TestConversation_ToolCallThenSuccess simulates a normal tool-call-then-result
// workflow: the model calls a tool, gets a result, and should continue.
func TestConversation_ToolCallThenSuccess(t *testing.T) {
	stage := NewContextManagementStage(0)
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "search for my name"}},
		MaxIterations: 10,
		Iteration:     1,
	}

	args, _ := json.Marshal(map[string]interface{}{"query": "name"})
	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call1", Name: "SearchMemories", Arguments: args}},
		FinishReason: "tool_use",
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "call1", Name: "SearchMemories", Result: "Your name is AJ", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)
	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE after successful tool call, got %v", action)
	}
}

// TestConversation_FileSearchNotFound simulates the common pattern where
// FileSearch returns no results and the agent should not loop trying again.
// The first search failed (in history), and the model tries again (new call
// that also returns "no results"). The combination should trigger stuck detection.
func TestConversation_FileSearchNotFound(t *testing.T) {
	stage := NewContextManagementStage(0)

	// History: first search returned "0 files found", then model tried again
	// and the second search also returned "no results"
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "find the config file"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "1", Name: "FileSearch"},
			}},
			{Role: "tool", Content: "0 files found", ToolCallID: "1", Name: "FileSearch"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "2", Name: "FileSearch"},
			}},
			{Role: "tool", Content: "no results", ToolCallID: "2", Name: "FileSearch"},
		},
		MaxIterations: 10,
		Iteration:     3,
	}

	args, _ := json.Marshal(map[string]interface{}{"pattern": "config"})
	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "3", Name: "FileSearch", Arguments: args},
		},
		FinishReason: "tool_use",
	}

	// Third search also returns nothing — should be detected as stuck
	// via isStuckFromHistory since the history already has 2 "not found" results
	action := stage.Process(convCtx, nil, parsed)
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD after repeated failed searches (stuck detected), got %v", action)
	}

	// Should have a helpful message
	lastMsg := convCtx.Messages[len(convCtx.Messages)-1]
	if lastMsg.Role != "assistant" {
		t.Errorf("Expected last message to be from assistant, got %s", lastMsg.Role)
	}
}

// TestConversation_ToolListingRepetition simulates the pattern where the
// model keeps calling the same tool (GetHotBlock) on every iteration.
func TestConversation_ToolListingRepetition(t *testing.T) {
	thresholds := DefaultThresholds()
	thresholds.RepetitiveLoopThreshold = 3 // re-enable deprecated handler for this test
	stage := NewContextManagementStageWithThresholds(0, thresholds)

	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "Let me check.", ToolCalls: []agentctx.ToolCall{
				{ID: "1", Name: "GetHotBlock", Arguments: args},
			}},
			{Role: "tool", Content: "hot block: AJ, likes Go", ToolCallID: "1", Name: "GetHotBlock"},
			{Role: "assistant", Content: "I see.", ToolCalls: []agentctx.ToolCall{
				{ID: "2", Name: "GetHotBlock", Arguments: args},
			}},
			{Role: "tool", Content: "hot block: AJ, likes Go", ToolCallID: "2", Name: "GetHotBlock"},
		},
		MaxIterations: 10,
		Iteration:     3,
	}

	parsed := agentctx.ParsedResponse{
		Text: "Let me check again.",
		ToolCalls: []agentctx.ToolCall{
			{ID: "3", Name: "GetHotBlock", Arguments: args},
		},
		FinishReason: "tool_use",
	}

	action := stage.Process(convCtx, nil, parsed)
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD (break repetitive loop), got %v", action)
	}
}

// TestConversation_CrossToolLoop simulates the pattern where the model
// cycles through different tools back to the same one (FileSearch → FileRead → Execute → FileSearch).
// With progress detection, this IS detected as a duplicate since FileSearch appeared
// earlier in the progress history.
func TestConversation_CrossToolLoop(t *testing.T) {
	stage := NewContextManagementStage(0)

	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "check the logs"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "1", Name: "FileSearch", Arguments: args},
			}},
			{Role: "tool", Content: "found 3 files", ToolCallID: "1", Name: "FileSearch"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "2", Name: "FileRead", Arguments: args},
			}},
			{Role: "tool", Content: "file contents...", ToolCallID: "2", Name: "FileRead"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "3", Name: "Execute", Arguments: args},
			}},
			{Role: "tool", Content: "command output...", ToolCallID: "3", Name: "Execute"},
		},
		MaxIterations: 10,
		Iteration:     4,
		HotBlockContent: "",
		ProgressHistory: []agentctx.ProgressSignature{
			{ToolNames: "FileSearch", ResultTag: "success"},
			{ToolNames: "FileRead", ResultTag: "success"},
			{ToolNames: "Execute", ResultTag: "success"},
		},
		DuplicateProgressCount: 0, // Fresh start — FileSearch hasn't been seen recently as duplicate
	}

	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "4", Name: "FileSearch", Arguments: args},
		},
		FinishReason: "tool_use",
	}

	toolResults := []agentctx.ToolResult{
		{CallID: "4", Name: "FileSearch", Result: "found 3 files", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	// FileSearch appears in the history but with "success" result, and the
	// current call is FileSearch with "success" — this IS a duplicate of
	// the first history entry. After enough consecutive duplicates (configured by
	// ProgressYieldThreshold, default 5), it would YIELD. But here DuplicateProgressCount
	// is only 1 (first duplicate), so it should CONTINUE without any nudge.
	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE (first duplicate gets nudge, not YIELD), got %v", action)
	}
}

// TestConversation_NormalThreeStepWorkflow simulates a legitimate
// three-step workflow (search → read → execute) that should NOT be
// broken by loop detection, because each step produces text.
func TestConversation_NormalThreeStepWorkflow(t *testing.T) {
	stage := NewContextManagementStage(0)

	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "find and fix the bug"},
			{Role: "assistant", Content: "Let me search for the bug.", ToolCalls: []agentctx.ToolCall{
				{ID: "1", Name: "FileSearch", Arguments: args},
			}},
			{Role: "tool", Content: "found 3 files", ToolCallID: "1", Name: "FileSearch"},
			{Role: "assistant", Content: "Found the files. Let me read them.", ToolCalls: []agentctx.ToolCall{
				{ID: "2", Name: "FileRead", Arguments: args},
			}},
			{Role: "tool", Content: "file contents...", ToolCallID: "2", Name: "FileRead"},
		},
		MaxIterations: 10,
		Iteration:     3,
	}

	parsed := agentctx.ParsedResponse{
		Text: "Now I'll run the tests.",
		ToolCalls: []agentctx.ToolCall{
			{ID: "3", Name: "Execute", Arguments: args},
		},
		FinishReason: "tool_use",
	}

	toolResults := []agentctx.ToolResult{
		{CallID: "3", Name: "Execute", Result: "tests passed", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)
	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE for normal 3-step workflow with text, got %v", action)
	}
}

// TestConversation_StuckWithHelpfulMessage verifies that stuck detection
// adds a helpful message for the user when the agent can't find what it needs.
func TestConversation_StuckWithHelpfulMessage(t *testing.T) {
	stage := NewContextManagementStage(0)

	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "find the config file"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "1", Name: "FileSearch"},
			}},
			{Role: "tool", Content: "Error: path not found", ToolCallID: "1", Name: "FileSearch"},
		},
		MaxIterations: 10,
		Iteration:     2,
	}

	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "2", Name: "FileSearch", Arguments: args},
		},
		FinishReason: "tool_use",
	}

	// Two error results — agent is stuck
	toolResults := []agentctx.ToolResult{
		{CallID: "1", Name: "FileSearch", Result: "Error: path not found", IsError: true},
		{CallID: "2", Name: "FileSearch", Result: "Error: still not found", IsError: true},
	}

	action := stage.Process(convCtx, toolResults, parsed)
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD for stuck agent, got %v", action)
	}

	// Verify helpful message was added
	lastMsg := convCtx.Messages[len(convCtx.Messages)-1]
	if lastMsg.Role != "assistant" {
		t.Errorf("Expected assistant message, got %s", lastMsg.Role)
	}
	if len(lastMsg.Content) == 0 {
		t.Error("Expected helpful message content, got empty string")
	}
}

// TestConversation_DelegateEmptyOutput simulates a Delegate tool call
// where the sub-agent returns nothing. The ContextManagement stage should
// handle this gracefully.
func TestConversation_DelegateEmptyOutput(t *testing.T) {
	stage := NewContextManagementStage(0)

	args, _ := json.Marshal(map[string]interface{}{"prompt": "analyze this"})
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "delegate this task"}},
		MaxIterations: 10,
		Iteration:     1,
	}

	parsed := agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call1", Name: "Delegate", Arguments: args}},
		FinishReason: "tool_use",
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "call1", Name: "Delegate", Result: "sub-agent completed but returned no output", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)
	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE after delegate with empty output, got %v", action)
	}

	// Verify the tool result was appended
	found := false
	for _, m := range convCtx.Messages {
		if m.Role == "tool" && m.Name == "Delegate" {
			found = true
		}
	}
	if !found {
		t.Error("Expected Delegate tool result to be appended to messages")
	}
}

// TestConversation_IntentLoopCrossTool simulates the key new case:
// the agent switches between different tools but with the same stated purpose
// ("Let me search..."). The old progress detection missed this because the
// tool names differ (FileSearch ≠ SearchMemories ≠ Execute).
// The intent loop detection catches it because "search" appears 5+ times.
func TestConversation_IntentLoopCrossTool(t *testing.T) {
	stage := NewContextManagementStage(0)

	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "find the bug"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "1", Name: "FileSearch", Arguments: args},
			}},
			{Role: "tool", Content: "found 3 files", ToolCallID: "1", Name: "FileSearch"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "2", Name: "SearchMemories", Arguments: args},
			}},
			{Role: "tool", Content: "no results found", ToolCallID: "2", Name: "SearchMemories"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "3", Name: "Execute", Arguments: args},
			}},
			{Role: "tool", Content: "output: grep found nothing", ToolCallID: "3", Name: "Execute"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "4a", Name: "FileSearch", Arguments: args},
			}},
			{Role: "tool", Content: "found 2 files", ToolCallID: "4a", Name: "FileSearch"},
			{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
				{ID: "4b", Name: "Execute", Arguments: args},
			}},
			{Role: "tool", Content: "output: grep found nothing", ToolCallID: "4b", Name: "Execute"},
		},
		MaxIterations: 10,
		Iteration:     6,
		ProgressHistory: []agentctx.ProgressSignature{
			{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
			{ToolNames: "SearchMemories", ResultTag: "not_found", IntentTag: "search"},
			{ToolNames: "Execute", ResultTag: "success", IntentTag: "search"},
			{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
			{ToolNames: "Execute", ResultTag: "success", IntentTag: "search"},
		},
		DuplicateProgressCount: 0, // Different signatures — not duplicate!
	}

	// 6th iteration: still searching, different tool
	parsed := agentctx.ParsedResponse{
		Text: "Let me search the codebase again.",
		ToolCalls: []agentctx.ToolCall{
			{ID: "6", Name: "FileSearch", Arguments: args},
		},
		FinishReason: "tool_use",
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "6", Name: "FileSearch", Result: "found 5 files", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	// Should CONTINUE (intent nudge injects a system message, doesn't YIELD)
	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE (intent nudge, not yield), got %v", action)
	}

	// Verify an intent nudge was injected
	foundNudge := false
	for _, m := range convCtx.Messages {
		if m.Role == "user" && strings.Contains(m.Content, "searching") {
			foundNudge = true
		}
	}
	if !foundNudge {
		t.Error("Expected intent nudge about 'searching' to be injected")
	}
}

// TestConversation_IntentLoopDifferentGoals verifies that different intents
// (search → read → execute) do NOT trigger intent loop detection, even with
// repeated tool calls. Each step has a distinct purpose.
func TestConversation_IntentLoopDifferentGoals(t *testing.T) {
	stage := NewContextManagementStage(0)

	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "audit the code"},
		},
		MaxIterations: 10,
		Iteration:     4,
		ProgressHistory: []agentctx.ProgressSignature{
			{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
			{ToolNames: "FileRead", ResultTag: "success", IntentTag: "read"},
			{ToolNames: "Execute", ResultTag: "success", IntentTag: "execute"},
		},
		DuplicateProgressCount: 0,
	}

	// 4th iteration: writing a fix — different goal entirely
	parsed := agentctx.ParsedResponse{
		Text: "Now I'll fix the issue.",
		ToolCalls: []agentctx.ToolCall{
			{ID: "4", Name: "FileWrite", Arguments: args},
		},
		FinishReason: "tool_use",
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "4", Name: "FileWrite", Result: "saved", IsError: false},
	}

	action := stage.Process(convCtx, toolResults, parsed)

	// Should CONTINUE — no loop detection, no nudge
	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE (different goals, no loop), got %v", action)
	}

	// No nudge should be injected
	for _, m := range convCtx.Messages {
		if m.Role == "user" && strings.HasPrefix(m.Content, "[system:") {
			t.Errorf("Unexpected nudge in messages: %s", m.Content)
		}
	}
}