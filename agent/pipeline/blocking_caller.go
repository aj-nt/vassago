// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"context"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// BlockingCaller implements ProviderCaller for non-streaming execution.
// It runs Stage 2 (Provider Call) and Stage 3 (Response Processing) directly.
type BlockingCaller struct {
	Stage2 *ProviderCallStage
	Stage3 *ResponseProcessingStage
}

// NewBlockingCaller creates a caller for non-streaming mode.
func NewBlockingCaller(stage2 *ProviderCallStage, stage3 *ResponseProcessingStage) *BlockingCaller {
	return &BlockingCaller{Stage2: stage2, Stage3: stage3}
}

// Call executes the provider call and parses the response.
func (c *BlockingCaller) Call(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (agentctx.ParsedResponse, bool, error) {
	// Stage 2: Provider Call
	result := c.Stage2.Process(ctx, convCtx, prepared)
	if result.Error != nil {
		return agentctx.ParsedResponse{}, false, result.Error
	}

	// Stage 3: Response Processing
	parsed, err := c.Stage3.Process(ctx, result)
	if err != nil {
		return agentctx.ParsedResponse{}, false, err
	}

	return parsed, true, nil
}