// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"context"
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
)

// ProviderCaller abstracts how the LLM provider is called (blocking vs streaming).
// This is the Strategy pattern — RunWithContext uses BlockingCaller, RunStreaming
// uses a StreamingCaller. The rest of the pipeline loop is shared.
//
// The Caller is responsible for Stages 2-3 (provider call + response parsing).
// Optionally, callers for streaming fallback may also run Stages 4-5 and return
// a complete LoopTurnResult with the StageAction already set.
type ProviderCaller interface {
	// Call invokes the LLM provider and returns the parsed response.
	// Returns an error if the provider call fails entirely.
	// The boolean ok=false means the caller should stop the loop
	// (e.g., a fatal streaming error where cleanup was already done).
	Call(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (parsed agentctx.ParsedResponse, ok bool, err error)
}

// LoopResult is the result of the pipeline loop.
// Both RunWithContext and RunStreaming translate this into their
// mode-specific result type.
type LoopResult struct {
	Parsed           agentctx.ParsedResponse
	Action           agentctx.StageAction
	Iterations       int
	Interrupted      bool
	MaxIterationsHit bool
	ContextOverflow  bool
	StoppedOnError  bool
	Error            error
}

// LoopConfig holds the configuration for the pipeline loop.
type LoopConfig struct {
	Stages               *Stages
	MaxContextTokens     int
	EventBus             *eventbus.EventBus
	PersistFunc          func(sessionID string)
	MaxConsecutiveErrors int
}

// Stages holds the 5 pipeline stages.
type Stages struct {
	Stage1 *RequestPrepStage
	Stage2 *ProviderCallStage
	Stage3 *ResponseProcessingStage
	Stage4 *ToolDispatchStage
	Stage5 *ContextManagementStage
}

// RunPipelineLoop is the Template Method for the 5-stage agent loop.
// It handles the common logic shared by RunWithContext and RunStreaming:
//   - Termination checks (interrupt, max iterations, context overflow)
//   - Stage 1 (request prep), Stage 4 (tool dispatch), Stage 5 (context management)
//   - Action handling (YIELD/TERMINATE/CONTINUE)
//   - Consecutive error tracking
//
// The ProviderCaller parameter provides the mode-specific Stage 2-3 logic.
// For streaming, the caller may also run Stages 4-5 in fallback scenarios,
// indicated by SkipStages45=true in the return.
func RunPipelineLoop(ctx context.Context, convCtx *agentctx.ConversationContext, cfg *LoopConfig, caller ProviderCaller, sessionID string) *LoopResult {
	if cfg.MaxConsecutiveErrors <= 0 {
		cfg.MaxConsecutiveErrors = 3
	}
	consecutiveErrors := 0

	for {
		// Check termination conditions
		if convCtx.Interrupt.IsSet() {
			return &LoopResult{Interrupted: true, Iterations: convCtx.Iteration}
		}
		if convCtx.Iteration >= convCtx.MaxIterations {
			return &LoopResult{MaxIterationsHit: true, Iterations: convCtx.Iteration}
		}
		// Check context token limit (with safety margin for estimate inaccuracy)
		if cfg.MaxContextTokens > 0 && convCtx.TokenCount() > agentctx.EffectiveMaxTokens(cfg.MaxContextTokens) {
			return &LoopResult{MaxIterationsHit: true, ContextOverflow: true, Iterations: convCtx.Iteration}
		}

		// Emit iteration start event
		if cfg.EventBus != nil {
			cfg.EventBus.Emit(eventbus.Event{Kind: "iteration_start", Data: convCtx.Iteration})
		}

		// Stage 1: Request Preparation
		prepared := cfg.Stages.Stage1.Process(convCtx)

		// Stages 2-3: Provider call (mode-specific via Strategy)
		parsed, ok, err := caller.Call(ctx, convCtx, prepared)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= cfg.MaxConsecutiveErrors {
				return &LoopResult{
					Error:          fmt.Errorf("provider failed %d times: %w", consecutiveErrors, err),
					StoppedOnError: true,
				}
			}
			// Increment iteration on error to prevent infinite loop
			convCtx.Iteration++
			continue
		}
		if !ok {
			// Caller handled the result itself (e.g., streaming error with cleanup)
			return &LoopResult{StoppedOnError: true, Iterations: convCtx.Iteration}
		}
		consecutiveErrors = 0 // reset on success

		// Emit response event
		if cfg.EventBus != nil {
			cfg.EventBus.Emit(eventbus.Event{Kind: "response", Data: parsed})
		}

		// Stage 4: Tool Dispatch
		dispatch := cfg.Stages.Stage4.Process(convCtx, parsed)

		// Stage 5: Context Management
		action := cfg.Stages.Stage5.Process(convCtx, dispatch.ToolResults, parsed)

		// Emit iteration end event
		if cfg.EventBus != nil {
			cfg.EventBus.Emit(eventbus.Event{Kind: "iteration_end", Data: action})
		}

		convCtx.Iteration++

		// Handle action
		switch action {
		case agentctx.StageActionYIELD:
			// If the LLM produced no text (only tool calls), inject a helpful message
			// so the user doesn't see a silent exit with just tool icons.
			if parsed.Text == "" {
				parsed.Text = "I've been making too many tool calls without responding. Let me answer your question directly."
			}
			// Add a recovery message to the conversation history so subsequent turns
			// don't repeat the same pattern. This teaches the model what went wrong.
			convCtx.Messages = append(convCtx.Messages,
				agentctx.Message{Role: "assistant", Content: parsed.Text},
				agentctx.Message{Role: "user", Content: "[system: The assistant made multiple tool calls without generating text. Please respond with text before making any additional tool calls.]"},
			)
			if cfg.PersistFunc != nil {
				cfg.PersistFunc(sessionID)
			}
			return &LoopResult{
				Parsed:     parsed,
				Action:     action,
				Iterations: convCtx.Iteration,
			}
		case agentctx.StageActionTERMINATE:
			if cfg.PersistFunc != nil {
				cfg.PersistFunc(sessionID)
			}
			return &LoopResult{
				Parsed:           parsed,
				Action:           action,
				Iterations:       convCtx.Iteration,
				MaxIterationsHit: convCtx.Iteration >= convCtx.MaxIterations,
				ContextOverflow:  cfg.MaxContextTokens > 0 && convCtx.TokenCount() > agentctx.EffectiveMaxTokens(cfg.MaxContextTokens),
			}
		case agentctx.StageActionCONTINUE:
			// Continue the loop
		}
	}
}