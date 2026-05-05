// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import (
	"fmt"
	"strings"
	"testing"
)

func TestConversationContext_AddMessage(t *testing.T) {
	ctx := &ConversationContext{}

	ctx.AddMessage(Message{Role: "user", Content: "hello"})
	ctx.AddMessage(Message{Role: "assistant", Content: "hi there"})

	if len(ctx.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(ctx.Messages))
	}
	if ctx.Messages[0].Role != "user" {
		t.Errorf("Expected first message role 'user', got %q", ctx.Messages[0].Role)
	}
	if ctx.Messages[1].Content != "hi there" {
		t.Errorf("Expected second message content 'hi there', got %q", ctx.Messages[1].Content)
	}
}

func TestConversationContext_LastAssistantMessage(t *testing.T) {
	ctx := &ConversationContext{
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "first"},
			{Role: "user", Content: "again"},
			{Role: "assistant", Content: "second"},
		},
	}

	last := ctx.LastAssistantMessage()
	if last == nil {
		t.Fatal("Expected to find last assistant message")
	}
	if last.Content != "second" {
		t.Errorf("Expected 'second', got %q", last.Content)
	}
}

func TestConversationContext_LastAssistantMessage_None(t *testing.T) {
	ctx := &ConversationContext{
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	}

	last := ctx.LastAssistantMessage()
	if last != nil {
		t.Errorf("Expected nil when no assistant messages, got %v", last)
	}
}

func TestConversationContext_LastAssistantMessage_Empty(t *testing.T) {
	ctx := &ConversationContext{}

	last := ctx.LastAssistantMessage()
	if last != nil {
		t.Errorf("Expected nil for empty context, got %v", last)
	}
}

func TestConversationContext_TokenCount(t *testing.T) {
	ctx := &ConversationContext{
		SystemPrompt: "You are helpful.",
		Messages: []Message{
			{Role: "user", Content: "Hello, how are you?"},
			{Role: "assistant", Content: "I'm doing great!"},
		},
	}

	tokens := ctx.TokenCount()
	if tokens <= 0 {
		t.Errorf("Expected positive token count, got %d", tokens)
	}

	// Rough check: system prompt (16 chars) + messages (~36 chars) = ~52 chars / 4 = ~13 tokens
	if tokens < 5 || tokens > 50 {
		t.Errorf("Token count seems unreasonable: %d", tokens)
	}
}

func TestConversationContext_TokenCount_WithToolCalls(t *testing.T) {
	ctx := &ConversationContext{
		Messages: []Message{
			{
				Role:    "assistant",
				Content: "Let me search",
				ToolCalls: []ToolCall{
					{ID: "call1", Name: "SearchMemories", Arguments: []byte(`{"query":"test"}`)},
				},
			},
		},
	}

	tokens := ctx.TokenCount()
	if tokens <= 0 {
		t.Errorf("Expected positive token count with tool calls, got %d", tokens)
	}
}

func TestConversationContext_IsOverTokenLimit(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		maxTokens int
		want      bool
	}{
		{name: "under limit", content: "hello", maxTokens: 10000, want: false},
		{name: "over limit", content: strings.Repeat("word ", 10000), maxTokens: 100, want: true},
		{name: "zero max means no limit", content: strings.Repeat("word ", 10000), maxTokens: 0, want: false},
		{name: "negative max means no limit", content: strings.Repeat("word ", 10000), maxTokens: -1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ConversationContext{
				Messages: []Message{{Role: "user", Content: tt.content}},
			}
			if got := ctx.IsOverTokenLimit(tt.maxTokens); got != tt.want {
				t.Errorf("IsOverTokenLimit(%d) = %v, want %v", tt.maxTokens, got, tt.want)
			}
		})
	}
}

func TestConversationContext_AppendToolResults(t *testing.T) {
	ctx := &ConversationContext{
		Messages: []Message{{Role: "user", Content: "hello"}},
	}

	results := []ToolResult{
		{CallID: "1", Name: "SearchMemories", Result: "found 3 memories", IsError: false},
		{CallID: "2", Name: "BadTool", Result: "something failed", IsError: true},
	}

	ctx.AppendToolResults(results)

	if len(ctx.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(ctx.Messages))
	}
	// First result: normal truncation
	if ctx.Messages[1].Role != "tool" || ctx.Messages[1].ToolCallID != "1" {
		t.Errorf("Expected tool result with CallID 1, got %+v", ctx.Messages[1])
	}
	// Second result: error prefix
	if ctx.Messages[2].Content != "Error: something failed" {
		t.Errorf("Expected error result to be prefixed with 'Error:', got %q", ctx.Messages[2].Content)
	}
}

func TestConversationContext_AppendToolResults_Truncation(t *testing.T) {
	ctx := &ConversationContext{
		Messages: []Message{{Role: "user", Content: "hello"}},
	}

	// Create a long tool result that triggers truncation
	longResult := strings.Repeat("line of data\n", 200)
	results := []ToolResult{
		{CallID: "1", Name: "BigTool", Result: longResult, IsError: false},
	}

	ctx.AppendToolResults(results)

	// The content should be truncated, not the original
	if ctx.Messages[1].Content == longResult {
		t.Error("Expected long result to be truncated, but got original")
	}
	if !strings.Contains(ctx.Messages[1].Content, "truncated") {
		t.Errorf("Expected truncated result to contain 'truncated', got %q", ctx.Messages[1].Content[:100])
	}
}

func TestConversationContext_SummarizeLargeToolResults(t *testing.T) {
	shortContent := "short result"
	longContent := strings.Repeat("x", 1000)

	ctx := &ConversationContext{
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "tool", Content: shortContent, Name: "ShortTool"},
			{Role: "tool", Content: longContent, Name: "LongTool"},
		},
	}

	ctx.SummarizeLargeToolResults(500)

	// Short result should be unchanged
	if ctx.Messages[1].Content != shortContent {
		t.Errorf("Expected short result unchanged, got %q", ctx.Messages[1].Content)
	}
	// Long result should be summarized
	if !strings.Contains(ctx.Messages[2].Content, "[tool result:") {
		t.Errorf("Expected long result to be summarized, got %q", ctx.Messages[2].Content)
	}
	if !strings.Contains(ctx.Messages[2].Content, "LongTool") {
		t.Errorf("Expected summary to include tool name, got %q", ctx.Messages[2].Content)
	}
}
func TestConversationContext_Compact_Phase0(t *testing.T) {
	// High iteration (5+) should aggressively strip tool results > 200 chars
	longToolResult := strings.Repeat("result data ", 50) // > 200 chars

	ctx := &ConversationContext{
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{{ID: "1", Name: "Tool1", Arguments: []byte(`{}`)}}},
			{Role: "tool", Content: longToolResult, ToolCallID: "1", Name: "Tool1"},
			{Role: "assistant", Content: "I found it"},
			{Role: "user", Content: strings.Repeat("more data ", 5000)}, // to exceed limit
		},
		Iteration: 6, // high iteration
	}

	result := ctx.Compact(100) // very low limit → triggers compaction

	if !result {
		t.Error("Expected Compact to return true (compaction performed)")
	}
	// In Phase 0 (high iteration), tool results > 200 chars are summarized
	// Then Phase 1 further compacts by removing old tool results before the last yield
	// After Phase 0, the tool result content should have been replaced with a summary
	// Check that the long content is no longer present in its original form
	for _, m := range ctx.Messages {
		if m.Role == "tool" && m.Content == longToolResult {
			t.Error("Expected long tool result to be summarized or removed during compaction")
		}
	}
}

func TestConversationContext_Compact_Phase1_StripsOldToolResults(t *testing.T) {
	// Build a context with old tool calls and enough content to exceed a low token limit
	longContent := strings.Repeat("data ", 200) // ~1000 chars = ~250 tokens
	ctx := &ConversationContext{
		Messages: []Message{
			{Role: "user", Content: "What's my name?"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{{ID: "1", Name: "SearchMemories", Arguments: []byte(`{}`)}}},
			{Role: "tool", Content: longContent, ToolCallID: "1", Name: "SearchMemories"},
			{Role: "assistant", Content: "Your name is AJ."},
			{Role: "user", Content: "What's the weather?"},
			{Role: "assistant", Content: "I don't have weather data."},
		},
		Iteration: 0,
	}

	// Trigger compaction with a low token limit (the long tool result should push us over)
	ctx.Compact(50) // very low limit → the long content forces compaction

	// The old tool result should be removed (it's before the yield point)
	toolCount := 0
	for _, m := range ctx.Messages {
		if m.Role == "tool" {
			toolCount++
		}
	}
	if toolCount != 0 {
		t.Errorf("Expected old tool results to be removed, found %d", toolCount)
	}

	// The old assistant+tool_call should be summarized
	found := false
	for _, m := range ctx.Messages {
		if m.Role == "assistant" && m.Content == "[Called SearchMemories]" {
			found = true
		}
	}
	if !found {
		t.Error("Expected old assistant+tool_call to be summarized as '[Called SearchMemories]'")
	}
}

func TestConversationContext_Compact_NotOverLimit(t *testing.T) {
	ctx := &ConversationContext{
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	}

	result := ctx.Compact(10000)
	if result {
		t.Error("Expected Compact to return false when not over limit")
	}
}

func TestTruncateToolResult_ShortResult(t *testing.T) {
	shortResult := "This is a short result\nwith only a few lines\nnot enough to trigger truncation"

	result := TruncateToolResult(shortResult, false)
	if result != shortResult {
		t.Errorf("Expected short result to remain unchanged, got:\n%s", result)
	}
}

func TestTruncateToolResult_LongResult(t *testing.T) {
	// Build a long result with more than 100 lines
	var lines []string
	for i := 0; i < 250; i++ {
		lines = append(lines, fmt.Sprintf("Line %d: This is a line of output", i+1))
	}
	longResult := strings.Join(lines, "\n")

	result := TruncateToolResult(longResult, false)

	// Result should have head + tail format
	resultLines := strings.Split(result, "\n")

	// Check that the result contains the summary line
	hasSummary := false
	for _, line := range resultLines {
		if strings.HasPrefix(line, "... (truncated:") {
			hasSummary = true
		}
	}
	if !hasSummary {
		t.Errorf("Expected result to contain summary line with truncation info")
	}

	// Check that the first headCount lines are preserved
	headCount := MaxToolResultLines / 2
	for i := 0; i < headCount; i++ {
		if resultLines[i] != lines[i] {
			t.Errorf("Line %d mismatch:\nExpected: %s\nGot: %s", i, lines[i], resultLines[i])
		}
	}
}

func TestTruncateToolResult_ErrorResult(t *testing.T) {
	errorResult := "This is an error message\nthat should not be truncated"

	result := TruncateToolResult(errorResult, true)
	if result != errorResult {
		t.Errorf("Expected error result to remain unchanged, got:\n%s", result)
	}
}

func TestIsResultNotFound(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"File not found", true},
		{"no such file or directory", true},
		{"Path does not exist", true},
		{"no results matched", true},
		{"No matches found", true},
		{"0 files found", true},
		{"Empty result set", true},
		{"Success: data returned", false},
		{"Here are your results", false},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			if got := IsResultNotFound(tt.content); got != tt.want {
				t.Errorf("IsResultNotFound(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestStageAction_String(t *testing.T) {
	tests := []struct {
		action   StageAction
		expected string
	}{
		{StageActionCONTINUE, "CONTINUE"},
		{StageActionYIELD, "YIELD"},
		{StageActionTERMINATE, "TERMINATE"},
		{StageAction(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.action.String(); got != tt.expected {
			t.Errorf("StageAction(%d).String() = %q, want %q", tt.action, got, tt.expected)
		}
	}
}

func TestInterruptSignal_Set(t *testing.T) {
	sig := NewInterruptSignal()
	if sig.IsSet() {
		t.Error("New signal should not be set")
	}

	sig.Set()
	if !sig.IsSet() {
		t.Error("Signal should be set after Set()")
	}

	// Channel should be closed
	<-sig.Channel()
}

func TestInterruptSignal_MultipleSet(t *testing.T) {
	sig := NewInterruptSignal()
	// Should not panic on multiple Set calls
	sig.Set()
	sig.Set()
	sig.Set()

	if !sig.IsSet() {
		t.Error("Signal should still be set")
	}
}

func TestProviderCapabilities(t *testing.T) {
	cap := ProviderCapabilities{
		SupportsStreaming:       true,
		SupportsFunctionCalling: true,
		SupportsPromptCaching:  false,
		SupportsVision:          false,
		MaxContextTokens:        200000,
	}

	if !cap.SupportsStreaming {
		t.Error("Expected SupportsStreaming=true")
	}
	if cap.MaxContextTokens != 200000 {
		t.Errorf("Expected MaxContextTokens=200000, got %d", cap.MaxContextTokens)
	}
}

func TestMessageJSON(t *testing.T) {
	msg := Message{
		Role:    "assistant",
		Content: "Hello!",
		ToolCalls: []ToolCall{
			{ID: "1", Name: "Test", Arguments: []byte(`{"key":"val"}`)},
		},
	}

	// Verify JSON marshaling works
	data, err := msg.ToolCalls[0].Arguments.MarshalJSON()
	if err != nil {
		t.Errorf("Failed to marshal tool call arguments: %v", err)
	}
	if string(data) != `{"key":"val"}` {
		t.Errorf("Unexpected marshaled arguments: %s", data)
	}
}

func TestEffectiveMaxTokens(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
		want      int
	}{
		{name: "zero returns zero", maxTokens: 0, want: 0},
		{name: "negative returns negative", maxTokens: -1, want: -1},
		{name: "128K returns 115200", maxTokens: 128000, want: 115200},
		{name: "200K returns 180000", maxTokens: 200000, want: 180000},
		{name: "small value", maxTokens: 100, want: 90},
		{name: "tiny value rounds down", maxTokens: 10, want: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveMaxTokens(tt.maxTokens)
			if got != tt.want {
				t.Errorf("EffectiveMaxTokens(%d) = %d, want %d", tt.maxTokens, got, tt.want)
			}
		})
	}
}