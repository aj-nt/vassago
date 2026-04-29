package pipeline

import (
	"encoding/json"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

func TestNewContextManagementStage(t *testing.T) {
	stage := NewContextManagementStage(1000)
	if stage == nil {
		t.Error("NewContextManagementStage should return a non-nil stage")
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