package agent

import (
	"context"
	"fmt"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

// === Existing tests are in agent_test.go ===
// This file adds streaming, context management, and edge case tests.

func TestAgentLoop_interrupt(t *testing.T) {
	mockProvider := provider.NewMockProvider("mock", []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "Let me think...",
				FinishReason: "stop",
			},
		},
	})
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	toolReg := tools.NewToolRegistry()
	cfg := Config{
		AgentID:          "test",
		MaxTurns:         50,
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	// RunWithContext with interrupt set — the agent should yield immediately
	// because the interrupt is checked at the start of each iteration
	interrupt := agentctx.NewInterruptSignal()
	interrupt.Set() // Set before running

	convCtx := &agentctx.ConversationContext{
		SessionID:       "test-interrupt",
		AgentID:         "test",
		Messages:        []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations:   50,
		Iteration:       0,
		Interrupt:       interrupt,
		HotBlockContent: "",
		Temperature:     0.7,
	}

	result, err := a.RunWithContext(context.Background(), convCtx)
	if err != nil {
		t.Fatalf("RunWithContext failed: %v", err)
	}
	if !result.Interrupted {
		t.Error("Expected interrupted=true when interrupt is set before run")
	}
	if result.Iterations != 0 {
		t.Errorf("Expected 0 iterations when interrupted before run, got %d", result.Iterations)
	}
}

func TestAgentLoop_emptyResponse(t *testing.T) {
	// Provider returns empty text
	mockProvider := provider.NewMockProvider("mock", []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "",
				FinishReason: "stop",
			},
		},
	})
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	toolReg := tools.NewToolRegistry()
	cfg := Config{
		AgentID:          "test",
		MaxTurns:         50,
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	result, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Response == nil {
		t.Fatal("Expected a response even for empty text")
	}
	if result.Response.Text != "" {
		t.Errorf("Expected empty text response, got %q", result.Response.Text)
	}
}

func TestAgentLoop_providerError(t *testing.T) {
	// Provider returns an error
	mockProvider := provider.NewMockProvider("mock", []provider.CannedResponse{
		{
			Error: fmt.Errorf("API rate limit exceeded"),
		},
	})
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	toolReg := tools.NewToolRegistry()
	cfg := Config{
		AgentID:          "test",
		MaxTurns:         50,
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	_, err := a.Run(context.Background(), "hello")
	if err == nil {
		t.Error("Expected error from provider failure")
	}
}

func TestAgentLoop_noHotBlock(t *testing.T) {
	// Agent with nil Mnemo client should still work (hot block is empty)
	mockProvider := provider.NewMockProvider("mock", []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "I don't have access to memory right now, but I can still help!",
				FinishReason: "stop",
			},
		},
	})
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	toolReg := tools.NewToolRegistry()
	cfg := Config{
		AgentID:          "test",
		Personality:     "helpful assistant",
		MaxTurns:         50,
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil) // nil Mnemo client
	defer a.Close()

	result, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Response == nil {
		t.Fatal("Expected a response")
	}
}

func TestAgentLoop_toolCallThenText(t *testing.T) {
	// Provider first calls a tool, then returns text
	toolReg := tools.NewToolRegistry()
	var toolCalled bool
	toolReg.Register("EchoTool", func(args map[string]interface{}) (string, bool) {
		toolCalled = true
		msg, _ := args["message"].(string)
		return "Echo: " + msg, false
	}, map[string]interface{}{
		"name":        "EchoTool",
		"description": "Echo a message",
		"parameters":  map[string]interface{}{},
	})

	mockProvider := provider.NewMockProvider("mock", []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "",
				ToolCalls:    []agentctx.ToolCall{{ID: "call-1", Name: "EchoTool", Arguments: []byte(`{"message":"test"}`)}},
				FinishReason: "tool_calls",
			},
		},
		{
			Response: agentctx.ParsedResponse{
				Text:         "The echo result is: Echo: test",
				FinishReason: "stop",
			},
		},
	})
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	cfg := Config{
		AgentID:          "test",
		MaxTurns:         50,
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	result, err := a.Run(context.Background(), "echo test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !toolCalled {
		t.Error("Expected EchoTool to be called")
	}
	if result.Iterations != 2 {
		t.Errorf("Expected 2 iterations (tool call + text), got %d", result.Iterations)
	}
}

func TestAgentBuildToolDefinitions(t *testing.T) {
	toolReg := tools.NewToolRegistry()
	toolReg.Register("SearchMemories", func(args map[string]interface{}) (string, bool) {
		return "found nothing", false
	}, map[string]interface{}{
		"name":        "SearchMemories",
		"description": "Search memories",
		"parameters":  map[string]interface{}{},
	})
	toolReg.Register("AddMemory", func(args map[string]interface{}) (string, bool) {
		return "saved", false
	}, map[string]interface{}{
		"name":        "AddMemory",
		"description": "Save a memory",
		"parameters":  map[string]interface{}{},
	})

	cfg := Config{AgentID: "test"}
	a := NewAgent(cfg, nil, toolReg, nil)
	defer a.Close()

	defs := a.BuildToolDefinitions()
	if len(defs) != 2 {
		t.Errorf("Expected 2 tool definitions, got %d", len(defs))
	}

	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["SearchMemories"] {
		t.Error("Expected SearchMemories in tool definitions")
	}
	if !names["AddMemory"] {
		t.Error("Expected AddMemory in tool definitions")
	}
}

func TestAgentGetHotBlock_NilClient(t *testing.T) {
	cfg := Config{AgentID: "test"}
	a := NewAgent(cfg, nil, nil, nil)
	defer a.Close()

	// Should return empty string with nil client, not panic
	hb, err := a.GetHotBlock(context.Background())
	if err != nil {
		t.Errorf("Expected no error with nil client, got %v", err)
	}
	if hb != "" {
		t.Errorf("Expected empty hot block with nil client, got %q", hb)
	}
}

func TestAgentLoop_multipleToolCalls(t *testing.T) {
	// Provider makes two tool calls in one response
	toolReg := tools.NewToolRegistry()
	toolReg.Register("AddMemory", func(args map[string]interface{}) (string, bool) {
		return "saved", false
	}, map[string]interface{}{"name": "AddMemory", "description": "Save memory", "parameters": map[string]interface{}{}})
	toolReg.Register("SearchMemories", func(args map[string]interface{}) (string, bool) {
		return "found 3 results", false
	}, map[string]interface{}{"name": "SearchMemories", "description": "Search memories", "parameters": map[string]interface{}{}})

	mockProvider := provider.NewMockProvider("mock", []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text: "",
				ToolCalls: []agentctx.ToolCall{
					{ID: "call-1", Name: "SearchMemories", Arguments: []byte(`{"query":"test"}`)},
					{ID: "call-2", Name: "AddMemory", Arguments: []byte(`{"key":"x","content":"y","target":"memory","category":"quirk","priority":3}`)},
				},
				FinishReason: "tool_calls",
			},
		},
		{
			Response: agentctx.ParsedResponse{
				Text:         "I searched and saved. Here's what I found...",
				FinishReason: "stop",
			},
		},
	})
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	cfg := Config{
		AgentID:          "test",
		MaxTurns:         10,
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	result, err := a.Run(context.Background(), "search and save")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Iterations != 2 {
		t.Errorf("Expected 2 iterations, got %d", result.Iterations)
	}
}

// TestAgentStreaming_FallbackOnStreamError tests that when a provider returns a StreamError,
// the agent falls back to the primary provider if the failed provider is not primary.
func TestAgentStreaming_FallbackOnStreamError(t *testing.T) {
	// Create a custom mock provider that supports streaming and returns a StreamError
	streamErrorProvider := &StreamErrorMockProvider{}

	// Create a primary provider that works
	primaryProvider := provider.NewMockProvider("primary", []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "Fallback response from primary",
				FinishReason: "stop",
			},
		},
	})

	registry := provider.NewProviderRegistry()
	registry.Register("stream-error", streamErrorProvider)
	registry.Register("primary", primaryProvider)
	registry.SetPrimary("primary")

	toolReg := tools.NewToolRegistry()
	cfg := Config{
		AgentID:          "test",
		MaxTurns:         10,
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	// Run should succeed with fallback
	result, err := a.Run(context.Background(), "test fallback")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have completed successfully with fallback
	if result.Iterations != 1 {
		t.Errorf("Expected 1 iteration with fallback, got %d", result.Iterations)
	}
}

// StreamErrorMockProvider is a mock provider that supports streaming and returns StreamError
type StreamErrorMockProvider struct{}

func (s *StreamErrorMockProvider) Name() string {
	return "stream-error"
}

func (s *StreamErrorMockProvider) Capabilities() agentctx.ProviderCapabilities {
	return agentctx.ProviderCapabilities{
		SupportsStreaming:       true,
		SupportsFunctionCalling: true,
		SupportsPromptCaching:  false,
		SupportsVision:         false,
		MaxContextTokens:       128000,
	}
}

func (s *StreamErrorMockProvider) Prepare(_ context.Context, _ *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	return prepared, nil
}

func (s *StreamErrorMockProvider) Execute(_ context.Context, _ interface{}) (agentctx.ProviderResult, error) {
	return agentctx.ProviderResult{}, fmt.Errorf("streaming provider does not support non-streaming execute")
}

func (s *StreamErrorMockProvider) Parse(_ context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	return agentctx.ParsedResponse{}, fmt.Errorf("streaming provider does not support parse")
}

func (s *StreamErrorMockProvider) Stream(_ context.Context, _ interface{}) (<-chan agentctx.StreamEvent, error) {
	resultCh := make(chan agentctx.StreamEvent, 1)
	resultCh <- agentctx.StreamEvent{
		Type:  agentctx.StreamError,
		Error: fmt.Errorf("streaming failed"),
	}
	close(resultCh)
	return resultCh, nil
}

func (s *StreamErrorMockProvider) EndpointURL() string {
	return ""
}