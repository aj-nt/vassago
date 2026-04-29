package pipeline

import (
	"strings"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

func TestNewRequestPrepStage(t *testing.T) {
	stage := NewRequestPrepStage("direct")
	if stage == nil {
		t.Error("NewRequestPrepStage should return a non-nil stage")
	}
	if stage.Personality != "direct" {
		t.Errorf("Expected Personality 'direct', got %q", stage.Personality)
	}
}

func TestRequestPrepStageProcess_EmptyContext(t *testing.T) {
	stage := NewRequestPrepStage("sarcastic")
	convCtx := &agentctx.ConversationContext{
		Messages:        []agentctx.Message{},
		HotBlockContent: "",
		Temperature:     0.7,
		Tools:           []agentctx.ToolDefinition{},
	}

	prepared := stage.Process(convCtx)

	if prepared.SystemPrompt == "" {
		t.Error("Expected non-empty system prompt even with no hot block")
	}
	if !strings.Contains(prepared.SystemPrompt, "sarcastic") {
		t.Errorf("Expected system prompt to contain personality, got: %s", prepared.SystemPrompt)
	}
	// Temperature is in APIKwargs, not a direct field
	if temp, ok := prepared.APIKwargs["temperature"]; !ok || temp != 0.7 {
		t.Errorf("Expected temperature 0.7 in APIKwargs, got %v", temp)
	}
}

func TestRequestPrepStageProcess_WithHotBlock(t *testing.T) {
	stage := NewRequestPrepStage("direct")
	hotBlock := "[user] name_paul: Paul Flinton. Goes by AJ."
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{Role: "user", Content: "What's my name?"},
		},
		HotBlockContent: hotBlock,
		Temperature:     0.5,
		Tools: []agentctx.ToolDefinition{
			{Name: "SearchMemories", Description: "Search memories"},
		},
	}

	prepared := stage.Process(convCtx)

	if !strings.Contains(prepared.SystemPrompt, "Memory") {
		t.Error("Expected system prompt to contain memory section header")
	}
	if !strings.Contains(prepared.SystemPrompt, hotBlock) {
		t.Error("Expected system prompt to contain hot block content")
	}
	if !strings.Contains(prepared.SystemPrompt, "SearchMemories") {
		t.Error("Expected system prompt to mention SearchMemories tool")
	}
	if len(prepared.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(prepared.Messages))
	}
}

func TestRequestPrepStageProcess_WithPersonalityFile(t *testing.T) {
	// Multi-line personality should be used as-is (not wrapped with "You are X.")
	stage := NewRequestPrepStage("You are bartimaeus.\nBe direct and witty.")
	convCtx := &agentctx.ConversationContext{
		Messages:        []agentctx.Message{},
		HotBlockContent: "",
		Temperature:     0.7,
		Tools:           nil,
	}

	prepared := stage.Process(convCtx)

	if !strings.Contains(prepared.SystemPrompt, "bartimaeus") {
		t.Errorf("Expected system prompt to contain personality, got: %s", prepared.SystemPrompt)
	}
	if strings.Contains(prepared.SystemPrompt, "You are You are") {
		t.Error("Multi-line personality should not be double-wrapped")
	}
}

func TestRequestPrepStageProcess_NoHotBlock(t *testing.T) {
	stage := NewRequestPrepStage("direct")
	convCtx := &agentctx.ConversationContext{
		Messages:        []agentctx.Message{{Role: "user", Content: "hello"}},
		HotBlockContent: "",
		Temperature:     0.7,
		Tools:           nil,
	}

	prepared := stage.Process(convCtx)

	// Should have personality but no memory section
	if !strings.Contains(prepared.SystemPrompt, "direct") {
		t.Error("Expected personality in system prompt")
	}
	if strings.Contains(prepared.SystemPrompt, "[Memory") {
		t.Error("Should not have memory section when hot block is empty")
	}
}

func TestRequestPrepStageProcess_AntiRepetitionDirective(t *testing.T) {
	stage := NewRequestPrepStage("direct")
	convCtx := &agentctx.ConversationContext{
		Messages:        []agentctx.Message{{Role: "user", Content: "hello"}},
		HotBlockContent: "",
		Temperature:     0.7,
		Tools:           nil,
	}

	prepared := stage.Process(convCtx)

	if !strings.Contains(prepared.SystemPrompt, "repeat") {
		t.Error("Expected anti-repetition directive in system prompt")
	}
	if !strings.Contains(prepared.SystemPrompt, "Do NOT repeat") {
		t.Error("Expected explicit anti-repetition instruction")
	}
}