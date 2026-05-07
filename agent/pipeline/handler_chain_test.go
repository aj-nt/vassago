// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
)

// --- OverflowHandler tests ---

func TestOverflowHandler_PassesWhenUnderLimit(t *testing.T) {
	handler := &OverflowHandler{MaxContextTokens: 100000, Thresholds: DefaultThresholds()}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 10,
	}
	action, handled := handler.Handle(convCtx, nil, parsedWithToolCalls())
	if handled {
		t.Errorf("Expected handler to pass through when under limit, got action=%v handled=%v", action, handled)
	}
}

func TestOverflowHandler_TerminatesWhenOverLimitAndCannotCompact(t *testing.T) {
	handler := &OverflowHandler{MaxContextTokens: 10, Thresholds: DefaultThresholds()} // very low limit
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: strings.Repeat("data ", 10000)}, // huge context
		},
		MaxIterations: 100,
	}
	action, handled := handler.Handle(convCtx, nil, parsedWithToolCalls())
	if !handled {
		t.Error("Expected handler to handle the request")
	}
	if action != agentctx.StageActionTERMINATE {
		t.Errorf("Expected TERMINATE when over limit and cannot compact, got %v", action)
	}
}

func TestOverflowHandler_CompactsAndContinuesWhenPossible(t *testing.T) {
	handler := &OverflowHandler{MaxContextTokens: 200, Thresholds: DefaultThresholds()} // moderate limit
	// Build a compactable context
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "Tell me everything"},
			{Role: "assistant", Content: "Let me search", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "SearchMemories", Arguments: json.RawMessage(`{}`)}}},
			{Role: "tool", Content: strings.Repeat("result data ", 100), ToolCallID: "1", Name: "SearchMemories"},
			{Role: "assistant", Content: "Based on my search..."},
			{Role: "user", Content: "Thanks, now something else"},
		},
		MaxIterations: 10,
		Iteration:     0,
	}
	action, handled := handler.Handle(convCtx, nil, parsedWithToolCalls())
	// Handler should not handle (compaction succeeded, pass to next handler)
	if handled {
		t.Errorf("Expected handler to pass through after successful compaction, got action=%v handled=%v", action, handled)
	}
	// But it should have set CompactedThisTurn
	if !convCtx.CompactedThisTurn {
		t.Error("Expected CompactedThisTurn to be set after compaction")
	}
}

// --- StuckHandler tests ---

func TestStuckHandler_PassesWhenNotStuck(t *testing.T) {
	handler := &StuckHandler{Thresholds: DefaultThresholds()}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 10,
	}
	action, handled := handler.Handle(convCtx, nil, parsedWithToolCalls())
	if handled {
		t.Errorf("Expected handler to pass through when not stuck, got action=%v handled=%v", action, handled)
	}
}

func TestStuckHandler_YieldsOnConsecutiveErrors(t *testing.T) {
	handler := &StuckHandler{Thresholds: DefaultThresholds()}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 10,
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "1", Name: "Tool1", Result: "error 1", IsError: true},
		{CallID: "2", Name: "Tool2", Result: "error 2", IsError: true},
	}
	action, handled := handler.Handle(convCtx, toolResults, parsedWithToolCalls())
	if !handled {
		t.Error("Expected handler to handle the stuck request")
	}
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD when stuck, got %v", action)
	}
}

func TestStuckHandler_YieldsOnConsecutiveNotFound(t *testing.T) {
	handler := &StuckHandler{Thresholds: DefaultThresholds()}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 10,
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "1", Name: "Tool1", Result: "file not found", IsError: false},
		{CallID: "2", Name: "Tool2", Result: "no such file", IsError: false},
	}
	action, handled := handler.Handle(convCtx, toolResults, parsedWithToolCalls())
	if !handled {
		t.Error("Expected handler to handle the stuck request")
	}
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD when stuck, got %v", action)
	}
}

// --- RepetitiveLoopHandler tests ---

func TestRepetitiveLoopHandler_DetectsLoop(t *testing.T) {
	thresholds := DefaultThresholds()
	thresholds.RepetitiveLoopThreshold = 3 // re-enable for this test
	handler := &RepetitiveLoopHandler{Thresholds: thresholds}
	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "check memory"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "GetHotBlock", Arguments: args}}},
			{Role: "tool", Content: "hot block data", ToolCallID: "1"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "GetHotBlock", Arguments: args}}},
			{Role: "tool", Content: "hot block data", ToolCallID: "2"},
		},
		MaxIterations: 20,
		Iteration:     3,
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "3", Name: "GetHotBlock", Arguments: args}},
		FinishReason: "tool_use",
	}
	action, handled := handler.Handle(convCtx, nil, parsed)
	if !handled {
		t.Error("Expected handler to handle the loop request")
	}
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD when repetitive loop detected, got %v", action)
	}
}

func TestRepetitiveLoopHandler_SkipsAfterCompaction(t *testing.T) {
	thresholds := DefaultThresholds()
	thresholds.RepetitiveLoopThreshold = 3 // re-enable for this test
	handler := &RepetitiveLoopHandler{Thresholds: thresholds}
	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages:          []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations:     20,
		CompactedThisTurn: true, // should skip loop detection
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "3", Name: "GetHotBlock", Arguments: args}},
		FinishReason: "tool_use",
	}
	action, handled := handler.Handle(convCtx, nil, parsed)
	if handled {
		t.Errorf("Expected handler to skip loop detection after compaction, got action=%v handled=%v", action, handled)
	}
}

// --- ConsecutiveLoopHandler tests ---

func TestConsecutiveLoopHandler_DetectsLoop(t *testing.T) {
	thresholds := DefaultThresholds()
	thresholds.ConsecutiveLoopThreshold = 3 // re-enable deprecated handler for this test
	handler := &ConsecutiveLoopHandler{Thresholds: thresholds}
	messages := []agentctx.Message{
		{Role: "user", Content: "check this"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FileSearch", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", Content: "found files", ToolCallID: "1"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "FileRead", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", Content: "file contents", ToolCallID: "2"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "3", Name: "Execute", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", Content: "command output", ToolCallID: "3"},
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      messages,
		MaxIterations: 20,
		Iteration:     4,
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "4", Name: "FileSearch", Arguments: json.RawMessage(`{}`)}},
		FinishReason: "tool_use",
	}
	action, handled := handler.Handle(convCtx, nil, parsed)
	if !handled {
		t.Error("Expected handler to detect consecutive loop")
	}
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD, got %v", action)
	}
}

func TestConsecutiveLoopHandler_SkipsAfterCompaction(t *testing.T) {
	handler := &ConsecutiveLoopHandler{Thresholds: DefaultThresholds()}
	convCtx := &agentctx.ConversationContext{
		Messages:          []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations:     20,
		CompactedThisTurn: true,
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "4", Name: "FileSearch", Arguments: json.RawMessage(`{}`)}},
		FinishReason: "tool_use",
	}
	action, handled := handler.Handle(convCtx, nil, parsed)
	if handled {
		t.Errorf("Expected handler to skip after compaction, got action=%v handled=%v", action, handled)
	}
}

// --- AppendAndContinueHandler tests ---

func TestAppendAndContinueHandler_AppendsMessages(t *testing.T) {
	handler := &AppendAndContinueHandler{MaxContextTokens: 0, Thresholds: DefaultThresholds()}
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

	action, handled := handler.Handle(convCtx, toolResults, parsed)
	if !handled {
		t.Error("Expected handler to handle the request")
	}
	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE, got %v", action)
	}
	// Verify messages were appended
	if len(convCtx.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(convCtx.Messages))
	}
	if convCtx.Messages[1].Role != "assistant" {
		t.Errorf("Expected second message to be assistant, got %s", convCtx.Messages[1].Role)
	}
	if convCtx.Messages[2].Role != "tool" {
		t.Errorf("Expected third message to be tool, got %s", convCtx.Messages[2].Role)
	}
}

func TestAppendAndContinueHandler_TerminatesOnMaxIterations(t *testing.T) {
	handler := &AppendAndContinueHandler{MaxContextTokens: 0, Thresholds: DefaultThresholds()}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 2,
		Iteration:     2, // at limit
	}
	parsed := parsedWithToolCalls()

	action, handled := handler.Handle(convCtx, nil, parsed)
	if !handled {
		t.Error("Expected handler to handle the request")
	}
	if action != agentctx.StageActionTERMINATE {
		t.Errorf("Expected TERMINATE when max iterations reached, got %v", action)
	}
}

func TestAppendAndContinueHandler_TerminatesOnOverflowAfterAppend(t *testing.T) {
	handler := &AppendAndContinueHandler{MaxContextTokens: 10, Thresholds: DefaultThresholds()} // very low limit
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: strings.Repeat("data ", 1000)}, // already over limit
		},
		MaxIterations: 10,
		Iteration:     1,
	}
	parsed := parsedWithToolCalls()

	action, handled := handler.Handle(convCtx, nil, parsed)
	if !handled {
		t.Error("Expected handler to handle the request")
	}
	if action != agentctx.StageActionTERMINATE {
		t.Errorf("Expected TERMINATE when overflow after append, got %v", action)
	}
}

// --- HandlerChain tests ---

func TestHandlerChain_CallsHandlersInOrder(t *testing.T) {
	// Create a chain where the overflow handler stops the request
	chain := NewHandlerChain(
		&OverflowHandler{MaxContextTokens: 10, Thresholds: DefaultThresholds()}, // very low limit
		&StuckHandler{Thresholds: DefaultThresholds()},
		&RepetitiveLoopHandler{Thresholds: DefaultThresholds()},
		&ConsecutiveLoopHandler{Thresholds: DefaultThresholds()},
		&AppendAndContinueHandler{},
	)

	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: strings.Repeat("data ", 10000)}, // huge context
		},
		MaxIterations: 100,
	}

	action := chain.Handle(convCtx, nil, parsedWithToolCalls())
	if action != agentctx.StageActionTERMINATE {
		t.Errorf("Expected TERMINATE from overflow handler, got %v", action)
	}
}

func TestHandlerChain_PassesThroughWhenFirstHandlerDeclines(t *testing.T) {
	chain := NewHandlerChain(
		&OverflowHandler{MaxContextTokens: 0, Thresholds: DefaultThresholds()}, // no limit → passes through
		&StuckHandler{Thresholds: DefaultThresholds()},                        // no stuck results → passes through
		&RepetitiveLoopHandler{Thresholds: DefaultThresholds()},               // not repetitive → passes through
		&ConsecutiveLoopHandler{Thresholds: DefaultThresholds()},              // not consecutive → passes through
		&AppendAndContinueHandler{MaxContextTokens: 0, Thresholds: DefaultThresholds()},
	)

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

	action := chain.Handle(convCtx, toolResults, parsed)
	if action != agentctx.StageActionCONTINUE {
		t.Errorf("Expected CONTINUE when all handlers pass through, got %v", action)
	}
}

// --- Integration: ContextManagementStage.Process uses the chain ---

func TestContextManagementStage_ResetsCompactedFlag(t *testing.T) {
	stage := NewContextManagementStage(0) // no token limit
	convCtx := &agentctx.ConversationContext{
		Messages:          []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations:     5,
		Iteration:         0,
		CompactedThisTurn: true, // should be reset at start of Process
	}

	parsed := agentctx.ParsedResponse{
		Text:         "Hello!",
		ToolCalls:    nil, // no tool calls → YIELD
		FinishReason: "stop",
	}

	action := stage.Process(convCtx, nil, parsed)
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD for text-only response, got %v", action)
	}
	// CompactedThisTurn should have been reset
	if convCtx.CompactedThisTurn {
		t.Error("Expected CompactedThisTurn to be reset to false")
	}
}

// --- Config override tests ---

func TestPipelineThresholds_CustomRepetitiveThreshold(t *testing.T) {
	// With a higher threshold (5), 3 identical calls should NOT trigger loop detection
	thresholds := DefaultThresholds()
	thresholds.RepetitiveLoopThreshold = 5

	handler := &RepetitiveLoopHandler{Thresholds: thresholds}
	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "check memory"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "GetHotBlock", Arguments: args}}},
			{Role: "tool", Content: "hot block data", ToolCallID: "1"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "GetHotBlock", Arguments: args}}},
			{Role: "tool", Content: "hot block data", ToolCallID: "2"},
		},
		MaxIterations: 20,
		Iteration:     3,
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "3", Name: "GetHotBlock", Arguments: args}},
		FinishReason: "tool_use",
	}
	action, handled := handler.Handle(convCtx, nil, parsed)
	if handled {
		t.Errorf("Expected handler to pass with threshold=5 and only 3 calls, got action=%v handled=%v", action, handled)
	}
}

func TestPipelineThresholds_CustomConsecutiveThreshold(t *testing.T) {
	// With threshold=3, 3 consecutive iterations should trigger loop detection
	thresholds := DefaultThresholds()
	thresholds.ConsecutiveLoopThreshold = 3

	handler := &ConsecutiveLoopHandler{Thresholds: thresholds}
	messages := []agentctx.Message{
		{Role: "user", Content: "check this"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FileSearch", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", Content: "found files", ToolCallID: "1"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "FileRead", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", Content: "file contents", ToolCallID: "2"},
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      messages,
		MaxIterations: 20,
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "3", Name: "Execute", Arguments: json.RawMessage(`{}`)}},
		FinishReason: "tool_use",
	}
	action, handled := handler.Handle(convCtx, nil, parsed)
	if !handled {
		t.Error("Expected handler to detect loop with threshold=3 and 3 consecutive calls")
	}
	if action != agentctx.StageActionYIELD {
		t.Errorf("Expected YIELD, got %v", action)
	}
}

func TestDefaultThresholds(t *testing.T) {
	th := DefaultThresholds()
	if th.AggressiveSummaryThreshold != 500 {
		t.Errorf("Expected AggressiveSummaryThreshold=500, got %d", th.AggressiveSummaryThreshold)
	}
	if th.RepetitiveLoopThreshold != 0 {
		t.Errorf("Expected RepetitiveLoopThreshold=0 (disabled, superseded by ProgressDetection), got %d", th.RepetitiveLoopThreshold)
	}
	if th.ProgressYieldThreshold != 5 {
		t.Errorf("Expected ProgressYieldThreshold=5, got %d", th.ProgressYieldThreshold)
	}
	if th.ConsecutiveLoopThreshold != 0 {
		t.Errorf("Expected ConsecutiveLoopThreshold=0 (disabled, superseded by ProgressYieldThreshold), got %d", th.ConsecutiveLoopThreshold)
	}
	if th.NarrationNudgeThreshold != 0 {
		t.Errorf("Expected NarrationNudgeThreshold=0 (disabled, superseded by ProgressDetection), got %d", th.NarrationNudgeThreshold)
	}
	if th.StuckMinFailures != 2 {
		t.Errorf("Expected StuckMinFailures=2, got %d", th.StuckMinFailures)
	}
	if th.StuckHistoryWindow != 3 {
		t.Errorf("Expected StuckHistoryWindow=3, got %d", th.StuckHistoryWindow)
	}
	if th.MaxConsecutiveErrors != 3 {
		t.Errorf("Expected MaxConsecutiveErrors=3, got %d", th.MaxConsecutiveErrors)
	}
}

func TestBlockingThresholds(t *testing.T) {
	th := BlockingThresholds()
	if th.MaxConsecutiveErrors != 1 {
		t.Errorf("Expected MaxConsecutiveErrors=1 for blocking mode, got %d", th.MaxConsecutiveErrors)
	}
	// Other thresholds should match defaults
	if th.RepetitiveLoopThreshold != 0 {
		t.Errorf("Expected RepetitiveLoopThreshold=0 (disabled, superseded by ProgressDetection), got %d", th.RepetitiveLoopThreshold)
	}
}

// --- Observer event tests ---

func TestStuckHandler_EmitsEvent(t *testing.T) {
	bus := eventbus.NewEventBus()
	ch := bus.Subscribe("stuck")

	handler := &StuckHandler{Thresholds: DefaultThresholds(), EventBus: bus}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 10,
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "1", Name: "Tool1", Result: "error 1", IsError: true},
		{CallID: "2", Name: "Tool2", Result: "error 2", IsError: true},
	}

	action, handled := handler.Handle(convCtx, toolResults, parsedWithToolCalls())
	if !handled || action != agentctx.StageActionYIELD {
		t.Fatalf("Expected YIELD, handled=true, got %v, %v", action, handled)
	}

	select {
	case evt := <-ch:
		loopEvt, ok := evt.Data.(LoopDetectionEvent)
		if !ok {
			t.Fatalf("Expected LoopDetectionEvent, got %T", evt.Data)
		}
		if loopEvt.Kind != "stuck" {
			t.Errorf("Expected kind 'stuck', got %q", loopEvt.Kind)
		}
		if loopEvt.Iteration != 0 {
			t.Errorf("Expected iteration 0, got %d", loopEvt.Iteration)
		}
	default:
		t.Error("Expected to receive stuck event from EventBus")
	}
}

func TestRepetitiveLoopHandler_EmitsEvent(t *testing.T) {
	bus := eventbus.NewEventBus()
	ch := bus.Subscribe("repetitive_loop")

	// Use explicit thresholds — handler is disabled by default (threshold=0)
	thresholds := DefaultThresholds()
	thresholds.RepetitiveLoopThreshold = 3
	handler := &RepetitiveLoopHandler{Thresholds: thresholds, EventBus: bus}
	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "check memory"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "GetHotBlock", Arguments: args}}},
			{Role: "tool", Content: "hot block data", ToolCallID: "1"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "GetHotBlock", Arguments: args}}},
			{Role: "tool", Content: "hot block data", ToolCallID: "2"},
		},
		MaxIterations: 20,
		Iteration:     3,
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "3", Name: "GetHotBlock", Arguments: args}},
		FinishReason: "tool_use",
	}

	action, handled := handler.Handle(convCtx, nil, parsed)
	if !handled || action != agentctx.StageActionYIELD {
		t.Fatalf("Expected YIELD, handled=true, got %v, %v", action, handled)
	}

	select {
	case evt := <-ch:
		loopEvt, ok := evt.Data.(LoopDetectionEvent)
		if !ok {
			t.Fatalf("Expected LoopDetectionEvent, got %T", evt.Data)
		}
		if loopEvt.Kind != "repetitive_loop" {
			t.Errorf("Expected kind 'repetitive_loop', got %q", loopEvt.Kind)
		}
		if loopEvt.ToolName != "GetHotBlock" {
			t.Errorf("Expected tool name 'GetHotBlock', got %q", loopEvt.ToolName)
		}
	default:
		t.Error("Expected to receive repetitive_loop event from EventBus")
	}
}

func TestRepetitiveLoopHandler_DisabledByDefault(t *testing.T) {
	// When RepetitiveLoopThreshold=0 (default), handler should return CONTINUE
	bus := eventbus.NewEventBus()
	handler := &RepetitiveLoopHandler{Thresholds: DefaultThresholds(), EventBus: bus}
	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "check memory"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "GetHotBlock", Arguments: args}}},
			{Role: "tool", Content: "hot block data", ToolCallID: "1"},
			{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "GetHotBlock", Arguments: args}}},
			{Role: "tool", Content: "hot block data", ToolCallID: "2"},
		},
		MaxIterations: 20,
		Iteration:     3,
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "3", Name: "GetHotBlock", Arguments: args}},
		FinishReason: "tool_use",
	}

	action, handled := handler.Handle(convCtx, nil, parsed)
	if action != agentctx.StageActionCONTINUE || handled != false {
		t.Errorf("Expected CONTINUE/false when RepetitiveLoopThreshold=0, got %v/%v", action, handled)
	}
}

func TestConsecutiveLoopHandler_EmitsEvent(t *testing.T) {
	bus := eventbus.NewEventBus()
	ch := bus.Subscribe("consecutive_loop")

	thresholds := DefaultThresholds()
	thresholds.ConsecutiveLoopThreshold = 3 // re-enable deprecated handler for this test
	handler := &ConsecutiveLoopHandler{Thresholds: thresholds, EventBus: bus}
	messages := []agentctx.Message{
		{Role: "user", Content: "check this"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "FileSearch", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", Content: "found files", ToolCallID: "1"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "2", Name: "FileRead", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", Content: "file contents", ToolCallID: "2"},
		{Role: "assistant", ToolCalls: []agentctx.ToolCall{{ID: "3", Name: "Execute", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", Content: "command output", ToolCallID: "3"},
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      messages,
		MaxIterations: 20,
		Iteration:     4,
	}
	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "4", Name: "FileSearch", Arguments: json.RawMessage(`{}`)}},
		FinishReason: "tool_use",
	}

	action, handled := handler.Handle(convCtx, nil, parsed)
	if !handled || action != agentctx.StageActionYIELD {
		t.Fatalf("Expected YIELD, handled=true, got %v, %v", action, handled)
	}

	select {
	case evt := <-ch:
		loopEvt, ok := evt.Data.(LoopDetectionEvent)
		if !ok {
			t.Fatalf("Expected LoopDetectionEvent, got %T", evt.Data)
		}
		if loopEvt.Kind != "consecutive_loop" {
			t.Errorf("Expected kind 'consecutive_loop', got %q", loopEvt.Kind)
		}
		if loopEvt.Iteration != 4 {
			t.Errorf("Expected iteration 4, got %d", loopEvt.Iteration)
		}
	default:
		t.Error("Expected to receive consecutive_loop event from EventBus")
	}
}

func TestOverflowHandler_EmitsCompactEvent(t *testing.T) {
	bus := eventbus.NewEventBus()
	ch := bus.Subscribe("overflow_compact")

	handler := &OverflowHandler{MaxContextTokens: 200, Thresholds: DefaultThresholds(), EventBus: bus}
	args, _ := json.Marshal(map[string]interface{}{})
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "Tell me everything"},
			{Role: "assistant", Content: "Let me search", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "SearchMemories", Arguments: args}}},
			{Role: "tool", Content: strings.Repeat("result data ", 100), ToolCallID: "1", Name: "SearchMemories"},
			{Role: "assistant", Content: "Based on my search..."},
			{Role: "user", Content: "Thanks, now something else"},
		},
		MaxIterations: 10,
		Iteration:     0,
	}

	parsed := agentctx.ParsedResponse{
		ToolCalls:    []agentctx.ToolCall{{ID: "2", Name: "GetHotBlock", Arguments: args}},
		FinishReason: "tool_use",
	}

	action, handled := handler.Handle(convCtx, nil, parsed)
	// Compaction should succeed — passes through to next handler
	if handled {
		t.Errorf("Expected handler to pass through after successful compaction, got action=%v handled=%v", action, handled)
	}

	select {
	case evt := <-ch:
		loopEvt, ok := evt.Data.(LoopDetectionEvent)
		if !ok {
			t.Fatalf("Expected LoopDetectionEvent, got %T", evt.Data)
		}
		if loopEvt.Kind != "overflow_compact" {
			t.Errorf("Expected kind 'overflow_compact', got %q", loopEvt.Kind)
		}
	default:
		t.Error("Expected to receive overflow_compact event from EventBus")
	}
}

func TestHandlerNoEventBus_NoPanic(t *testing.T) {
	// Verify handlers work correctly with nil EventBus (no events, no crashes)
	handler := &StuckHandler{Thresholds: DefaultThresholds(), EventBus: nil}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 10,
	}
	toolResults := []agentctx.ToolResult{
		{CallID: "1", Name: "Tool1", Result: "error 1", IsError: true},
		{CallID: "2", Name: "Tool2", Result: "error 2", IsError: true},
	}

	// Should not panic
	action, handled := handler.Handle(convCtx, toolResults, parsedWithToolCalls())
	if !handled || action != agentctx.StageActionYIELD {
		t.Fatalf("Expected YIELD, handled=true, got %v, %v", action, handled)
	}
}

// --- Helper ---

func parsedWithToolCalls() agentctx.ParsedResponse {
	args, _ := json.Marshal(map[string]interface{}{})
	return agentctx.ParsedResponse{
		Text:         "",
		ToolCalls:    []agentctx.ToolCall{{ID: "call1", Name: "SomeTool", Arguments: args}},
		FinishReason: "tool_use",
	}
}