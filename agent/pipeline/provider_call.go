// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

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
	Router   *provider.ModelRouter // Optional: nil = always-primary (backward compatible)
}

// NewProviderCallStage creates a new Stage 2.
func NewProviderCallStage(registry *provider.ProviderRegistry) *ProviderCallStage {
	return &ProviderCallStage{Registry: registry}
}

// NewProviderCallStageWithRouter creates a Stage 2 with model routing.
func NewProviderCallStageWithRouter(registry *provider.ProviderRegistry, router *provider.ModelRouter) *ProviderCallStage {
	return &ProviderCallStage{Registry: registry, Router: router}
}

// Process calls the LLM and returns the raw result.
func (s *ProviderCallStage) Process(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) agentctx.ProviderResult {
	var p provider.Provider
	var err error

	if s.Router != nil {
		// Use router to select provider
		routingCtx := provider.RoutingContext{
			Iteration:       convCtx.Iteration,
			UserMessage:     convCtx.FirstUserMessage(),
			HasToolResults:  convCtx.LastMessageIsToolResult(),
			ToolCallCount:   convCtx.ToolResultCount(),
			PromptCharCount: convCtx.CharCount(),
		}
		modelCfg, selectErr := s.Router.Select(routingCtx)
		if selectErr != nil {
			// Fallback to primary
			p, err = s.Registry.GetPrimary()
		} else {
			p, err = s.Registry.Get(modelCfg.Provider)
			if err != nil {
				// Provider not found, try primary
				p, err = s.Registry.GetPrimary()
			}
		}
	} else {
		p, err = s.Registry.GetPrimary()
	}

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
			ShouldRetry:    result.ShouldRetry,
			ShouldFallback: result.ShouldFallback,
			Error:          fmt.Errorf("execute: %w", err),
		}
	}

	return result
}

// populateModelProvider adds model/provider info to a ProviderResult for observability.
func populateModelProvider(result agentctx.ProviderResult, p provider.Provider, modelName string) agentctx.ProviderResult {
	result.Model = modelName
	result.Provider = p.Name()
	return result
}
