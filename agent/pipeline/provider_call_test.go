package pipeline

import (
	"context"
	"errors"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
)

func TestProviderCallStage_Success(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", &mockProvider{
		name: "mock",
		executeFn: func(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
			return agentctx.ProviderResult{RawResponse: []byte(`{"content":[]}`)}, nil
		},
	})
	registry.SetPrimary("mock")

	stage := NewProviderCallStage(registry)
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	prepared := agentctx.PreparedRequest{
		SystemPrompt: "You are helpful",
		Messages:     convCtx.Messages,
		Tools:        nil,
		APIKwargs:    map[string]interface{}{"temperature": 0.7},
	}

	result := stage.Process(context.Background(), convCtx, prepared)

	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}
}

func TestProviderCallStage_ProviderExecutionError(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", &mockProvider{
		name: "mock",
		prepareFn: func(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
			return nil, nil
		},
		executeFn: func(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
			return agentctx.ProviderResult{Error: errors.New("API rate limit")}, nil
		},
	})
	registry.SetPrimary("mock")

	stage := NewProviderCallStage(registry)
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	prepared := agentctx.PreparedRequest{
		Messages: convCtx.Messages,
	}

	result := stage.Process(context.Background(), convCtx, prepared)

	if result.Error == nil {
		t.Error("Expected error from provider execution failure")
	}
}

func TestProviderCallStage_NoPrimaryProvider(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.SetPrimary("nonexistent")

	stage := NewProviderCallStage(registry)
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{{Role: "user", Content: "hello"}},
	}
	prepared := agentctx.PreparedRequest{
		Messages: convCtx.Messages,
	}

	result := stage.Process(context.Background(), convCtx, prepared)

	if result.Error == nil {
		t.Error("Expected error when no primary provider is available")
	}
}