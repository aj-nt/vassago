package iface

import (
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// compactMessagesForNextTurn removes old tool call/result pairs from the message
// history before sending to the LLM on a new user turn. Tool results are essential
// within the current agentic loop (the model needs them to form its next response),
// but sending them again on subsequent turns causes the model to re-read and
// re-state information from prior tool calls — producing the repetition bug where
// the agent re-asserts facts it already mentioned (e.g., "Your name is Paul Flinton,
// goes by AJ" in every single response).
//
// Strategy:
//   - Keep all user and assistant text messages intact
//   - Replace old assistant messages that contained tool calls with a summary:
//     "[Called {tool1}, {tool2}, ...]" — preserving the fact that tools were called
//     without the full input/output
//   - Remove the corresponding tool result messages entirely
//   - Keep the most recent assistant message and its tool calls/results intact if
//     the model is still in a tool-calling loop (no final text response yet)
func compactMessagesForNextTurn(messages []agentctx.Message) []agentctx.Message {
	if len(messages) == 0 {
		return messages
	}

	result := make([]agentctx.Message, 0, len(messages))

	// Track whether we're looking at messages from the "previous turn" or
	// the "current agentic loop". After a YIELD (assistant text with no tool calls),
	// everything before that point is "old" and should be compacted.
	// We scan from the end to find the last YIELD point.
	lastYieldIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && len(messages[i].ToolCalls) == 0 && messages[i].Content != "" {
			lastYieldIdx = i
			break
		}
	}

	// If the last message is a tool result (model hasn't yielded yet),
	// keep everything as-is — we're mid-loop
	if len(messages) > 0 {
		lastMsg := messages[len(messages)-1]
		if lastMsg.Role == "tool" || (lastMsg.Role == "assistant" && len(lastMsg.ToolCalls) > 0) {
			// Mid-loop: don't compact, let the agent finish its loop
			return messages
		}
	}

	for i, msg := range messages {
		switch msg.Role {
		case "user":
			// Keep all user messages
			result = append(result, msg)

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool calls — compact it
				if i <= lastYieldIdx {
					// This is an old tool-calling turn — summarize the tool calls
					toolNames := make([]string, 0, len(msg.ToolCalls))
					for _, tc := range msg.ToolCalls {
						toolNames = append(toolNames, tc.Name)
					}
					summary := "[Called " + strings.Join(toolNames, ", ") + "]"
					// Preserve any text the assistant said alongside the tool calls
					if msg.Content != "" {
						summary = msg.Content + "\n" + summary
					}
					result = append(result, agentctx.Message{
						Role:    "assistant",
						Content: summary,
					})
				} else {
					// Keep recent tool calls intact
					result = append(result, msg)
				}
			} else {
				// Plain text response — keep it
				result = append(result, msg)
			}

		case "tool":
			// Tool result — keep only if it follows a tool call we kept intact
			if i > lastYieldIdx {
				result = append(result, msg)
			}
			// Otherwise skip (the summary in the assistant message covers it)

		default:
			// Keep system messages and anything else
			result = append(result, msg)
		}
	}

	return result
}