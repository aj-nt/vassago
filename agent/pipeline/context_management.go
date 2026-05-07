// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"fmt"
	"log"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
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

// RepetitiveLoopHandler detects same-tool loops (e.g., GetHotBlock called
// N+ times consecutively). Skipped if the context was just compacted.
type RepetitiveLoopHandler struct {
	Thresholds PipelineThresholds
	EventBus   *eventbus.EventBus
}

// Handle checks for repetitive tool loops.
func (h *RepetitiveLoopHandler) Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) (agentctx.StageAction, bool) {
	// Skip loop detection after compaction — the model should get a chance
	// with the smaller context.
	if convCtx.CompactedThisTurn {
		return agentctx.StageActionCONTINUE, false
	}

	if isRepetitiveToolLoop(convCtx.Messages, parsed.ToolCalls, h.Thresholds.RepetitiveLoopThreshold) {
		// Identify the looping tool name for observability
		toolName := ""
		if len(parsed.ToolCalls) > 0 {
			toolName = parsed.ToolCalls[0].Name
		}
		emitEvent(h.EventBus, LoopDetectionEvent{
			Kind:      "repetitive_loop",
			ToolName:  toolName,
			Iteration: convCtx.Iteration,
			Message:   fmt.Sprintf("repetitive loop detected: %s called %d+ times consecutively", toolName, h.Thresholds.RepetitiveLoopThreshold),
		})
		return agentctx.StageActionYIELD, true
	}

	return agentctx.StageActionCONTINUE, false
}

// NarrationNudgeHandler injects a system message when the LLM produces
// consecutive tool-only responses without narration.
//
// DEPRECATED: This handler is superseded by ProgressDetectionHandler which
// uses progress signatures instead of consecutive counts. It remains in the
// codebase for backward compatibility (set NarrationNudgeThreshold > 0 to
// re-enable), but is NOT part of the default handler chain.
//
// Must be placed BEFORE ConsecutiveLoopHandler in the chain.
// Disabled when NarrationNudgeThreshold is 0.
type NarrationNudgeHandler struct {
	Thresholds PipelineThresholds
	EventBus   *eventbus.EventBus
}

// Handle injects a narration nudge if consecutive tool-only iterations
// are detected. Returns CONTINUE (not handled) so the chain continues
// to ConsecutiveLoopHandler.
func (h *NarrationNudgeHandler) Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) (agentctx.StageAction, bool) {
	// Disabled?
	if h.Thresholds.NarrationNudgeThreshold <= 0 {
		return agentctx.StageActionCONTINUE, false
	}

	// Skip after compaction
	if convCtx.CompactedThisTurn {
		return agentctx.StageActionCONTINUE, false
	}

	// Only nudge if this is a tool-only response (no text)
	if parsed.Text != "" {
		return agentctx.StageActionCONTINUE, false
	}

	// Only nudge if we've hit the nudge threshold but NOT the YIELD threshold
	consecutiveCount := countConsecutiveToolIterations(convCtx.Messages, parsed.ToolCalls)
	if consecutiveCount < h.Thresholds.NarrationNudgeThreshold {
		return agentctx.StageActionCONTINUE, false
	}

	// Already at or above YIELD threshold? Let ConsecutiveLoopHandler handle it.
	if consecutiveCount >= h.Thresholds.ConsecutiveLoopThreshold {
		return agentctx.StageActionCONTINUE, false
	}

	// Inject a gentle nudge into the conversation context.
	// This goes into the messages so the LLM sees it on the next iteration.
	convCtx.Messages = append(convCtx.Messages,
		agentctx.Message{
			Role:    "user",
			Content: "[system: You've made " + fmt.Sprintf("%d", consecutiveCount) + " tool call(s) without narrating. Please briefly explain your actions using text before making additional tool calls.]",
		},
	)

	emitEvent(h.EventBus, LoopDetectionEvent{
		Kind:      "narration_nudge",
		Iteration: convCtx.Iteration,
		Message:   fmt.Sprintf("narration nudge injected after %d consecutive tool-only iterations", consecutiveCount),
	})

	// Return CONTINUE (not handled) — the chain continues.
	// The nudge message is now in the conversation for the next LLM call.
	return agentctx.StageActionCONTINUE, false
}

// ConsecutiveLoopHandler detects cross-tool loops (N+ consecutive tool
// iterations regardless of tool name). Skipped after compaction.
//
// DEPRECATED: This handler is superseded by ProgressDetectionHandler which
// uses progress signatures instead of consecutive counts. It remains in the
// codebase for backward compatibility (set ConsecutiveLoopThreshold > 0 to
// re-enable), but is NOT part of the default handler chain.
type ConsecutiveLoopHandler struct {
	Thresholds PipelineThresholds
	EventBus   *eventbus.EventBus
}

// Handle checks for consecutive iteration loops.
func (h *ConsecutiveLoopHandler) Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) (agentctx.StageAction, bool) {
	// Disabled when threshold is 0
	if h.Thresholds.ConsecutiveLoopThreshold <= 0 {
		return agentctx.StageActionCONTINUE, false
	}

	// Skip loop detection after compaction
	if convCtx.CompactedThisTurn {
		return agentctx.StageActionCONTINUE, false
	}

	if isConsecutiveToolIterationLoop(convCtx.Messages, parsed.ToolCalls, h.Thresholds.ConsecutiveLoopThreshold) {
		log.Printf("ConsecutiveLoopHandler: YIELD — %d+ tool iterations without text response (iteration %d)", h.Thresholds.ConsecutiveLoopThreshold, convCtx.Iteration)
		emitEvent(h.EventBus, LoopDetectionEvent{
			Kind:      "consecutive_loop",
			Iteration: convCtx.Iteration,
			Message:   fmt.Sprintf("consecutive loop detected: %d+ tool iterations without text response", h.Thresholds.ConsecutiveLoopThreshold),
		})
		return agentctx.StageActionYIELD, true
	}

	return agentctx.StageActionCONTINUE, false
}

// ProgressDetectionHandler detects when the agent stops making forward progress.
// It tracks "progress signatures" (tool names + result tag) across iterations.
// If an iteration's signature duplicates a recent one, the handler increments
// a duplicate counter. After ProgressYieldThreshold duplicates, it YIELDs.
// It also detects oscillation: if the same tool appears 3+ times across the
// last 6 signatures (even in different combinations), that's a spiral too.
//
// This is smarter than counting consecutive tool calls because it distinguishes:
//   - PROGRESS: different tools, different results → signature is novel → counter resets
//   - REPETITION: same tools, same results → signature duplicates → counter increments
//   - SPIRAL: oscillating patterns (A→B→A→B) → partial overlap → counter increments
//   - WANDERING: many different tools without converging → oscillation detection
//
// A narration nudge is injected after 2+ consecutive duplicates or when
// oscillation is detected (but before YIELD), giving the model a chance to
// self-correction.
type ProgressDetectionHandler struct {
	Thresholds PipelineThresholds
	EventBus   *eventbus.EventBus
}

// Handle checks for progress stalls using progress signatures.
func (h *ProgressDetectionHandler) Handle(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) (agentctx.StageAction, bool) {
	// Skip after compaction — the model should get a chance with the smaller context.
	if convCtx.CompactedThisTurn {
		return agentctx.StageActionCONTINUE, false
	}

	// Only check progress when there are tool calls
	if len(parsed.ToolCalls) == 0 {
		return agentctx.StageActionCONTINUE, false
	}

	// Compute progress signature for this iteration
	sig := agentctx.ComputeProgressSignature(parsed.ToolCalls, toolResults)

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

	// Detect oscillation: same tool appearing 3+ times in last 6 signatures
	// even if exact signatures differ (e.g., SearchMemories→FileSearch→SearchMemories→FileSearch)
	isOscillating := agentctx.IsOscillatingProgress(convCtx.ProgressHistory)

	// Nudge zone: 2+ consecutive duplicates OR oscillation detected
	// Nudge regardless of whether text was produced — "Let me check..." doesn't count
	if (convCtx.DuplicateProgressCount >= 2 || isOscillating) && convCtx.DuplicateProgressCount < h.Thresholds.ProgressYieldThreshold {
		convCtx.Messages = append(convCtx.Messages,
			agentctx.Message{
				Role:    "user",
				Content: fmt.Sprintf("[system: You've made %d tool calls without converging. Stop searching and summarize what you've found so far, or ask the user for clarification.]", convCtx.Iteration+1),
			},
		)
		emitEvent(h.EventBus, LoopDetectionEvent{
			Kind:      "progress_nudge",
			Iteration: convCtx.Iteration,
			Message:   fmt.Sprintf("progress nudge: %d iterations without convergence (duplicate=%d, oscillating=%v)", convCtx.Iteration+1, convCtx.DuplicateProgressCount, isOscillating),
		})
		return agentctx.StageActionCONTINUE, false
	}

	// Yield zone: ProgressYieldThreshold or more duplicates, or oscillation past threshold
	if convCtx.DuplicateProgressCount >= h.Thresholds.ProgressYieldThreshold || (isOscillating && convCtx.DuplicateProgressCount >= 1) {
		log.Printf("ProgressDetectionHandler: YIELD — no convergence (iteration %d, duplicate=%d, oscillating=%v, signature: %s/%s)",
			convCtx.Iteration, convCtx.DuplicateProgressCount, isOscillating, sig.ToolNames, sig.ResultTag)
		emitEvent(h.EventBus, LoopDetectionEvent{
			Kind:      "progress_stall",
			Iteration: convCtx.Iteration,
			Message:   fmt.Sprintf("progress stall: iteration %d, duplicate=%d, oscillating=%v (tools: %s, results: %s)", convCtx.Iteration, convCtx.DuplicateProgressCount, isOscillating, sig.ToolNames, sig.ResultTag),
		})
		return agentctx.StageActionYIELD, true
	}

	return agentctx.StageActionCONTINUE, false
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
		&RepetitiveLoopHandler{Thresholds: s.Thresholds, EventBus: s.EventBus},
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

// countConsecutiveToolIterations counts how many consecutive tool-only
// iterations exist ending at the current calls. This is used by the
// deprecated NarrationNudgeHandler for backward compatibility.
func countConsecutiveToolIterations(messages []agentctx.Message, currentCalls []agentctx.ToolCall) int {
	count := 0

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "user" {
			break
		}
		if msg.Role != "assistant" {
			continue
		}
		if msg.Content != "" {
			break
		}
		if len(msg.ToolCalls) > 0 {
			count++
		}
	}

	if len(currentCalls) > 0 {
		count++
	}

	return count
}

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

// isRepetitiveToolLoop detects if the same tool is being called repeatedly
// without a meaningful text response between iterations. The threshold
// parameter controls how many consecutive same-tool calls trigger a loop.
func isRepetitiveToolLoop(messages []agentctx.Message, currentCalls []agentctx.ToolCall, threshold int) bool {
	for _, call := range currentCalls {
		count := 1 // current call
		for i := len(messages) - 1; i >= 0; i-- {
			msg := messages[i]
			if msg.Role == "assistant" {
				if len(msg.ToolCalls) == 0 && msg.Content != "" {
					// Text-only response: the model gave a real answer before
					// deciding to use a tool again. This breaks the loop.
					break
				}
				// Assistant message with tool calls (or empty): check for same tool
				for _, tc := range msg.ToolCalls {
					if tc.Name == call.Name {
						count++
						break // one match per message is enough
					}
				}
			}
			// tool and user messages don't affect the count
		}
		if count >= threshold {
			return true
		}
	}
	return false
}

// isConsecutiveToolIterationLoop checks if there have been `threshold` or more
// consecutive iterations with ANY tool calls. This catches cross-tool loops
// that isRepetitiveToolLoop misses.
func isConsecutiveToolIterationLoop(messages []agentctx.Message, currentCalls []agentctx.ToolCall, threshold int) bool {
	count := 0

	// Walk backwards through messages
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]

		// A new user message resets the loop context — tool calls
		// from a previous question shouldn't count against the current one.
		if msg.Role == "user" {
			break
		}

		if msg.Role != "assistant" {
			continue
		}

		// An assistant message with text content means the agent is narrating
		// its actions — this breaks the "silent tool loop" pattern.
		if msg.Content != "" {
			break
		}

		// Count this as a tool-only iteration
		if len(msg.ToolCalls) > 0 {
			count++
			if count >= threshold {
				return true
			}
		}
	}

	// Include current calls in the count
	if len(currentCalls) > 0 {
		count++
		if count >= threshold {
			return true
		}
	}

	return false
}