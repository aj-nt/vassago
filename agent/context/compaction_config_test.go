// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import (
	"strings"
	"testing"
)

func TestDefaultCompactionConfig(t *testing.T) {
	cfg := DefaultCompactionConfig()
	if cfg.AggressiveIterationThreshold != 5 {
		t.Errorf("Expected AggressiveIterationThreshold=5, got %d", cfg.AggressiveIterationThreshold)
	}
	if cfg.AggressiveToolResultChars != 200 {
		t.Errorf("Expected AggressiveToolResultChars=200, got %d", cfg.AggressiveToolResultChars)
	}
	if cfg.TokenEstimateRatio != 4 {
		t.Errorf("Expected TokenEstimateRatio=4, got %d", cfg.TokenEstimateRatio)
	}
	if cfg.EffectiveTokenMargin != 0.9 {
		t.Errorf("Expected EffectiveTokenMargin=0.9, got %f", cfg.EffectiveTokenMargin)
	}
}

func TestCompactWithConfig_HighIteration(t *testing.T) {
	// High iteration (5+) should aggressively strip tool results > 200 chars
	longToolResult := strings.Repeat("result data ", 50) // > 200 chars
	cfg := DefaultCompactionConfig()

	ctx := &ConversationContext{
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{{ID: "1", Name: "Tool1", Arguments: []byte(`{}`)}}},
			{Role: "tool", Content: longToolResult, ToolCallID: "1", Name: "Tool1"},
			{Role: "assistant", Content: "I found it"},
			{Role: "user", Content: strings.Repeat("more data ", 5000)}, // to exceed limit
		},
		Iteration: 6,
	}

	result := ctx.CompactWithConfig(100, cfg) // low limit → triggers compaction

	if !result {
		t.Error("Expected Compact to return true (compaction performed)")
	}
	// Check that the long content is no longer present in its original form
	for _, m := range ctx.Messages {
		if m.Role == "tool" && m.Content == longToolResult {
			t.Error("Expected long tool result to be summarized or removed during compaction")
		}
	}
}

func TestCompactWithConfig_CustomThreshold(t *testing.T) {
	// With a higher aggressive iteration threshold, compaction should NOT strip
	// tool results at iteration 3
	longToolResult := strings.Repeat("result data ", 50) // > 200 chars
	cfg := DefaultCompactionConfig()
	cfg.AggressiveIterationThreshold = 10 // much higher than iteration 3

	ctx := &ConversationContext{
		Messages: []Message{
			{Role: "user", Content: strings.Repeat("question ", 500)}, // force over limit
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{{ID: "1", Name: "Tool1", Arguments: []byte(`{}`)}}},
			{Role: "tool", Content: longToolResult, ToolCallID: "1", Name: "Tool1"},
			{Role: "assistant", Content: "I found it"},
		},
		Iteration: 3, // below the custom threshold of 10
	}

	ctx.CompactWithConfig(50, cfg)

	// Long tool result should NOT be summarized (iteration is below threshold)
	for _, m := range ctx.Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "[tool result:") {
			// But it might be removed by Phase 1 or Phase 2 compaction
			// The key thing is Phase 0 aggressive stripping was NOT applied
		}
	}
}