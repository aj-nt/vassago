package agent

import (
	"context"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

func TestAgentLoop_textOnly(t *testing.T) {
	// Set up mock provider that returns text immediately (no tool calls)
	mockResponses := []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "Hello! How can I help you?",
				FinishReason: "stop",
			},
		},
	}
	mockProvider := provider.NewMockProvider("mock", mockResponses)
	registry := provider.NewProviderRegistry()
	registry.Register("openai", mockProvider)

	// Set up tool registry (empty for this test)
	toolReg := tools.NewToolRegistry()

	// Create agent without daemon (nil client — we won't call it)
	cfg := Config{
		AgentID:          "test-agent",
		AgentName:        "test",
		AgentRole:        "dev",
		Personality:      "direct",
		MaxTurns:         50,
		MaxContextTokens:  128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	// Override Mnemo methods by not calling them (hot block will fail gracefully)
	result, err := a.Run(context.Background(), "Hello!")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Response == nil {
		t.Fatal("Expected a response")
	}
	if result.Response.Text != "Hello! How can I help you?" {
		t.Errorf("Expected 'Hello! How can I help you?', got %q", result.Response.Text)
	}
	if result.Iterations != 1 {
		t.Errorf("Expected 1 iteration, got %d", result.Iterations)
	}
	if result.MaxIterationsHit {
		t.Error("Should not hit max iterations")
	}
}

func TestAgentLoop_toolCall(t *testing.T) {
	// Mock: first returns a tool call, then returns text
	mockResponses := []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				ToolCalls: []agentctx.ToolCall{
					{ID: "call-1", Name: "AddMemory", Arguments: []byte(`{"key":"test","content":"testing"}`)},
				},
				FinishReason: "tool_calls",
			},
		},
		{
			Response: agentctx.ParsedResponse{
				Text:         "Memory saved!",
				FinishReason: "stop",
			},
		},
	}
	mockProvider := provider.NewMockProvider("mock", mockResponses)
	registry := provider.NewProviderRegistry()
	registry.Register("openai", mockProvider)

	// Set up tool registry with a test tool
	toolReg := tools.NewToolRegistry()
	toolReg.Register("AddMemory", func(args map[string]interface{}) (string, bool) {
		return "saved", false
	}, map[string]interface{}{
		"name":        "AddMemory",
		"description": "Save a memory",
	})

	cfg := Config{
		AgentID:          "test-agent",
		MaxTurns:         50,
		MaxContextTokens:  128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	result, err := a.Run(context.Background(), "Remember this: testing")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Iterations != 2 {
		t.Errorf("Expected 2 iterations, got %d", result.Iterations)
	}
}

func TestAgentLoop_maxIterations(t *testing.T) {
	// Mock that always returns tool calls (infinite loop)
	mockResponses := []provider.CannedResponse{}
	mockProvider := provider.NewMockProvider("mock", mockResponses)
	// Override default to return tool calls
	mockProvider.SetDefaultResponse(agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "loop", Name: "AddMemory", Arguments: []byte(`{"key":"x","content":"y"}`)},
		},
		FinishReason: "tool_calls",
	})

	registry := provider.NewProviderRegistry()
	registry.Register("openai", mockProvider)

	toolReg := tools.NewToolRegistry()
	toolReg.Register("AddMemory", func(args map[string]interface{}) (string, bool) {
		return "ok", false
	}, map[string]interface{}{"name": "AddMemory"})

	cfg := Config{
		AgentID:          "test-agent",
		MaxTurns:         3,
		MaxContextTokens:  128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	result, err := a.Run(context.Background(), "loop test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !result.MaxIterationsHit {
		t.Error("Expected max iterations to be hit")
	}
}
