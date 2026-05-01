package pipeline

import (
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

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

	// Detect same-tool loop: if the same tool name appears 3+ times consecutively
	// in tool calls with no text response between iterations, break the loop.
	if isRepetitiveToolLoop(convCtx.Messages, parsed.ToolCalls) {
		return agentctx.StageActionYIELD
	}

	// Append assistant message with tool calls
	assistantMsg := agentctx.Message{
		Role:      "assistant",
		Content:   parsed.Text,
		ToolCalls: parsed.ToolCalls,
	}
	convCtx.AddMessage(assistantMsg)

	// Append tool results
	for _, tr := range toolResults {
		content := tr.Result
		if tr.IsError {
			content = fmt.Sprintf("Error: %s", tr.Result)
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
	if s.MaxContextTokens > 0 && convCtx.TokenCount() > s.MaxContextTokens {
		compactContext(convCtx, s.MaxContextTokens)

		// If still over limit after compaction, terminate
		if convCtx.TokenCount() > s.MaxContextTokens {
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
// without any substantive text response between iterations. This breaks infinite
// loops where the agent calls GetHotBlock (or similar tools) on every iteration.
//
// Returns true if any tool name in the current call appears 3+ times consecutively
// in the conversation, with no intervening text-only assistant responses.
func isRepetitiveToolLoop(messages []agentctx.Message, currentCalls []agentctx.ToolCall) bool {
	// Count consecutive same-tool calls backward through the message history
	for _, call := range currentCalls {
		count := 1 // current call
		// Walk backward through messages
		for i := len(messages) - 1; i >= 0; i-- {
			msg := messages[i]
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
				// Check if this assistant message called the same tool
				for _, tc := range msg.ToolCalls {
					if tc.Name == call.Name {
						count++
						break // one match per message is enough
					}
				}
			} else if msg.Role == "assistant" && msg.Content != "" && len(msg.ToolCalls) == 0 {
				// Text-only assistant response breaks the loop
				break
			}
			// tool and user messages don't break the count
		}
		if count >= 3 {
			return true
		}
	}
	return false
}