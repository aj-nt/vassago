// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import (
	"testing"

	"encoding/json"
)

func TestDefaultTokenCounter(t *testing.T) {
	text := "Hello, world! This is a test of token counting. (48 chars)"
	tokens := DefaultTokenCounter.CountTokens(text)
	expected := len(text) / 4 // 48/4 = 12
	if tokens != expected {
		t.Errorf("DefaultTokenCounter = %d, want %d (chars/4)", tokens, expected)
	}
}

func TestCounterForModel_OpenAI(t *testing.T) {
	// GPT models should use tiktoken cl100k_base
	c := CounterForModel("gpt-4o")
	if _, ok := c.(*TiktokenCounter); !ok {
		t.Errorf("expected TiktokenCounter for gpt-4o, got %T", c)
	}

	// o-series should use o200k_base
	c = CounterForModel("o1")
	if _, ok := c.(*TiktokenCounter); !ok {
		t.Errorf("expected TiktokenCounter for o1, got %T", c)
	}
}

func TestCounterForModel_Claude(t *testing.T) {
	c := CounterForModel("claude-sonnet-4-6")
	rc, ok := c.(*RatioCounter)
	if !ok {
		t.Fatalf("expected RatioCounter for claude, got %T", c)
	}
	if rc.Ratio != 3.5 {
		t.Errorf("claude ratio = %f, want 3.5", rc.Ratio)
	}
}

func TestCounterForModel_DeepSeek(t *testing.T) {
	c := CounterForModel("deepseek-v4-pro")
	rc, ok := c.(*RatioCounter)
	if !ok {
		t.Fatalf("expected RatioCounter for deepseek, got %T", c)
	}
	if rc.Ratio != 3.5 {
		t.Errorf("deepseek ratio = %f, want 3.5", rc.Ratio)
	}
}

func TestCounterForModel_GLM(t *testing.T) {
	c := CounterForModel("glm-5.1")
	rc, ok := c.(*RatioCounter)
	if !ok {
		t.Fatalf("expected RatioCounter for glm, got %T", c)
	}
	if rc.Ratio != 4.0 {
		t.Errorf("glm ratio = %f, want 4.0", rc.Ratio)
	}
}

func TestCounterForModel_Unknown(t *testing.T) {
	c := CounterForModel("some-unknown-model")
	if c != DefaultTokenCounter {
		t.Errorf("expected DefaultTokenCounter for unknown model, got %T", c)
	}
}

func TestTiktokenCounter_Accuracy(t *testing.T) {
	tc := NewTiktokenCounter("cl100k_base")
	if tc == nil {
		t.Fatal("NewTiktokenCounter returned nil")
	}

	// English text: chars/4 should be close
	eng := "Hello, world! This is a short test sentence."
	engTokens := tc.CountTokens(eng)
	engEstimated := len(eng) / 4
	// Tokens should be within 50% of chars/4 estimate
	if engTokens < engEstimated/2 || engTokens > engEstimated*2 {
		t.Errorf("tiktoken English: chars=%d tokens=%d estimated=%d (too far off)", len(eng), engTokens, engEstimated)
	}
	t.Logf("English: %d chars → %d tokens (chars/4 estimate: %d, ratio: %.1f)", len(eng), engTokens, engEstimated, float64(len(eng))/float64(engTokens))

	// Code: Go code tends to have more tokens per char
	code := `func (c *ConversationContext) TokenCount() int {
	counter := c.getTokenCounter()
	return counter.CountTokens(c.SystemPrompt)
}`
	codeTokens := tc.CountTokens(code)
	codeEstimated := len(code) / 4
	t.Logf("Go code: %d chars → %d tokens (chars/4 estimate: %d, ratio: %.1f)", len(code), codeTokens, codeEstimated, float64(len(code))/float64(codeTokens))
	// Code should be within reasonable bounds
	if codeTokens < 1 {
		t.Error("tiktoken returned 0 tokens for Go code")
	}
}

func TestCountMessageTokens(t *testing.T) {
	counter := DefaultTokenCounter
	msg := Message{
		Role:    "user",
		Content: "Hello, this is a test message with 48 characters.",
	}
	tokens := CountMessageTokens(counter, msg)
	// Content tokens (48/4=12) + role overhead (4) = 16
	if tokens < 12 || tokens > 20 {
		t.Errorf("CountMessageTokens = %d, want ~16", tokens)
	}

	// Message with tool calls
	msgWithTools := Message{
		Role:    "assistant",
		Content: "Let me search for that.",
		ToolCalls: []ToolCall{
			{Name: "SearchMemories", Arguments: json.RawMessage(`{"query":"test"}`)},
		},
	}
	tokensWithTools := CountMessageTokens(counter, msgWithTools)
	if tokensWithTools <= tokens {
		t.Errorf("message with tool calls (%d) should have more tokens than plain message (%d)", tokensWithTools, tokens)
	}
}

func TestConversationContext_TokenCountWithCounter(t *testing.T) {
	ctx := &ConversationContext{
		SystemPrompt: "You are a helpful assistant.",
		Messages: []Message{
			{Role: "user", Content: "Hello, how are you?"},
			{Role: "assistant", Content: "I'm doing well, thank you!"},
		},
	}

	// Default counter (chars/4)
	defTokens := ctx.TokenCount()
	t.Logf("Default token count: %d", defTokens)

	// Switch to ratio 3.5 (should increase token count)
	ctx.SetTokenCounter(NewRatioCounter(3.5))
	ratioTokens := ctx.TokenCount()
	t.Logf("Ratio 3.5 token count: %d", ratioTokens)

	// More aggressive ratio should produce more tokens
	if ratioTokens <= defTokens {
		t.Errorf("ratio 3.5 (%d) should produce more tokens than chars/4 (%d)", ratioTokens, defTokens)
	}

	// Reset to nil → should use default
	ctx.SetTokenCounter(nil)
	nilTokens := ctx.TokenCount()
	if nilTokens != defTokens {
		t.Errorf("nil counter produced %d tokens, want default %d", nilTokens, defTokens)
	}
}

func TestCharCount(t *testing.T) {
	ctx := &ConversationContext{
		SystemPrompt: "You are a bot.", // 14 chars
		Messages: []Message{
			{Role: "user", Content: "Hi"},      // 2 chars
			{Role: "assistant", Content: "Hey"}, // 3 chars
		},
	}
	chars := ctx.CharCount()
	expected := 14 + 2 + 3 // 19
	if chars != expected {
		t.Errorf("CharCount = %d, want %d", chars, expected)
	}
}

func TestTokenCounter_Consistency(t *testing.T) {
	// Same text should produce same result across calls
	tc := NewTiktokenCounter("cl100k_base")
	text := "Consistency check: this text is tokenized twice."
	t1 := tc.CountTokens(text)
	t2 := tc.CountTokens(text)
	if t1 != t2 {
		t.Errorf("tiktoken inconsistent: %d != %d", t1, t2)
	}
}
