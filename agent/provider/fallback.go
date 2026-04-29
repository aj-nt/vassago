package provider

import (
	"context"
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// FallbackChain implements a fallback mechanism for provider calls.
type FallbackChain struct {
	primary   Provider
	fallback1 Provider
	fallback2 Provider
}

// NewFallbackChain creates a new fallback chain.
func NewFallbackChain(primary, fallback1, fallback2 Provider) *FallbackChain {
	return &FallbackChain{
		primary:   primary,
		fallback1: fallback1,
		fallback2: fallback2,
	}
}

// ExecuteWithFallback tries the primary provider, then fallbacks on specific errors.
func (f *FallbackChain) ExecuteWithFallback(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (agentctx.ParsedResponse, error) {
	// Try primary provider
	result, providerResult := f.tryProvider(ctx, f.primary, convCtx, prepared)
	if providerResult.Error == nil {
		return result, nil
	}

	// Check if error is retryable/fallbackable
	if providerResult.ShouldFallback {
		// Try first fallback
		result, fallbackResult1 := f.tryProvider(ctx, f.fallback1, convCtx, prepared)
		if fallbackResult1.Error == nil {
			return result, nil
		}

		// Check if first fallback error is retryable
		if fallbackResult1.ShouldFallback {
			// Try second fallback
			result, fallbackResult2 := f.tryProvider(ctx, f.fallback2, convCtx, prepared)
			if fallbackResult2.Error == nil {
				return result, nil
			}
			return agentctx.ParsedResponse{}, fmt.Errorf("all providers failed: %v", fallbackResult2.Error)
		}
		return agentctx.ParsedResponse{}, fmt.Errorf("primary and first fallback failed: %v", fallbackResult1.Error)
	}

	return agentctx.ParsedResponse{}, fmt.Errorf("primary provider failed: %v", providerResult.Error)
}

// tryProvider attempts to execute a provider and parse the result.
func (f *FallbackChain) tryProvider(ctx context.Context, provider Provider, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (agentctx.ParsedResponse, agentctx.ProviderResult) {
	// Prepare request
	req, err := provider.Prepare(ctx, convCtx, prepared)
	if err != nil {
		return agentctx.ParsedResponse{}, agentctx.ProviderResult{Error: fmt.Errorf("prepare failed: %w", err)}
	}

	// Execute request
	result, err := provider.Execute(ctx, req)
	if err != nil {
		return agentctx.ParsedResponse{}, agentctx.ProviderResult{Error: fmt.Errorf("execute failed: %w", err)}
	}

	// Parse response
	parsed, err := provider.Parse(ctx, result)
	if err != nil {
		return agentctx.ParsedResponse{}, agentctx.ProviderResult{Error: fmt.Errorf("parse failed: %w", err)}
	}

	return parsed, agentctx.ProviderResult{}
}
