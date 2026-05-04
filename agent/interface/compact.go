// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"strings"
	"unicode/utf8"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// compactMessagesForNextTurn removes old tool call/result pairs from the message
// history before sending to the LLM on a new user turn. Tool results are essential
// within the current agentic loop (the model needs them to form its next response),
// but sending them again on subsequent turns causes the model to re-read and
// re-state information from prior tool calls — producing the repetition bug where
// the agent re-asserts facts it already mentioned (e.g., "Your name is AJ,
// goes by AJ" in every single response).
//
// Additionally, old assistant text responses are truncated to prevent the model from
// pattern-matching prior verbose responses and regurgitating them. The model tends
// to see "I said X before, so I should say X again" — even with the anti-repetition
// directive in the system prompt.
//
// Strategy:
//   - Keep all user messages intact
//   - Replace old assistant messages that contained tool calls with a summary:
//     "[Called {tool1}, {tool2}, ...]" — preserving the fact that tools were called
//     without the full input/output
//   - Truncate old assistant text responses to a maximum length, adding "..."
//     if they were longer. This prevents the model from seeing and re-stating
//     verbose prior responses.
//   - Remove the corresponding tool result messages entirely
//   - Keep the most recent assistant message and its tool calls/results intact if
//     the model is still in a tool-calling loop (no final text response yet)
const (
	// maxOldAssistantChars is the maximum number of characters to keep from
	// old assistant text responses. Responses longer than this are truncated
	// with "..." to prevent the model from re-reading and re-stating verbose
	// prior responses.
	//
	// 300 chars ≈ 75 tokens — enough to preserve the gist of what was said
	// without giving the model enough context to parrot it back verbatim.
	maxOldAssistantChars = 300

	// maxOldAssistantLines is the maximum number of lines to keep from old
	// assistant responses. Even if under the char limit, very long multi-line
	// responses (like comparison tables) trigger repetition.
	maxOldAssistantLines = 5
)

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

	// Find the SECOND-to-last yield point. Everything before it is "old"
	// and can have its assistant text truncated. The most recent exchange
	// (from second-to-last yield to end) keeps full text.
	secondYieldIdx := -1
	for i := lastYieldIdx - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && len(messages[i].ToolCalls) == 0 && messages[i].Content != "" {
			secondYieldIdx = i
			break
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
				// Plain text response
				if secondYieldIdx >= 0 && i < secondYieldIdx {
					// OLD assistant text — truncate to prevent repetition
					result = append(result, agentctx.Message{
						Role:    "assistant",
						Content: truncateAssistantText(msg.Content),
					})
				} else {
					// Recent assistant text — keep intact
					result = append(result, msg)
				}
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

// truncateAssistantText shortens old assistant text responses to prevent
// the model from re-reading and re-stating verbose prior responses.
// It limits both character count and line count, whichever limit is hit first.
func truncateAssistantText(text string) string {
	if utf8.RuneCountInString(text) <= maxOldAssistantChars && strings.Count(text, "\n")+1 <= maxOldAssistantLines {
		return text
	}

	// Split into lines and keep only the first (maxOldAssistantLines - 1)
	// so the "..." suffix fits within the line budget.
	maxLines := maxOldAssistantLines - 1
	if maxLines < 1 {
		maxLines = 1
	}
	lines := strings.SplitN(text, "\n", maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	result := strings.Join(lines, "\n")
	if utf8.RuneCountInString(result) > maxOldAssistantChars {
		// Truncate to maxOldAssistantChars, being careful not to split mid-rune
		runes := []rune(result)
		if len(runes) > maxOldAssistantChars {
			result = string(runes[:maxOldAssistantChars])
		}
	}

	return result + "\n..."
}