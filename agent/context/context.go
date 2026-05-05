// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import (
	"fmt"
	"strings"
)

// Message is a conversation message in the agent's context.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// Truncation thresholds for tool results.
const (
	MaxToolResultLines = 100
	MaxToolResultChars = 4000
)

// ConversationContext holds the full state for a conversation loop.
type ConversationContext struct {
	SessionID       string
	Messages        []Message
	SystemPrompt    string
	Tools           []ToolDefinition
	MaxIterations   int
	Iteration       int
	Interrupt       *InterruptSignal
	HotBlockContent string
	AgentID         string
	Temperature     float64

	// CompactedThisTurn is set by the OverflowHandler to signal that
	// compaction was performed this turn. Loop detection handlers skip
	// their checks after compaction so the model gets a chance to
	// operate on the newly compacted context.
	CompactedThisTurn bool
}

// AddMessage appends a message to the conversation.
func (c *ConversationContext) AddMessage(msg Message) {
	c.Messages = append(c.Messages, msg)
}

// LastAssistantMessage returns the last assistant message, or nil.
func (c *ConversationContext) LastAssistantMessage() *Message {
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Role == "assistant" {
			return &c.Messages[i]
		}
	}
	return nil
}

// TokenCount estimates the total token count (rough: 4 chars per token).
func (c *ConversationContext) TokenCount() int {
	total := len(c.SystemPrompt)
	for _, m := range c.Messages {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Name) + len(tc.Arguments)
		}
	}
	return total / 4
}

// IsOverTokenLimit checks whether the context exceeds the given max token limit,
// using the effective max (90% safety margin) for the comparison.
func (c *ConversationContext) IsOverTokenLimit(maxTokens int) bool {
	if maxTokens <= 0 {
		return false
	}
	return c.TokenCount() > EffectiveMaxTokens(maxTokens)
}

// AppendToolResults appends tool results to the conversation, truncating long
// results to prevent context bloat.
func (c *ConversationContext) AppendToolResults(results []ToolResult) {
	for _, tr := range results {
		content := TruncateToolResult(tr.Result, tr.IsError)
		if tr.IsError {
			content = fmt.Sprintf("Error: %s", content)
		}
		c.AddMessage(Message{
			Role:       "tool",
			Content:    content,
			ToolCallID: tr.CallID,
			Name:       tr.Name,
		})
	}
}

// SummarizeLargeToolResults replaces tool result content larger than threshold
// with a summary placeholder. Used during aggressive compaction.
func (c *ConversationContext) SummarizeLargeToolResults(threshold int) {
	for i := range c.Messages {
		if c.Messages[i].Role == "tool" && len(c.Messages[i].Content) > threshold {
			c.Messages[i].Content = fmt.Sprintf("[tool result: %s (%d chars)]", c.Messages[i].Name, len(c.Messages[i].Content))
		}
	}
}

// Compact removes old messages from the conversation to bring it under
// the token limit. Strategy (in order):
//  1. For high iterations (5+), aggressively strip tool results > 200 chars
//  2. Remove old tool call/result pairs (keep the assistant text summary)
//  3. Remove oldest user/assistant pairs, preserving the most recent context
//
// The system prompt is not compacted — it's always included.
// Returns true if compaction was performed (even if it didn't fully bring
// the context under the limit — the caller should check IsOverTokenLimit again).
func (c *ConversationContext) Compact(maxTokens int) bool {
	if !c.IsOverTokenLimit(maxTokens) {
		return false
	}

	// Phase 0: For high iterations (5+), aggressively strip tool results
	if c.Iteration >= 5 {
		for i, msg := range c.Messages {
			if msg.Role == "tool" && len(msg.Content) > 200 {
				c.Messages[i].Content = fmt.Sprintf("[tool result: %s (%d chars)]", msg.Name, len(msg.Content))
			}
		}
	}

	// Phase 1: Strip old tool results and summarize tool calls
	// Find the last YIELD point (assistant text response with no tool calls)
	lastYieldIdx := -1
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Role == "assistant" && len(c.Messages[i].ToolCalls) == 0 && c.Messages[i].Content != "" {
			lastYieldIdx = i
			break
		}
	}

	if lastYieldIdx > 0 {
		// Compact messages before the last yield point
		var compacted []Message
		for i, msg := range c.Messages {
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
					compacted = append(compacted, Message{
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
		c.Messages = compacted
	}

	// Phase 2: If still over limit, remove oldest user/assistant pairs
	if c.TokenCount() > maxTokens {
		for len(c.Messages) > 2 && c.TokenCount() > maxTokens {
			// Remove the first user message and its response
			removed := 0
			filtered := make([]Message, 0, len(c.Messages))
			skipNext := false
			for i, msg := range c.Messages {
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
			c.Messages = filtered
		}
	}

	return true
}

// EffectiveMaxTokens returns the effective max tokens with a safety margin.
// TokenCount() uses chars/4 as a rough estimate, but actual tokenizers (BPE,
// tiktoken, etc.) may use more tokens per character for certain languages,
// code, or special sequences. We apply a 10% safety margin so the estimate
// doesn't let through requests that actually exceed the limit.
func EffectiveMaxTokens(maxTokens int) int {
	if maxTokens <= 0 {
		return maxTokens
	}
	// 90% safety margin — if estimate says 90% of limit, consider it at limit
	return int(float64(maxTokens) * 0.9)
}

// TruncateToolResult truncates long tool results to prevent context bloat.
// Keeps first half and last quarter of lines, with a summary in between.
// Error results are never truncated (they're usually short and important).
func TruncateToolResult(result string, isError bool) string {
	if isError {
		return result
	}

	lines := strings.Split(result, "\n")
	if len(lines) <= MaxToolResultLines && len(result) <= MaxToolResultChars {
		return result
	}

	// If the result has very few lines but many chars (single long line),
	// truncate by character count instead of lines.
	if len(lines) <= MaxToolResultLines/2 {
		half := MaxToolResultChars / 2
		if len(result) > half {
			return fmt.Sprintf("%s\n\n... (truncated: %d total chars, showing first %d)", result[:half], len(result), half)
		}
		return result
	}

	// Show head + tail: first half of MaxToolResultLines, then last quarter
	headCount := MaxToolResultLines / 2
	tailCount := MaxToolResultLines / 4
	if headCount > len(lines) {
		headCount = len(lines)
	}
	if tailCount > len(lines) {
		tailCount = len(lines)
	}
	head := strings.Join(lines[:headCount], "\n")
	tail := strings.Join(lines[len(lines)-tailCount:], "\n")

	return fmt.Sprintf("%s\n\n... (truncated: %d total lines, %d chars, showing first %d and last %d)\n\n%s",
		head, len(lines), len(result), headCount, tailCount, tail)
}

// IsResultNotFound checks if a tool result content indicates a "not found" condition.
func IsResultNotFound(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "no such file") ||
		strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "no results") ||
		strings.Contains(lower, "no matches") ||
		strings.Contains(lower, "0 files found") ||
		strings.Contains(lower, "empty result")
}

// joinNames joins tool names with commas for summaries.
func joinNames(names []string) string {
	result := names[0]
	for _, n := range names[1:] {
		result += ", " + n
	}
	return result
}