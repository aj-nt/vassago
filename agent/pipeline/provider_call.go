package pipeline

import (
	"context"
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
)

// ProviderCallStage dispatches the LLM call via the ProviderRegistry.
type ProviderCallStage struct {
	Registry *provider.ProviderRegistry
}

// NewProviderCallStage creates a new Stage 2.
func NewProviderCallStage(registry *provider.ProviderRegistry) *ProviderCallStage {
	return &ProviderCallStage{Registry: registry}
}

// Process calls the LLM and returns the raw result.
func (s *ProviderCallStage) Process(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) agentctx.ProviderResult {
	// Get provider (use first available if no specific provider set)
	p, err := s.Registry.GetWithFallback("openai")
	if err != nil {
		return agentctx.ProviderResult{
			ShouldRetry: false,
			Error:       fmt.Errorf("provider call: %w", err),
		}
	}

	// Prepare provider-specific request
	req, err := p.Prepare(ctx, convCtx, prepared)
	if err != nil {
		return agentctx.ProviderResult{
			ShouldRetry: false,
			Error:       fmt.Errorf("prepare: %w", err),
		}
	}

	// Execute the call
	result, err := p.Execute(ctx, req)
	if err != nil {
		return agentctx.ProviderResult{
			ShouldRetry:   result.ShouldRetry,
			ShouldFallback: result.ShouldFallback,
			Error:          fmt.Errorf("execute: %w", err),
		}
	}

	return result
}
