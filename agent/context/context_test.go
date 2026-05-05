// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import (
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