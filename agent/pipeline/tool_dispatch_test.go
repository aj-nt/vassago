package pipeline

import (
	"encoding/json"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

func TestToolDispatchStage_NoToolCalls(t *testing.T) {
	registry := tools.NewToolRegistry()
	bus := eventbus.NewEventBus()
	stage := NewToolDispatchStage(registry, bus)

	parsed := agentctx.ParsedResponse{
		Text:      "No tools needed",
		ToolCalls: nil,
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{},
		MaxIterations: 5,
	}

	result := stage.Process(convCtx, parsed)

	if len(result.ToolResults) != 0 {
		t.Errorf("Expected no tool results, got %d", len(result.ToolResults))
	}
}

func TestToolDispatchStage_SuccessfulToolCall(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.Register("EchoTool", func(args map[string]interface{}) (string, bool) {
		return "echo: hello", false
	}, map[string]interface{}{
		"description": "Echo tool",
		"parameters":  map[string]interface{}{},
	})
	bus := eventbus.NewEventBus()
	stage := NewToolDispatchStage(registry, bus)

	args, _ := json.Marshal(map[string]interface{}{"message": "hello"})
	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "call_1", Name: "EchoTool", Arguments: args},
		},
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{},
		MaxIterations: 5,
	}

	result := stage.Process(convCtx, parsed)

	if len(result.ToolResults) != 1 {
		t.Fatalf("Expected 1 tool result, got %d", len(result.ToolResults))
	}
	if result.ToolResults[0].Result != "echo: hello" {
		t.Errorf("Expected 'echo: hello', got %q", result.ToolResults[0].Result)
	}
	if result.ToolResults[0].CallID != "call_1" {
		t.Errorf("Expected CallID 'call_1', got %q", result.ToolResults[0].CallID)
	}
	if result.ToolResults[0].IsError {
		t.Error("Expected IsError=false for successful tool call")
	}
}

func TestToolDispatchStage_UnknownTool(t *testing.T) {
	registry := tools.NewToolRegistry()
	bus := eventbus.NewEventBus()
	stage := NewToolDispatchStage(registry, bus)

	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "call_1", Name: "NonExistentTool", Arguments: args},
		},
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{},
		MaxIterations: 5,
	}

	result := stage.Process(convCtx, parsed)

	if len(result.ToolResults) != 1 {
		t.Fatalf("Expected 1 tool result (error), got %d", len(result.ToolResults))
	}
	if !result.ToolResults[0].IsError {
		t.Error("Expected IsError=true for unknown tool")
	}
	if result.ToolResults[0].Result != "unknown tool: NonExistentTool" {
		t.Errorf("Expected unknown tool error message, got %q", result.ToolResults[0].Result)
	}
}

func TestToolDispatchStage_InvalidArguments(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.Register("TestTool", func(args map[string]interface{}) (string, bool) {
		return "should not reach here", false
	}, map[string]interface{}{})
	bus := eventbus.NewEventBus()
	stage := NewToolDispatchStage(registry, bus)

	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "call_1", Name: "TestTool", Arguments: json.RawMessage("not valid json")},
		},
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{},
		MaxIterations: 5,
	}

	result := stage.Process(convCtx, parsed)

	if len(result.ToolResults) != 1 {
		t.Fatalf("Expected 1 tool result, got %d", len(result.ToolResults))
	}
	if !result.ToolResults[0].IsError {
		t.Error("Expected IsError=true for invalid arguments")
	}
}

func TestToolDispatchStage_EmptyArguments(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.Register("GetHotBlock", func(args map[string]interface{}) (string, bool) {
		// GetHotBlock should work fine with empty args (defaults apply)
		return "Hot block retrieved", false
	}, map[string]interface{}{})
	bus := eventbus.NewEventBus()
	stage := NewToolDispatchStage(registry, bus)

	testCases := []struct {
		name string
		args json.RawMessage
	}{
		{"empty byte slice", json.RawMessage("")},
		{"null literal", json.RawMessage("null")},
		{"empty object", json.RawMessage("{}")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := agentctx.ParsedResponse{
				Text: "",
				ToolCalls: []agentctx.ToolCall{
					{ID: "call_1", Name: "GetHotBlock", Arguments: tc.args},
				},
			}
			convCtx := &agentctx.ConversationContext{
				Messages:      []agentctx.Message{},
				MaxIterations: 5,
			}

			result := stage.Process(convCtx, parsed)

			if len(result.ToolResults) != 1 {
				t.Fatalf("Expected 1 tool result, got %d", len(result.ToolResults))
			}
			if result.ToolResults[0].IsError {
				t.Errorf("Empty/null arguments should default to {}, got error: %s", result.ToolResults[0].Result)
			}
			if result.ToolResults[0].Result != "Hot block retrieved" {
				t.Errorf("Expected successful result, got %q", result.ToolResults[0].Result)
			}
		})
	}
}

func TestToolDispatchStage_PanicRecovery(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.Register("PanickingTool", func(args map[string]interface{}) (string, bool) {
		panic("tool panic!")
	}, map[string]interface{}{})
	bus := eventbus.NewEventBus()
	stage := NewToolDispatchStage(registry, bus)

	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "call_1", Name: "PanickingTool", Arguments: args},
		},
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{},
		MaxIterations: 5,
	}

	result := stage.Process(convCtx, parsed)

	if len(result.ToolResults) != 1 {
		t.Fatalf("Expected 1 tool result (from panic recovery), got %d", len(result.ToolResults))
	}
	if !result.ToolResults[0].IsError {
		t.Error("Expected IsError=true for panicking tool")
	}
}

func TestToolDispatchStage_MultipleToolCalls(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.Register("Tool1", func(args map[string]interface{}) (string, bool) {
		return "result1", false
	}, map[string]interface{}{})
	registry.Register("Tool2", func(args map[string]interface{}) (string, bool) {
		return "result2", false
	}, map[string]interface{}{})
	bus := eventbus.NewEventBus()
	stage := NewToolDispatchStage(registry, bus)

	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "call_1", Name: "Tool1", Arguments: args},
			{ID: "call_2", Name: "Tool2", Arguments: args},
		},
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{},
		MaxIterations: 5,
	}

	result := stage.Process(convCtx, parsed)

	if len(result.ToolResults) != 2 {
		t.Fatalf("Expected 2 tool results, got %d", len(result.ToolResults))
	}
	names := map[string]bool{}
	for _, tr := range result.ToolResults {
		names[tr.Name] = true
	}
	if !names["Tool1"] || !names["Tool2"] {
		t.Errorf("Expected both Tool1 and Tool2 in results, got %v", names)
	}
}

func TestToolDispatchStage_ErrorReturningTool(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.Register("ErrorTool", func(args map[string]interface{}) (string, bool) {
		return "database connection failed", true // isError = true
	}, map[string]interface{}{})
	bus := eventbus.NewEventBus()
	stage := NewToolDispatchStage(registry, bus)

	args, _ := json.Marshal(map[string]interface{}{})
	parsed := agentctx.ParsedResponse{
		Text: "",
		ToolCalls: []agentctx.ToolCall{
			{ID: "call_1", Name: "ErrorTool", Arguments: args},
		},
	}
	convCtx := &agentctx.ConversationContext{
		Messages:      []agentctx.Message{},
		MaxIterations: 5,
	}

	result := stage.Process(convCtx, parsed)

	if len(result.ToolResults) != 1 {
		t.Fatalf("Expected 1 tool result, got %d", len(result.ToolResults))
	}
	if !result.ToolResults[0].IsError {
		t.Error("Expected IsError=true when tool returns isError=true")
	}
	if result.ToolResults[0].Result != "database connection failed" {
		t.Errorf("Expected error message, got %q", result.ToolResults[0].Result)
	}
}