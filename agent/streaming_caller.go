// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/pipeline"
	"github.com/aj-nt/mnemosyne/agent/provider"
)

// streamingCaller implements pipeline.ProviderCaller for streaming execution.
// It tries streaming first, falls back to non-streaming on stream init failure,
// and accumulates streaming events into a ParsedResponse.
//
// Phase 37: provider-level fallback is handled by the FallbackProvider decorator
// composed in setup.go. This caller no longer duplicates fallback logic.
type streamingCaller struct {
	agent   *Agent
	out     chan<- StreamEvent
	ctx     context.Context
	convCtx *agentctx.ConversationContext
}

// newStreamingCaller creates a caller for streaming mode.
func newStreamingCaller(agent *Agent, out chan<- StreamEvent, ctx context.Context, convCtx *agentctx.ConversationContext) *streamingCaller {
	return &streamingCaller{agent: agent, out: out, ctx: ctx, convCtx: convCtx}
}

// Call tries streaming first, falls back to non-streaming on stream init failure.
func (c *streamingCaller) Call(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (agentctx.ParsedResponse, bool, error) {
	// Select provider using agent's router
	p, err := c.agent.selectProvider(convCtx)
	if err != nil {
		return agentctx.ParsedResponse{}, false, fmt.Errorf("provider error: %w", err)
	}

	// Track model name for failure demotion
	modelName := ""
	if c.agent.Router != nil {
		modelCfg, selectErr := c.agent.Router.Select(provider.RoutingContext{
			Iteration:       convCtx.Iteration,
			UserMessage:     convCtx.FirstUserMessage(),
			HasToolResults:  convCtx.LastMessageIsToolResult(),
			ToolCallCount:   convCtx.ToolResultCount(),
			PromptCharCount: convCtx.CharCount(),
		})
		if selectErr == nil {
			modelName = modelCfg.Name
			convCtx.SetTokenCounter(agentctx.CounterForModel(modelCfg.Name))
		}
	}

	req, prepErr := p.Prepare(ctx, convCtx, prepared)
	if prepErr != nil {
		return agentctx.ParsedResponse{}, false, fmt.Errorf("prepare error: %w", prepErr)
	}

	// Try streaming. FallbackProvider (if composed) handles provider fallback
	// internally — if the primary's Stream fails, it tries the fallback chain
	// and returns a synthetic stream channel from non-streaming results.
	streamCh, streamErr := p.Stream(ctx, req)
	if streamErr != nil {
		// Streaming init failed even after decorator fallback — use Stage 2-3.
		return c.callNonStreaming(ctx, convCtx, prepared, p, modelName)
	}

	// Process streaming events
	parsed, processErr := c.processStreamingEvents(streamCh, p, modelName)
	if processErr != nil {
		return agentctx.ParsedResponse{}, false, processErr
	}

	return parsed, true, nil
}

// callNonStreaming is the fallback when streaming init fails.
// It runs Stage 2-3 synchronously and emits the response as a single text chunk.
func (c *streamingCaller) callNonStreaming(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest, p provider.Provider, modelName string) (agentctx.ParsedResponse, bool, error) {
	stage2 := pipeline.NewProviderCallStageWithRouter(c.agent.Registry, c.agent.Router)
	result := stage2.Process(ctx, convCtx, prepared)
	if result.Error != nil {
		c.recordFailure(p, modelName, result.Error)
		return agentctx.ParsedResponse{}, false, fmt.Errorf("provider call failed: %w", result.Error)
	}

	parsed, err := c.agent.Stage3.Process(ctx, result)
	if err != nil {
		return agentctx.ParsedResponse{}, false, err
	}

	// Emit text as a single chunk for streaming display
	if parsed.Text != "" {
		c.out <- StreamEvent{Type: agentctx.StreamText, Text: parsed.Text}
	}

	// Record success
	if c.agent.Router != nil {
		c.agent.Router.RecordSuccess(p.Name())
	}

	return parsed, true, nil
}

// processStreamingEvents handles streaming events and constructs the parsed response.
func (c *streamingCaller) processStreamingEvents(streamCh <-chan agentctx.StreamEvent, p provider.Provider, modelName string) (agentctx.ParsedResponse, error) {
	var fullText string
	var fullReasoning strings.Builder
	var toolCalls []agentctx.ToolCall
	var currentToolID string
	var currentToolName string
	var currentToolInput strings.Builder
	var lastUsage *agentctx.UsageInfo
	var stopReason string

	for event := range streamCh {
		// Check interrupt during streaming
		if c.convCtx.Interrupt.IsSet() {
			c.out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
			return agentctx.ParsedResponse{}, fmt.Errorf("interrupted")
		}

		switch event.Type {
		case agentctx.StreamText:
			c.out <- StreamEvent{Type: agentctx.StreamText, Text: event.Text}
			fullText += event.Text
		case agentctx.StreamToolStart:
			currentToolID = event.ToolCallID
			currentToolName = event.ToolName
			currentToolInput.Reset()
			c.out <- StreamEvent{Type: agentctx.StreamToolStart, ToolName: event.ToolName, ToolCallID: event.ToolCallID}
		case agentctx.StreamToolDelta:
			currentToolInput.WriteString(event.ToolInputDelta)
		case agentctx.StreamToolEnd:
			toolCalls = append(toolCalls, agentctx.ToolCall{
				ID:        currentToolID,
				Name:      currentToolName,
				Arguments: json.RawMessage(currentToolInput.String()),
			})
		case agentctx.StreamError:
			// Phase 37: provider-level fallback is handled by decorators.
			// Mid-stream errors are unrecoverable — record and return.
			c.recordFailure(p, modelName, event.Error)
			return agentctx.ParsedResponse{}, fmt.Errorf("provider streaming error: %w", event.Error)

		case agentctx.StreamReasoning:
			c.out <- StreamEvent{Type: agentctx.StreamReasoning, Reasoning: event.Reasoning}
			fullReasoning.WriteString(event.Reasoning)

		case agentctx.StreamDone:
			lastUsage = event.Usage
			if event.StopReason != "" {
				stopReason = event.StopReason
			}
			c.out <- StreamEvent{Type: agentctx.StreamDone, Done: true, Usage: event.Usage}
		}
	}

	// Build parsed response
	parsed := agentctx.ParsedResponse{
		Text:      fullText,
		Reasoning: fullReasoning.String(),
		ToolCalls: toolCalls,
	}
	if stopReason != "" {
		parsed.FinishReason = stopReason
	} else if len(toolCalls) == 0 {
		parsed.FinishReason = "stop"
	} else {
		parsed.FinishReason = "tool_use"
	}

	// Track cost for this turn
	if c.agent.Router != nil && lastUsage != nil {
		c.agent.Router.RecordTurnCost(p.Name(), lastUsage.PromptTokens, lastUsage.CompletionTokens, c.convCtx.Iteration)
	}

	// Record success (reset failure counter)
	if c.agent.Router != nil {
		c.agent.Router.RecordSuccess(p.Name())
	}

	// Emit tool execution events before dispatch
	for _, tc := range toolCalls {
		c.out <- StreamEvent{Type: agentctx.StreamToolExec, ToolName: tc.Name, ToolCallID: tc.ID}
	}

	return parsed, nil
}

// recordFailure records a provider failure for routing demotion.
func (c *streamingCaller) recordFailure(p provider.Provider, modelName string, err error) {
	if c.agent.Router != nil {
		failName := p.Name()
		if modelName != "" {
			failName = modelName
		}
		c.agent.Router.RecordFailure(failName)
	}
}
