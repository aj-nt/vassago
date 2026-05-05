// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"fmt"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

const (
	maxToolResultLines = 200
	maxToolResultChars = 8000
)

// truncateToolResult truncates long tool results to prevent context bloat.
// Keeps first half and last quarter of lines, with a summary in between.
// Error results are never truncated (they're usually short and important).
func truncateToolResult(result string, isError bool) string {
	if isError {
		return result
	}

	lines := strings.Split(result, "\n")
	if len(lines) <= maxToolResultLines && len(result) <= maxToolResultChars {
		return result
	}

	// Show head + tail: first half of maxToolResultLines, then last quarter
	headCount := maxToolResultLines / 2
	tailCount := maxToolResultLines / 4
	head := strings.Join(lines[:headCount], "\n")
	tail := strings.Join(lines[len(lines)-tailCount:], "\n")

	return fmt.Sprintf("%s\n\n... (truncated: %d total lines, %d chars, showing first %d and last %d)\n\n%s",
		head, len(lines), len(result), headCount, tailCount, tail)
}

// isResultNotFound checks if a tool result content indicates a "not found" condition.
func isResultNotFound(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "no such file") ||
		strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "no results") ||
		strings.Contains(lower, "no matches") ||
		strings.Contains(lower, "0 files found") ||
		strings.Contains(lower, "empty result")
}

// isStuckFromHistory checks conversation messages for recent tool failures.
// It looks at the last N tool results in the conversation history for consecutive
// errors or "not found" patterns. This is more robust than checking the toolResults
// parameter because it works even when loop detection fires before tool results
// are appended to the context.
func isStuckFromHistory(messages []agentctx.Message) bool {
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
			if len(recentResults) >= 3 {
				break // Check last 3 tool results
			}
		}
	}

	if len(recentResults) < 2 {
		return false
	}

	// Check if the last 2+ tool results all indicate failure
	failureCount := 0
	for _, r := range recentResults {
		if r.isError || isResultNotFound(r.content) {
			failureCount++
		}
	}

	// Stuck if all recent results are failures (at least 2)
	return failureCount >= 2 && failureCount == len(recentResults)
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
	if isResultNotFound(lastResult.Result) && isResultNotFound(secondLastResult.Result) {
		return true
	}

	return false
}

// ContextManagementStage appends results, checks limits, and decides whether to loop.
// When the context exceeds the token limit, it attempts to compact by removing old
// tool call/result pairs before terminating.
type ContextManagementStage struct {
	MaxContextTokens int
}

// NewContextManagementStage creates a new Stage 5.
func NewContextManagementStage(maxContextTokens int) *ContextManagementStage {
	return &ContextManagementStage{MaxContextTokens: maxContextTokens}
}

// Process updates the conversation context and returns the next action.
func (s *ContextManagementStage) Process(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) agentctx.StageAction {
	// If LLM just returned text (no tool calls), we're done
	if len(parsed.ToolCalls) == 0 {
		return agentctx.StageActionYIELD
	}

	// Detect stuck agent FIRST — check both current tool results and
	// conversation history for consecutive failures. This takes priority
	// over loop detection because stuck detection provides a helpful message
	// to the user, while loop detection just silently breaks the loop.
	stuckFromResults := isStuck(toolResults)
	stuckFromHistory := isStuckFromHistory(convCtx.Messages)
	if stuckFromResults || stuckFromHistory {
		// Replace tool calls with a text-only response asking for clarification
		convCtx.AddMessage(agentctx.Message{
			Role:    "assistant",
			Content: "I wasn't able to find what I was looking for after multiple attempts. Could you provide more detail or a different approach?",
		})
		return agentctx.StageActionYIELD
	}

	// Detect same-tool loop: if the same tool name appears 3+ times consecutively
	// in tool calls with no text response between iterations, break the loop.
	if isRepetitiveToolLoop(convCtx.Messages, parsed.ToolCalls) {
		return agentctx.StageActionYIELD
	}

	// Detect cross-tool loop: if there have been 4+ consecutive iterations with ANY tool calls,
	// break the loop to prevent agents from looping across different tools.
	if isConsecutiveToolIterationLoop(convCtx.Messages, parsed.ToolCalls) {
		return agentctx.StageActionYIELD
	}

	// Append assistant message with tool calls
	assistantMsg := agentctx.Message{
		Role:      "assistant",
		Content:   parsed.Text,
		ToolCalls: parsed.ToolCalls,
	}
	convCtx.AddMessage(assistantMsg)

	// Append tool results (truncated to prevent context bloat)
	for _, tr := range toolResults {
		content := truncateToolResult(tr.Result, tr.IsError)
		if tr.IsError {
			content = fmt.Sprintf("Error: %s", content)
		}
		convCtx.AddMessage(agentctx.Message{
			Role:       "tool",
			Content:    content,
			ToolCallID: tr.CallID,
			Name:       tr.Name,
		})
	}

	// Check iteration limit
	if convCtx.Iteration >= convCtx.MaxIterations {
		return agentctx.StageActionTERMINATE
	}

	// Check context token limit — try compaction before terminating
	if s.MaxContextTokens > 0 && convCtx.TokenCount() > agentctx.EffectiveMaxTokens(s.MaxContextTokens) {
		compactContext(convCtx, s.MaxContextTokens)

		// If still over limit after compaction, terminate
		if convCtx.TokenCount() > agentctx.EffectiveMaxTokens(s.MaxContextTokens) {
			return agentctx.StageActionTERMINATE
		}
	}

	// Continue the loop to process tool results
	return agentctx.StageActionCONTINUE
}

// compactContext removes old messages from the conversation to bring it under
// the token limit. Strategy (in order):
//  1. Remove old tool call/result pairs (keep the assistant text summary)
//  2. Remove oldest user/assistant pairs, preserving the most recent context
//
// The system prompt is not compacted — it's always included.
func compactContext(convCtx *agentctx.ConversationContext, maxTokens int) {
	// Phase 0: For high iterations (5+), aggressively strip tool results
	// The model has had multiple chances to read the details — now force it
	// to rely on accumulated understanding.
	if convCtx.Iteration >= 5 {
		for i, msg := range convCtx.Messages {
			if msg.Role == "tool" && len(msg.Content) > 200 {
				convCtx.Messages[i].Content = fmt.Sprintf("[tool result: %s (%d chars)]", msg.Name, len(msg.Content))
			}
		}
	}

	// Phase 1: Strip old tool results and summarize tool calls
	// Find the last YIELD point (assistant text response with no tool calls)
	lastYieldIdx := -1
	for i := len(convCtx.Messages) - 1; i >= 0; i-- {
		if convCtx.Messages[i].Role == "assistant" && len(convCtx.Messages[i].ToolCalls) == 0 && convCtx.Messages[i].Content != "" {
			lastYieldIdx = i
			break
		}
	}

	if lastYieldIdx > 0 {
		// Compact messages before the last yield point
		var compacted []agentctx.Message
		for i, msg := range convCtx.Messages {
			switch msg.Role {
			case "user":
				compacted = append(compacted, msg)
			case "assistant":
				if i < lastYieldIdx && len(msg.ToolCalls) > 0 {
					// Summarize old tool calls
					names := make([]string, 0, len(msg.ToolCalls))
					for _, tc := range msg.ToolCalls {
						names = append(names, tc.Name)
					}
					summary := "[Called " + joinNames(names) + "]"
					if msg.Content != "" {
						summary = msg.Content + "\n" + summary
					}
					compacted = append(compacted, agentctx.Message{
						Role:    "assistant",
						Content: summary,
					})
				} else {
					compacted = append(compacted, msg)
				}
			case "tool":
				// Only keep tool results after the last yield
				if i > lastYieldIdx {
					compacted = append(compacted, msg)
				}
				// Old tool results are dropped
			default:
				compacted = append(compacted, msg)
			}
		}
		convCtx.Messages = compacted
	}

	// Phase 2: If still over limit, remove oldest user/assistant pairs
	if convCtx.TokenCount() > maxTokens {
		for len(convCtx.Messages) > 2 && convCtx.TokenCount() > maxTokens {
			// Remove the first user message and its response
			// Find the first pair to remove
			removed := 0
			filtered := make([]agentctx.Message, 0, len(convCtx.Messages))
			skipNext := false
			for i, msg := range convCtx.Messages {
				if skipNext {
					skipNext = false
					removed++
					continue
				}
				if msg.Role == "user" && i > 0 && removed == 0 {
					// Skip this user message and the next assistant/tool message
					skipNext = true
					removed++
					continue
				}
				filtered = append(filtered, msg)
			}
			if removed == 0 {
				break // Nothing left to remove
			}
			convCtx.Messages = filtered
		}
	}
}

// joinNames joins tool names with commas for summaries.
func joinNames(names []string) string {
	result := names[0]
	for _, n := range names[1:] {
		result += ", " + n
	}
	return result
}

// isRepetitiveToolLoop detects if the same tool is being called repeatedly
// without a meaningful text response between iterations. This breaks infinite
// loops where the agent calls GetHotBlock (or similar tools) on every iteration.
//
// Returns true if any tool name in the current call appears 3+ times consecutively
// in the conversation. An assistant message with ONLY text (no tool calls)
// resets the counter, because that represents a genuine turn where the model
// responded to the user before deciding to use a tool again.
// An assistant message with BOTH text and tool calls does NOT reset the counter,
// because that's the loop pattern: model says something, calls GetHotBlock, gets
// the result, says something slightly different, calls GetHotBlock again.
func isRepetitiveToolLoop(messages []agentctx.Message, currentCalls []agentctx.ToolCall) bool {
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
		if count >= 3 {
			return true
		}
	}
	return false
}

// isConsecutiveToolIterationLoop checks if there have been 4+ consecutive iterations with ANY tool calls.
// This catches cross-tool loops (FileSearch→Execute→FileRead→repeat) that isRepetitiveToolLoop misses
// because it only checks for same-tool repetition. The threshold of 4 allows legitimate 3-step
// workflows (read→search→edit) while still breaking loops before they consume excessive turns.
func isConsecutiveToolIterationLoop(messages []agentctx.Message, currentCalls []agentctx.ToolCall) bool {
	count := 0

	// Walk backwards through messages
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}

		// Text-only message breaks the loop
		if len(msg.ToolCalls) == 0 && msg.Content != "" {
			break
		}

		// Count consecutive iterations with tool calls
		if len(msg.ToolCalls) > 0 {
			count++
			if count >= 4 {
				return true
			}
		}
	}

	// Include current calls in the count
	if len(currentCalls) > 0 {
		count++
		if count >= 4 {
			return true
		}
	}

	return false
}