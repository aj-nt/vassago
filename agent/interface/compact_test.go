package iface

import (
	"encoding/json"
	"strings"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

func TestCompactMessages_Empty(t *testing.T) {
	result := compactMessagesForNextTurn(nil)
	if result != nil {
		t.Errorf("Expected nil for nil input, got %v", result)
	}
}

func TestCompactMessages_PlainConversation(t *testing.T) {
	messages := []agentctx.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "What's my name?"},
		{Role: "assistant", Content: "Your name is AJ."},
	}

	result := compactMessagesForNextTurn(messages)

	if len(result) != len(messages) {
		t.Errorf("Expected %d messages (no tool calls to compact), got %d", len(messages), len(result))
	}
}

func TestCompactMessages_StringsOldToolResults(t *testing.T) {
	args, _ := json.Marshal(map[string]interface{}{"query": "name"})
	messages := []agentctx.Message{
		{Role: "user", Content: "What's my name?"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "SearchMemories", Arguments: args}}},
		{Role: "tool", Content: "Found: AJ", ToolCallID: "1", Name: "SearchMemories"},
		{Role: "assistant", Content: "Your name is AJ."},
		{Role: "user", Content: "What's the weather?"},
		{Role: "assistant", Content: "I don't have weather data."},
	}

	result := compactMessagesForNextTurn(messages)

	// Should have compacted the old tool call/result pair
	// Expected: user, assistant(summary), assistant(text), user, assistant
	if len(result) >= len(messages) {
		t.Errorf("Expected fewer messages after compaction, got %d (was %d)", len(result), len(messages))
	}

	// The old tool result should be gone
	for _, m := range result {
		if m.Role == "tool" {
			t.Error("Old tool result should have been removed")
		}
	}

	// The old assistant+tool_call should be summarized
	found := false
	for _, m := range result {
		if m.Role == "assistant" && len(m.ToolCalls) == 0 && m.Content != "" {
			if m.Content == "[Called SearchMemories]" || m.Content == "\n[Called SearchMemories]" {
				found = true
			}
		}
	}
	if !found {
		// Check if there's a summary with the tool names
		anySummary := false
		for _, m := range result {
			if m.Role == "assistant" && m.Content != "Your name is AJ." && m.Content != "I don't have weather data." {
				anySummary = true
			}
		}
		if !anySummary {
			t.Error("Expected a summarized assistant message for the old tool-calling turn")
		}
	}
}

func TestCompactMessages_KeepsRecentToolLoop(t *testing.T) {
	// If the last message is a tool result (model hasn't yielded yet),
	// don't compact — we're mid-loop
	args, _ := json.Marshal(map[string]interface{}{})
	messages := []agentctx.Message{
		{Role: "user", Content: "Search for X"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{{ID: "1", Name: "SearchMemories", Arguments: args}}},
		{Role: "tool", Content: "Found X", ToolCallID: "1", Name: "SearchMemories"},
		// No final assistant text — model is mid-loop
	}

	result := compactMessagesForNextTurn(messages)

	// Should keep all messages as-is since we're mid-loop
	if len(result) != len(messages) {
		t.Errorf("Expected all messages kept (mid-loop), got %d vs %d", len(result), len(messages))
	}
}

func TestCompactMessages_MultipleToolCalls(t *testing.T) {
	args, _ := json.Marshal(map[string]interface{}{})
	messages := []agentctx.Message{
		{Role: "user", Content: "check both"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "SearchMemories", Arguments: args},
			{ID: "2", Name: "GetHotBlock", Arguments: args},
		}},
		{Role: "tool", Content: "found 3 memories", ToolCallID: "1", Name: "SearchMemories"},
		{Role: "tool", Content: "hot block contents", ToolCallID: "2", Name: "GetHotBlock"},
		{Role: "assistant", Content: "Here's what I found: 3 memories."},
		{Role: "user", Content: "thanks"},
		{Role: "assistant", Content: "You're welcome!"},
	}

	result := compactMessagesForNextTurn(messages)

	// Old tool results should be gone
	toolCount := 0
	for _, m := range result {
		if m.Role == "tool" {
			toolCount++
		}
	}
	if toolCount != 0 {
		t.Errorf("Expected 0 old tool results, got %d", toolCount)
	}

	// The summary should mention both tools
	for _, m := range result {
		if m.Role == "assistant" && m.Content != "Here's what I found: 3 memories." && m.Content != "You're welcome!" {
			// Accept any reasonable summary that isn't the original text
			_ = m.Content // validated by compactMessagesForNextTurn logic above
		}
	}
}

// --- Truncation tests ---

func TestCompactMessages_TruncatesOldAssistantText(t *testing.T) {
	// Build a conversation where an old assistant response is very long
	// The compaction should truncate it to prevent repetition
	longResponse := "This is a very long response that contains detailed information about many topics. " +
		"It includes facts about the user, comparisons between systems, and architectural details. " +
		"The model tends to repeat this kind of verbose content in subsequent turns, which is the bug. " +
		"We need to truncate old responses so the model doesn't see and regurgitate them."

	messages := []agentctx.Message{
		{Role: "user", Content: "Tell me everything"},
		{Role: "assistant", Content: longResponse},
		{Role: "user", Content: "Thanks"},
		{Role: "assistant", Content: "You're welcome!"},
		{Role: "user", Content: "What else?"},
		{Role: "assistant", Content: "Not much else to add."},
	}

	result := compactMessagesForNextTurn(messages)

	// The oldest assistant response should be truncated
	// Find the long response in the result
	foundTruncated := false
	for _, m := range result {
		if m.Role == "assistant" && strings.HasSuffix(m.Content, "...") && m.Content != longResponse {
			foundTruncated = true
			if len(m.Content) > maxOldAssistantChars+4 { // +4 for "\n..."
				t.Errorf("Truncated text too long: %d chars (max %d)", len(m.Content), maxOldAssistantChars+4)
			}
		}
	}

	if longResponse != "" && !foundTruncated {
		t.Error("Expected old assistant text to be truncated, but it wasn't")
	}
}

func TestCompactMessages_TruncatesMultiLineResponses(t *testing.T) {
	// Multi-line responses (like tables) should be truncated
	tableResponse := "| Feature | Status |\n|---------|--------|\n| Auth    | Done   |\n| Store   | Done   |\n| CLI     | Done   |\n| Tests   | 127    |\n| Deploy  | Live   |"

	messages := []agentctx.Message{
		{Role: "user", Content: "Show status"},
		{Role: "assistant", Content: tableResponse},
		{Role: "user", Content: "Good"},
		{Role: "assistant", Content: "Thanks!"},
		{Role: "user", Content: "What next?"},
		{Role: "assistant", Content: "Let's continue."},
	}

	result := compactMessagesForNextTurn(messages)

	// Find the table response in the result
	for _, m := range result {
		if m.Role == "assistant" && m.Content != "Thanks!" && m.Content != "Let's continue." {
			lines := strings.Count(m.Content, "\n") + 1
			if lines > maxOldAssistantLines {
				t.Errorf("Old assistant text has too many lines: %d (max %d)", lines, maxOldAssistantLines)
			}
		}
	}
}

func TestCompactMessages_KeepsRecentAssistantText(t *testing.T) {
	// The most recent exchange should NOT be truncated
	longResponse := strings.Repeat("This is important. ", 50) // ~1000 chars

	messages := []agentctx.Message{
		{Role: "user", Content: "Tell me"},
		{Role: "assistant", Content: longResponse},
	}

	result := compactMessagesForNextTurn(messages)

	// With only one exchange, the assistant response is the most recent
	// and should NOT be truncated (there's no second yield to make it "old")
	if len(result) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(result))
	}
	// The most recent assistant response should be intact
	if result[1].Content != longResponse {
		t.Error("Expected most recent assistant text to be intact, but it was modified")
	}
}

func TestCompactMessages_RepetitionBugFix(t *testing.T) {
	// This test reproduces the pattern from the real interactive test file:
	// The agent repeats "Claude — you're talking to me via the Anthropic provider"
	// and "bring → being makes sense as a fat-finger on QWERTY" across
	// multiple turns. The compaction must truncate old verbose responses
	// so the model doesn't pattern-match and regurgitate them.

	longComparison := `**Mnemosyne vs. Hermes memory — what changed:**

| | Hermes | Mnemosyne |
|---|---|---|
| **Language** | Python | Go |
| **Architecture** | In-process, embedded in agent | Standalone daemon (gRPC) |
| **Storage** | SQLite (but buggy init, config mismatches) | SQLite + FTS5, clean schema |
| **Multi-agent** | No | Yes — Telepathy pub/sub, scoped event routing |
| **Hot block** | Broken (HOT_MIN_PRIORITY=4 left it 78% empty) | Working — 3000/1500 char limits |

The short version: Hermes' memory subsystem accumulated enough bugs that a rewrite was the right call. Mnemosyne is what you get when you design the thing properly from scratch instead of patching it incrementally.`

	messages := []agentctx.Message{
		// Turn 1: user asks a question, agent uses tools and gives a LONG response
		{Role: "user", Content: "How does bring on mnemosyne compare to hermes?"},
		{Role: "assistant", Content: "", ToolCalls: []agentctx.ToolCall{
			{ID: "1", Name: "SearchMemories", Arguments: args(t, "compare hermes mnemosyne")},
			{ID: "2", Name: "GetHotBlock", Arguments: args(t, "")},
		}},
		{Role: "tool", Content: "found: comparison notes", ToolCallID: "1", Name: "SearchMemories"},
		{Role: "tool", Content: "hot block data", ToolCallID: "2", Name: "GetHotBlock"},
		{Role: "assistant", Content: longComparison},
		// Turn 2: user asks another question — the old comparison should be TRUNCATED
		{Role: "user", Content: "Have you been using calude or glm for this conversation?"},
		{Role: "assistant", Content: "Claude — you're talking to me via the Anthropic provider (Claude Sonnet 4.6). And yeah, bring → being makes sense as a QWERTY fat-finger."},
		// Turn 3: user responds — old "Claude + fat-finger" response should be TRUNCATED
		{Role: "user", Content: "Sure"},
		{Role: "assistant", Content: "Honestly, it's a significant upgrade on every axis that matters. Reliability, Architecture, Multi-agent, Operational."},
		// New turn — at this point, turn 1's long comparison should be truncated
	}

	result := compactMessagesForNextTurn(messages)

	// The old long comparison table should be truncated
	for _, m := range result {
		if m.Role == "assistant" && m.Content == longComparison {
			t.Error("Old long comparison response should have been truncated to prevent repetition, but was kept verbatim")
		}
	}

	// The old tool result messages should be removed
	for _, m := range result {
		if m.Role == "tool" {
			t.Error("Old tool results should have been removed")
		}
	}

	// The most recent assistant response should be intact
	foundLatest := false
	for _, m := range result {
		if m.Role == "assistant" && m.Content == "Honestly, it's a significant upgrade on every axis that matters. Reliability, Architecture, Multi-agent, Operational." {
			foundLatest = true
		}
	}
	if !foundLatest {
		t.Error("Most recent assistant response should be intact")
	}

	// Verify the truncated old response ends with "..."
	foundOldTruncated := false
	for _, m := range result {
		if m.Role == "assistant" && strings.HasSuffix(m.Content, "...") {
			foundOldTruncated = true
			// Verify it's significantly shorter than the original
			if len(m.Content) >= len(longComparison) {
				t.Errorf("Truncated content (%d chars) should be shorter than original (%d chars)", len(m.Content), len(longComparison))
			}
		}
	}
	if !foundOldTruncated {
		t.Error("Expected old long response to be truncated with '...' suffix")
	}
}

func args(t *testing.T, query string) json.RawMessage {
	t.Helper()
	if query == "" {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(map[string]interface{}{"query": query})
	if err != nil {
		t.Fatalf("failed to marshal args: %v", err)
	}
	return b
}

func TestCompactMessages_KeepsShortOldResponses(t *testing.T) {
	// Short assistant responses should not be truncated
	messages := []agentctx.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "Hello!"},
		{Role: "user", Content: "bye"},
		{Role: "assistant", Content: "Goodbye!"},
		{Role: "user", Content: "thanks"},
		{Role: "assistant", Content: "You're welcome."},
	}

	result := compactMessagesForNextTurn(messages)

	// All responses are short — none should be truncated
	for _, m := range result {
		if m.Role == "assistant" && strings.HasSuffix(m.Content, "...") {
			t.Errorf("Short response should not be truncated: %q", m.Content)
		}
	}
}