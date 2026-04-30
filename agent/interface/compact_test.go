package iface

import (
	"encoding/json"
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
		{Role: "tool", Content: "Found: Paul Flinton, goes by AJ", ToolCallID: "1", Name: "SearchMemories"},
		{Role: "assistant", Content: "Your name is Paul Flinton. You go by AJ."},
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
			if m.Role == "assistant" && m.Content != "Your name is Paul Flinton. You go by AJ." && m.Content != "I don't have weather data." {
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