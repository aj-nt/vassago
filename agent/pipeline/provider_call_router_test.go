package pipeline

import (
	"context"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
)

func TestProviderCallStageWithRouter(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", &mockProvider{name: "mock"})
	registry.SetPrimary("mock")

	// Test without router — falls back to primary
	stage := NewProviderCallStage(registry)
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	prepared := agentctx.PreparedRequest{
		SystemPrompt: "test",
		Messages:     []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	result := stage.Process(context.Background(), convCtx, prepared)
	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
}

func TestProviderCallStageWithRouterSelectsProvider(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", &mockProvider{name: "mock"})
	registry.SetPrimary("mock")

	router := provider.NewModelRouter(provider.StrategyAlwaysPrimary)
	router.Register(provider.ModelConfig{
		Name:      "mock-model",
		Provider:  "mock",
		ToolUse:   true,
	})
	router.SetPrimary("mock-model")

	stage := NewProviderCallStageWithRouter(registry, router)
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	prepared := agentctx.PreparedRequest{
		SystemPrompt: "test",
		Messages:     []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	result := stage.Process(context.Background(), convCtx, prepared)
	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
}

func TestProviderCallStageWithRouterFallback(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", &mockProvider{name: "mock"})
	registry.SetPrimary("mock")

	// Router points to a non-existent provider
	router := provider.NewModelRouter(provider.StrategyAlwaysPrimary)
	router.Register(provider.ModelConfig{
		Name:     "nonexistent-model",
		Provider: "nonexistent-provider",
		ToolUse:  true,
	})
	router.SetPrimary("nonexistent-model")

	stage := NewProviderCallStageWithRouter(registry, router)
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	prepared := agentctx.PreparedRequest{
		SystemPrompt: "test",
		Messages:     []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	// Should fall back to primary provider, not fail
	result := stage.Process(context.Background(), convCtx, prepared)
	if result.Error != nil {
		t.Fatalf("expected no error (fallback to primary), got: %v", result.Error)
	}
}

func TestFirstUserMessage(t *testing.T) {
	msgs := []agentctx.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	if got := firstUserMessage(msgs); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
	if got := firstUserMessage(nil); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestLastMessageIsToolResult(t *testing.T) {
	msgs := []agentctx.Message{
		{Role: "user", Content: "hello"},
		{Role: "tool", Content: "result"},
	}
	if !lastMessageIsToolResult(msgs) {
		t.Error("expected true for tool result as last message")
	}
	if lastMessageIsToolResult(msgs[:1]) {
		t.Error("expected false for user message as last message")
	}
	if lastMessageIsToolResult(nil) {
		t.Error("expected false for nil messages")
	}
}

func TestCountToolResults(t *testing.T) {
	msgs := []agentctx.Message{
		{Role: "user", Content: "hello"},
		{Role: "tool", Content: "result1"},
		{Role: "tool", Content: "result2"},
	}
	if got := countToolResults(msgs); got != 2 {
		t.Errorf("expected 2 tool results, got %d", got)
	}
	if got := countToolResults(nil); got != 0 {
		t.Errorf("expected 0 for nil, got %d", got)
	}
}