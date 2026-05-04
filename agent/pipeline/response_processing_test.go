// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"context"
	"fmt"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
)

func TestResponseProcessingStage_ErrorResult(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", &mockProvider{})
	registry.SetPrimary("mock")
	stage := NewResponseProcessingStage(registry)

	result := agentctx.ProviderResult{
		Error: fmt.Errorf("provider call failed"),
	}

	_, err := stage.Process(context.Background(), result)
	if err == nil {
		t.Error("Expected error when ProviderResult has an error")
	}
}

func TestResponseProcessingStage_ParseDelegatesToProvider(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", &mockProvider{
		name: "mock",
		parseFn: func(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
			return agentctx.ParsedResponse{Text: "hello", FinishReason: "stop"}, nil
		},
	})
	registry.SetPrimary("mock")
	stage := NewResponseProcessingStage(registry)

	result := agentctx.ProviderResult{
		RawResponse: []byte(`{"content":[{"type":"text","text":"hello"}],"stop_reason":"stop"}`),
	}

	parsed, err := stage.Process(context.Background(), result)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if parsed.Text != "hello" {
		t.Errorf("Expected text 'hello', got %q", parsed.Text)
	}
}

func TestResponseProcessingStage_ParseError(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", &mockProvider{
		name: "mock",
		parseFn: func(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
			return agentctx.ParsedResponse{}, fmt.Errorf("parse error")
		},
	})
	registry.SetPrimary("mock")
	stage := NewResponseProcessingStage(registry)

	result := agentctx.ProviderResult{
		RawResponse: []byte("garbage"),
	}

	_, err := stage.Process(context.Background(), result)
	if err == nil {
		t.Error("Expected error when provider Parse returns an error")
	}
}