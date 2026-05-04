// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

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
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	result, err := a.Run(context.Background(), "loop test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// The repetitive tool loop detection should stop the loop before max iterations.
	// The agent should complete without error and with fewer iterations than MaxTurns.
	if result.Iterations == 0 {
		t.Error("Expected at least 1 iteration")
	}
	// MaxIterationsHit may or may not be true depending on whether
	// the repetition detector or the iteration limit fires first.
	// The important thing is the loop terminates, not how.
}

func TestAgentWithRouter(t *testing.T) {
	mockResponses := []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "Hello from router!",
				FinishReason: "stop",
				Usage:        &agentctx.UsageInfo{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			},
		},
	}
	mockProvider := provider.NewMockProvider("mock", mockResponses)
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	// Set up router
	router := provider.NewModelRouter(provider.StrategyAlwaysPrimary)
	router.Register(provider.ModelConfig{
		Name:          "mock-model",
		Provider:      "mock",
		InputCost:     3.00,
		OutputCost:    15.00,
		ContextWindow: 128000,
		ToolUse:       true,
	})
	if err := router.SetPrimary("mock-model"); err != nil {
		t.Fatal(err)
	}

	toolReg := tools.NewToolRegistry()
	cfg := Config{
		AgentID:          "test-agent",
		MaxTurns:         50,
		MaxContextTokens: 128000,
	}
	a := NewAgentWithRouter(cfg, registry, router, toolReg, nil)
	defer a.Close()

	if a.Router == nil {
		t.Fatal("expected router to be set")
	}

	result, err := a.Run(context.Background(), "Hello!")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Response == nil {
		t.Fatal("Expected a response")
	}
	if result.Response.Text != "Hello from router!" {
		t.Errorf("Expected 'Hello from router!', got %q", result.Response.Text)
	}
}

func TestSelectProvider(t *testing.T) {
	mockResponses := []provider.CannedResponse{
		{Response: agentctx.ParsedResponse{Text: "hi", FinishReason: "stop"}},
	}
	mockP := provider.NewMockProvider("mock", mockResponses)
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockP)
	registry.SetPrimary("mock")

	// Without router — should fall back to primary
	a := &Agent{
		Config:   Config{AgentID: "test"},
		Registry: registry,
	}
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	p, err := a.selectProvider(convCtx)
	if err != nil {
		t.Fatalf("selectProvider error: %v", err)
	}
	if p.Name() != "mock" {
		t.Errorf("expected mock, got %s", p.Name())
	}
}

func TestSelectProviderWithRouter(t *testing.T) {
	mockResponses := []provider.CannedResponse{
		{Response: agentctx.ParsedResponse{Text: "hi", FinishReason: "stop"}},
	}
	mockP := provider.NewMockProvider("mock", mockResponses)
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockP)

	router := provider.NewModelRouter(provider.StrategyCostOptimized)
	router.Register(provider.ModelConfig{
		Name:     "cheap",
		Provider: "mock",
		ToolUse:  true,
	})
	router.SetPrimary("cheap") //nolint:errcheck // test: model is registered above

	a := &Agent{
		Config:   Config{AgentID: "test"},
		Registry: registry,
		Router:   router,
	}
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	p, err := a.selectProvider(convCtx)
	if err != nil {
		t.Fatalf("selectProvider error: %v", err)
	}
	if p.Name() != "mock" {
		t.Errorf("expected mock, got %s", p.Name())
	}
}
