package pipeline

import (
	"context"
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
)

// ResponseProcessingStage parses the raw LLM response into a canonical ParsedResponse.
type ResponseProcessingStage struct {
	Registry *provider.ProviderRegistry
}

// NewResponseProcessingStage creates a new Stage 3.
func NewResponseProcessingStage(registry *provider.ProviderRegistry) *ResponseProcessingStage {
	return &ResponseProcessingStage{Registry: registry}
}

// Process converts raw response to ParsedResponse using the primary provider.
func (s *ResponseProcessingStage) Process(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	if result.Error != nil {
		return agentctx.ParsedResponse{}, fmt.Errorf("response processing: %w", result.Error)
	}

	p, err := s.Registry.GetPrimary()
	if err != nil {
		return agentctx.ParsedResponse{}, fmt.Errorf("get provider for parse: %w", err)
	}

	return p.Parse(ctx, result)
}