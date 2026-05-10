// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"fmt"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	agentlog "github.com/aj-nt/mnemosyne/agent/log"
)

// LoopDetectionEvent is the data payload for loop detection events.
// It provides structured information about why the pipeline stopped
// or yielded, enabling observability and metrics.
type LoopDetectionEvent struct {
	// Kind identifies the detection type: "stuck", "repetitive_loop",
	// "consecutive_loop", "overflow_compact", "overflow_terminate"
	Kind string

	// ToolName is the tool that triggered the detection (for repetitive loops).
	ToolName string

	// Iteration is the pipeline iteration when this was detected.
	Iteration int

	// TokenCount is the approximate token count at detection time (for overflow).
	TokenCount int

	// Message is a human-readable description of what happened.
	Message string
}

// ContextHandler is a link in the Chain of Responsibility for context management.
// Each handler checks a specific condition. If it handles the request (returns
// handled=true), the chain stops. Otherwise, it passes to the next handler.
type ContextHandler interface {
	// Handle checks a condition in the pipeline. Returns (action, handled).
	// If handled=true, the chain stops and the action is returned.
	// If handled=false, the next handler in the chain is called.
	Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) (agentctx.StageAction, bool)
}

// HandlerChain executes a sequence of ContextHandlers in order.
// The first handler that returns handled=true wins.
type HandlerChain struct {
	handlers []ContextHandler
}

// NewHandlerChain creates a chain from the given handlers.
func NewHandlerChain(handlers ...ContextHandler) *HandlerChain {
	return &HandlerChain{handlers: handlers}
}

// Handle walks the chain. Returns the action from the first handler that
// handles the request, or StageActionCONTINUE if none do.
func (c *HandlerChain) Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) agentctx.StageAction {
	for _, h := range c.handlers {
		action, handled := h.Handle(convCtx, toolResults, parsed)
		if handled {
			return action
		}
	}
	return agentctx.StageActionCONTINUE
}

// emitEvent is a helper to emit an event if the EventBus is non-nil.
func emitEvent(bus *eventbus.EventBus, event LoopDetectionEvent) {
	if bus == nil {
		return
	}
	bus.Emit(eventbus.Event{Kind: event.Kind, Data: event})
}

// --- Concrete handlers ---

// OverflowHandler checks if the context exceeds the token limit and
// attempts compaction. If compaction doesn't bring it under the limit,
// it terminates the loop. This must be PRIORITY 1 — a bloated context
// makes all other checks unreliable.
type OverflowHandler struct {
	MaxContextTokens int
	Thresholds      PipelineThresholds
	EventBus        *eventbus.EventBus
}

// Handle checks context overflow.
func (h *OverflowHandler) Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) (agentctx.StageAction, bool) {
	if h.MaxContextTokens <= 0 || !convCtx.IsOverTokenLimit(h.MaxContextTokens) {
		return agentctx.StageActionCONTINUE, false
	}

	// Try to compact
	convCtx.Compact(h.MaxContextTokens)

	// If still over after compaction, aggressively summarize
	if convCtx.IsOverTokenLimit(h.MaxContextTokens) {
		convCtx.SummarizeLargeToolResults(h.Thresholds.AggressiveSummaryThreshold)
		if convCtx.IsOverTokenLimit(h.MaxContextTokens) {
			emitEvent(h.EventBus, LoopDetectionEvent{
				Kind:      "overflow_terminate",
				Iteration: convCtx.Iteration,
				TokenCount: convCtx.TokenCount(),
				Message:   fmt.Sprintf("context overflow after compaction: %d tokens > %d limit", convCtx.TokenCount(), h.MaxContextTokens),
			})
			return agentctx.StageActionTERMINATE, true
		}
	}

	// Compaction succeeded — let later handlers run with the smaller context.
	emitEvent(h.EventBus, LoopDetectionEvent{
		Kind:      "overflow_compact",
		Iteration: convCtx.Iteration,
		TokenCount: convCtx.TokenCount(),
		Message:   fmt.Sprintf("context compacted: %d tokens (limit %d)", convCtx.TokenCount(), h.MaxContextTokens),
	})
	convCtx.CompactedThisTurn = true
	return agentctx.StageActionCONTINUE, false
}

// StuckHandler detects if the agent is stuck (consecutive errors or "not found"
// results). It yields a helpful message to break the loop.
type StuckHandler struct {
	Thresholds PipelineThresholds
	EventBus   *eventbus.EventBus
}

// Handle checks for stuck agent patterns.
func (h *StuckHandler) Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) (agentctx.StageAction, bool) {
	stuckFromResults := isStuck(toolResults)
	stuckFromHistory := isStuckFromHistory(convCtx.Messages, h.Thresholds.StuckHistoryWindow, h.Thresholds.StuckMinFailures)

	if stuckFromResults || stuckFromHistory {
		source := "results"
		if stuckFromHistory {
			source = "history"
		}
		emitEvent(h.EventBus, LoopDetectionEvent{
			Kind:      "stuck",
			Iteration: convCtx.Iteration,
			Message:   fmt.Sprintf("agent stuck: consecutive failures detected from %s", source),
		})
		convCtx.AddMessage(agentctx.Message{
			Role:    "assistant",
			Content: "I wasn't able to find what I was looking for after multiple attempts. Could you provide more detail or a different approach?",
		})
		return agentctx.StageActionYIELD, true
	}

	return agentctx.StageActionCONTINUE, false
}

// ProgressDetectionHandler detects when the agent stops making forward progress.
// It uses two complementary signals:
//
//  1. Duplicate progress signatures (same tools + results) — catches same-tool loops
//     like SearchMemories/not_found repeated 5x.
//
//  2. Intent loops (same purpose across different tools) — catches cross-tool spirals
//     where the agent keeps saying "Let me search..." but switches between FileSearch,
//     Execute, and FileRead without converging.
//
// The nudge is intentionally soft: it's a progress check, not a reprimand.
// The agent should feel free to continue if it has new information.
type ProgressDetectionHandler struct {
	Thresholds PipelineThresholds
	EventBus   *eventbus.EventBus
}

// Handle checks for progress stalls using progress signatures and intent loops.
func (h *ProgressDetectionHandler) Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) (agentctx.StageAction, bool) {
	// Skip after compaction — the model should get a chance with the smaller context.
	if convCtx.CompactedThisTurn {
		return agentctx.StageActionCONTINUE, false
	}

	// Only check progress when there are tool calls
	if len(parsed.ToolCalls) == 0 {
		return agentctx.StageActionCONTINUE, false
	}

	// Compute progress signature for this iteration, including intent from LLM text
	sig := agentctx.ComputeProgressSignature(parsed.ToolCalls, toolResults, parsed.Text)

	// Check if this signature duplicates a recent one
	isDuplicate := agentctx.IsDuplicateProgress(sig, convCtx.ProgressHistory)

	// Update progress history (keep last 10)
	convCtx.ProgressHistory = append(convCtx.ProgressHistory, sig)
	if len(convCtx.ProgressHistory) > 10 {
		convCtx.ProgressHistory = convCtx.ProgressHistory[len(convCtx.ProgressHistory)-10:]
	}

	if isDuplicate {
		convCtx.DuplicateProgressCount++
	} else {
		convCtx.DuplicateProgressCount = 0
	}

	// Check for intent loops: same purpose across different tools
	isIntentLoop := agentctx.IsIntentLoop(convCtx.ProgressHistory, h.Thresholds.IntentLoopThreshold)

	// Nudge zone: 3+ consecutive duplicates (same tool, same result pattern)
	// The nudge is a gentle progress check, not a reprimand.
	// The agent should feel free to continue if it has new information.
	if convCtx.DuplicateProgressCount >= 3 && convCtx.DuplicateProgressCount < h.Thresholds.ProgressYieldThreshold {
		nudgeMsg := fmt.Sprintf(
			"[system: You've called the same tools with the same results %d times. If you have new information, continue. Otherwise, summarize what you know and ask for guidance.]",
			convCtx.DuplicateProgressCount,
		)
		convCtx.Messages = append(convCtx.Messages,
			agentctx.Message{
				Role:    "user",
				Content: nudgeMsg,
			},
		)
		emitEvent(h.EventBus, LoopDetectionEvent{
			Kind:      "progress_nudge",
			Iteration: convCtx.Iteration,
			Message:   fmt.Sprintf("progress nudge: %d duplicate iterations (tools: %s, results: %s, intent: %s)", convCtx.DuplicateProgressCount, sig.ToolNames, sig.ResultTag, sig.IntentTag),
		})
		return agentctx.StageActionCONTINUE, false
	}

	// Intent nudge: same purpose across different tools, but not yet at yield threshold.
	// This fires independently of the duplicate check — even if the signature matches
	// a previous one, we still want to flag when the agent is pursuing the same GOAL
	// across different tools. The progress nudge above handles "same tool+result" loops,
	// while this catches "different tools, same intent" spirals.
	// We skip only if we already nudged for duplicates this turn.
	nudgedForDuplicates := convCtx.DuplicateProgressCount >= 3 && convCtx.DuplicateProgressCount < h.Thresholds.ProgressYieldThreshold
	if isIntentLoop && !nudgedForDuplicates && convCtx.DuplicateProgressCount < h.Thresholds.ProgressYieldThreshold {
		// Find the dominant intent
		intentCounts := make(map[string]int)
		recentStart := len(convCtx.ProgressHistory) - 6
		if recentStart < 0 {
			recentStart = 0
		}
		for i := recentStart; i < len(convCtx.ProgressHistory); i++ {
			intentCounts[convCtx.ProgressHistory[i].IntentTag]++
		}
		dominantIntent := ""
		maxCount := 0
		for intent, count := range intentCounts {
			if count > maxCount {
				maxCount = count
				dominantIntent = intent
			}
		}

		nudgeMsg := fmt.Sprintf(
			"[system: Note: you've been %s several times. If you have new information, keep going. If stuck, try a different approach.]",
			intentVerb(dominantIntent),
		)
		convCtx.Messages = append(convCtx.Messages,
			agentctx.Message{
				Role:    "user",
				Content: nudgeMsg,
			},
		)
		emitEvent(h.EventBus, LoopDetectionEvent{
			Kind:      "intent_nudge",
			Iteration: convCtx.Iteration,
			Message:   fmt.Sprintf("intent nudge: repeating purpose '%s' across %d iterations (tools: %s)", dominantIntent, maxCount, sig.ToolNames),
		})
		return agentctx.StageActionCONTINUE, false
	}

	// Yield zone: ProgressYieldThreshold or more consecutive duplicates
	if convCtx.DuplicateProgressCount >= h.Thresholds.ProgressYieldThreshold {
		agentlog.Printf("ProgressDetectionHandler: YIELD — %d consecutive no-progress iterations (iteration %d, signature: %s/%s, intent: %s)",
			convCtx.DuplicateProgressCount, convCtx.Iteration, sig.ToolNames, sig.ResultTag, sig.IntentTag)
		emitEvent(h.EventBus, LoopDetectionEvent{
			Kind:      "progress_stall",
			Iteration: convCtx.Iteration,
			Message:   fmt.Sprintf("progress stall: %d consecutive no-progress iterations (tools: %s, results: %s, intent: %s)", convCtx.DuplicateProgressCount, sig.ToolNames, sig.ResultTag, sig.IntentTag),
		})
		return agentctx.StageActionYIELD, true
	}

	return agentctx.StageActionCONTINUE, false
}

// intentVerb converts an intent bucket to a human-readable verb phrase
// for nudge messages. "search" → "searching", "execute" → "running commands", etc.
func intentVerb(bucket string) string {
	switch bucket {
	case "search":
		return "searching"
	case "read":
		return "reading files"
	case "execute":
		return "running commands"
	case "write":
		return "writing files"
	case "memory":
		return "saving memories"
	default:
		return "pursuing the same goal"
	}
}

// AppendAndContinueHandler appends assistant messages and tool results to the
// conversation, then checks iteration and overflow limits. This is always
// the last handler in the chain — it does the bookkeeping that allows the
// loop to continue.
type AppendAndContinueHandler struct {
	MaxContextTokens int
	Thresholds      PipelineThresholds
	EventBus        *eventbus.EventBus
}

// Handle appends messages and checks post-append limits.
func (h *AppendAndContinueHandler) Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) (agentctx.StageAction, bool) {
	// Append assistant message with tool calls
	assistantMsg := agentctx.Message{
		Role:      "assistant",
		Content:   parsed.Text,
		ToolCalls: parsed.ToolCalls,
	}
	convCtx.AddMessage(assistantMsg)

	// Append tool results (with truncation)
	convCtx.AppendToolResults(toolResults)

	// Check iteration limit
	if convCtx.Iteration >= convCtx.MaxIterations {
		emitEvent(h.EventBus, LoopDetectionEvent{
			Kind:      "max_iterations",
			Iteration: convCtx.Iteration,
			Message:   fmt.Sprintf("max iterations reached: %d >= %d", convCtx.Iteration, convCtx.MaxIterations),
		})
		return agentctx.StageActionTERMINATE, true
	}

	// Check context token limit again after appending
	if h.MaxContextTokens > 0 && convCtx.IsOverTokenLimit(h.MaxContextTokens) {
		convCtx.Compact(h.MaxContextTokens)

		if convCtx.IsOverTokenLimit(h.MaxContextTokens) {
			// Aggressively truncate large tool results
			convCtx.SummarizeLargeToolResults(h.Thresholds.AggressiveSummaryThreshold)
			if convCtx.IsOverTokenLimit(h.MaxContextTokens) {
				emitEvent(h.EventBus, LoopDetectionEvent{
					Kind:       "overflow_terminate",
					Iteration:  convCtx.Iteration,
					TokenCount: convCtx.TokenCount(),
					Message:    fmt.Sprintf("context overflow after append: %d tokens > %d limit", convCtx.TokenCount(), h.MaxContextTokens),
				})
				return agentctx.StageActionTERMINATE, true
			}
		}
	}

	// Continue the loop to process tool results
	return agentctx.StageActionCONTINUE, true
}

// ContextManagementStage appends results, checks limits, and decides whether to loop.
// When the context exceeds the token limit, it attempts to compact by removing old
// tool call/result pairs before terminating.
//
// Internally, it uses a Chain of Responsibility pattern: a sequence of handlers
// that each check a specific condition. The first handler that "handles" the
// request determines the action. The chain order is:
//
//	1. OverflowHandler          — compact/terminate on token overflow
//	2. StuckHandler             — yield on consecutive errors/not-found
//	3. RepetitiveLoopHandler    — yield on same-tool loops (threshold defaults to 3)
//	4. ProgressDetectionHandler — detect no-progress spirals via progress signatures
//	5. AppendAndContinueHandler — append messages + check limits, then CONTINUE
type ContextManagementStage struct {
	MaxContextTokens int
	Thresholds      PipelineThresholds
	EventBus        *eventbus.EventBus
	chain            *HandlerChain
}

// NewContextManagementStage creates a new Stage 5 with the default handler chain.
func NewContextManagementStage(maxContextTokens int) *ContextManagementStage {
	return NewContextManagementStageWithThresholds(maxContextTokens, DefaultThresholds())
}

// NewContextManagementStageWithThresholds creates a new Stage 5 with custom thresholds.
func NewContextManagementStageWithThresholds(maxContextTokens int, thresholds PipelineThresholds) *ContextManagementStage {
	s := &ContextManagementStage{
		MaxContextTokens: maxContextTokens,
		Thresholds:       thresholds,
	}
	s.chain = s.defaultChain()
	return s
}

// SetEventBus sets the EventBus for all handlers. This must be called
// before Process() if observability is desired.
func (s *ContextManagementStage) SetEventBus(bus *eventbus.EventBus) {
	s.EventBus = bus
	s.chain = s.defaultChain()
}

// defaultChain builds the standard handler chain used in production.
func (s *ContextManagementStage) defaultChain() *HandlerChain {
	return NewHandlerChain(
		&OverflowHandler{MaxContextTokens: s.MaxContextTokens, Thresholds: s.Thresholds, EventBus: s.EventBus},
		&StuckHandler{Thresholds: s.Thresholds, EventBus: s.EventBus},
		&ProgressDetectionHandler{Thresholds: s.Thresholds, EventBus: s.EventBus},
		&AppendAndContinueHandler{MaxContextTokens: s.MaxContextTokens, Thresholds: s.Thresholds, EventBus: s.EventBus},
	)
}

// Process updates the conversation context and returns the next action.
// If the LLM returned text (no tool calls), it returns YIELD immediately.
// Otherwise, it runs the handler chain.
func (s *ContextManagementStage) Process(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) agentctx.StageAction {
	// Reset per-turn state (always, even for YIELD paths)
	convCtx.CompactedThisTurn = false

	// If LLM just returned text (no tool calls), we're done
	if len(parsed.ToolCalls) == 0 {
		return agentctx.StageActionYIELD
	}

	// Run the handler chain
	action := s.chain.Handle(convCtx, toolResults, parsed)

	return action
}

// --- Helper functions that remain local to pipeline ---

// isStuckFromHistory checks conversation messages for recent tool failures.
// It looks at the last `window` tool results in the conversation history for
// `minFailures` or more consecutive errors or "not found" patterns.
func isStuckFromHistory(messages []agentctx.Message, window int, minFailures int) bool {
	// Collect the last few tool results from history
	var recentResults []struct {
		content string
		isError bool
	}

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "tool" {
			isErr := strings.HasPrefix(msg.Content, "Error: ") || msg.Content == ""
			recentResults = append(recentResults, struct {
				content string
				isError bool
			}{msg.Content, isErr})
			if len(recentResults) >= window {
				break
			}
		}
	}

	if len(recentResults) < minFailures {
		return false
	}

	// Check if the last N tool results all indicate failure
	failureCount := 0
	for _, r := range recentResults {
		if r.isError || agentctx.IsResultNotFound(r.content) {
			failureCount++
		}
	}

	// Stuck if all recent results are failures (at least minFailures)
	return failureCount >= minFailures && failureCount == len(recentResults)
}

// isStuck detects if the agent is stuck by checking if the last 2 tool results
// were both errors or contained "not found" messages.
func isStuck(results []agentctx.ToolResult) bool {
	if len(results) < 2 {
		return false
	}

	// Check the last 2 tool results
	lastResult := results[len(results)-1]
	secondLastResult := results[len(results)-2]

	// Check if both are errors
	if lastResult.IsError && secondLastResult.IsError {
		return true
	}

	// Check if both contain "not found" or "no such file" (case insensitive)
	if agentctx.IsResultNotFound(lastResult.Result) && agentctx.IsResultNotFound(secondLastResult.Result) {
		return true
	}

	return false
}

