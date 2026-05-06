// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import "fmt"

// CompactionConfig holds thresholds for context compaction (the process
// of shrinking a conversation to fit within token limits).
//
// Defaults are defined by DefaultCompactionConfig(). Override individual
// fields in tests or config-driven construction.
type CompactionConfig struct {
	// AggressiveIterationThreshold is the iteration count at which
	// compaction aggressively strips tool results > AggressiveToolResultChars.
	// Default: 5 (after 5 iterations, the model has had multiple chances
	// to read tool details — force it to rely on accumulated understanding).
	AggressiveIterationThreshold int

	// AggressiveToolResultChars is the character count above which tool
	// results are replaced with "[tool result: Name (N chars)]" during
	// aggressive compaction. Default: 200.
	AggressiveToolResultChars int

	// TokenEstimateRatio is the ratio of characters to estimated tokens.
	// Default: 4 (4 characters ≈ 1 token for English text).
	TokenEstimateRatio int

	// EffectiveTokenMargin is the safety margin for token estimation.
	// Actual tokenizers may use more tokens per character, so we consider
	// the context "full" at EffectiveTokenMargin × maxTokens.
	// Default: 0.9 (90% of the nominal limit).
	EffectiveTokenMargin float64
}

// DefaultCompactionConfig returns production defaults for compaction.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		AggressiveIterationThreshold: 5,
		AggressiveToolResultChars:   200,
		TokenEstimateRatio:          4,
		EffectiveTokenMargin:        0.9,
	}
}

// CompactWithConfig removes old messages from the conversation to bring it under
// the token limit, using the provided config for thresholds. This is like Compact
// but with explicit config rather than hardcoded values.
func (c *ConversationContext) CompactWithConfig(maxTokens int, cfg CompactionConfig) bool {
	if !c.IsOverTokenLimit(maxTokens) {
		return false
	}

	// Phase 0: For high iterations, aggressively strip tool results
	if c.Iteration >= cfg.AggressiveIterationThreshold {
		for i, msg := range c.Messages {
			if msg.Role == "tool" && len(msg.Content) > cfg.AggressiveToolResultChars {
				c.Messages[i].Content = fmt.Sprintf("[tool result: %s (%d chars)]", msg.Name, len(msg.Content))
			}
		}
	}

	// Phase 1: Strip old tool results and summarize tool calls
	lastYieldIdx := -1
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Role == "assistant" && len(c.Messages[i].ToolCalls) == 0 && c.Messages[i].Content != "" {
			lastYieldIdx = i
			break
		}
	}

	if lastYieldIdx > 0 {
		var compacted []Message
		for i, msg := range c.Messages {
			switch msg.Role {
			case "user":
				compacted = append(compacted, msg)
			case "assistant":
				if i < lastYieldIdx && len(msg.ToolCalls) > 0 {
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
				if i > lastYieldIdx {
					compacted = append(compacted, msg)
				}
			default:
				compacted = append(compacted, msg)
			}
		}
		c.Messages = compacted
	}

	// Phase 2: If still over limit, remove oldest user/assistant pairs
	if c.TokenCount() > maxTokens {
		for len(c.Messages) > 2 && c.TokenCount() > maxTokens {
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
					skipNext = true
					removed++
					continue
				}
				filtered = append(filtered, msg)
			}
			if removed == 0 {
				break
			}
			c.Messages = filtered
		}
	}

	return true
}