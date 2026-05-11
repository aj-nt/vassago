// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package provider

import (
	"context"
	"encoding/json"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

func TestAnthropicParse(t *testing.T) {
	provider := NewAnthropicProvider("test-key", "claude-test")

	// Also test basic accessor methods
	t.Run("Name", func(t *testing.T) {
		if provider.Name() != "anthropic" {
			t.Errorf("Expected Name() = 'anthropic', got '%s'", provider.Name())
		}
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := provider.Capabilities()
		if !caps.SupportsStreaming {
			t.Error("Expected SupportsStreaming = true")
		}
		if !caps.SupportsFunctionCalling {
			t.Error("Expected SupportsFunctionCalling = true")
		}
		if !caps.SupportsPromptCaching {
			t.Error("Expected SupportsPromptCaching = true")
		}
	})

	t.Run("CacheControlHeaders", func(t *testing.T) {
		// Test that cache_control headers are set correctly when CacheBreakpoint > 0
		ctx := &agentctx.ConversationContext{
			SystemPrompt:    "Test system prompt",
			Messages:        []agentctx.Message{{Role: "user", Content: "Hello"}},
			CacheBreakpoint: 5, // > 0 should enable caching
		}
		
		prepared := agentctx.PreparedRequest{
			SystemPrompt:    "Test system prompt",
			Messages:        ctx.Messages,
			CacheBreakpoint: ctx.CacheBreakpoint,
		}
		
		req, err := provider.Prepare(context.Background(), ctx, prepared)
		if err != nil {
			t.Fatalf("Failed to prepare request: %v", err)
		}
		
		body, ok := req.(map[string]interface{})
		if !ok {
			t.Fatal("Expected request body to be map[string]interface{}")
		}
		
		system, ok := body["system"].([]map[string]interface{})
		if !ok {
			t.Fatal("Expected system to be []map[string]interface{}")
		}
		
		if len(system) == 0 {
			t.Fatal("Expected system to have at least one content block")
		}
		
		cacheControl, ok := system[0]["cache_control"].(map[string]string)
		if !ok {
			t.Fatal("Expected cache_control to be present")
		}
		
		if cacheControl["type"] != "ephemeral" {
			t.Errorf("Expected cache_control type 'ephemeral', got '%s'", cacheControl["type"])
		}
	})

	t.Run("CacheDisabledWhenBreakpointZero", func(t *testing.T) {
		// Test that caching is disabled when CacheBreakpoint = 0
		ctx := &agentctx.ConversationContext{
			SystemPrompt:    "Test system prompt",
			Messages:        []agentctx.Message{{Role: "user", Content: "Hello"}},
			CacheBreakpoint: 0, // Should disable caching
		}
		
		prepared := agentctx.PreparedRequest{
			SystemPrompt:    "Test system prompt",
			Messages:        ctx.Messages,
			CacheBreakpoint: ctx.CacheBreakpoint,
		}
		
		req, err := provider.Prepare(context.Background(), ctx, prepared)
		if err != nil {
			t.Fatalf("Failed to prepare request: %v", err)
		}
		
		body, ok := req.(map[string]interface{})
		if !ok {
			t.Fatal("Expected request body to be map[string]interface{}")
		}
		
		// When caching is disabled, system should be a plain string, not content blocks
		system, ok := body["system"].(string)
		if !ok {
			t.Fatal("Expected system to be a plain string when caching is disabled")
		}
		
		if system != "Test system prompt" {
			t.Errorf("Expected system prompt 'Test system prompt', got '%s'", system)
		}
	})

	// Test response with text content
	textResponse := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": "Hello, world!",
			},
		},
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 20,
		},
	}

	textJSON, _ := json.Marshal(textResponse)
	result := agentctx.ProviderResult{RawResponse: textJSON}
	parsed, err := provider.Parse(context.Background(), result)
	if err != nil {
		t.Fatalf("Failed to parse text response: %v", err)
	}

	if parsed.Text != "Hello, world!" {
		t.Errorf("Expected text 'Hello, world!', got '%s'", parsed.Text)
	}

	if parsed.FinishReason != "end_turn" {
		t.Errorf("Expected finish reason 'end_turn', got '%s'", parsed.FinishReason)
	}

	if parsed.Usage == nil || parsed.Usage.TotalTokens != 30 {
		t.Errorf("Expected usage total 30, got %v", parsed.Usage)
	}

	// Test response with tool calls
	toolResponse := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": "Let me help with that.",
			},
			{
				"type": "tool_use",
				"id":   "tool_123",
				"name": "FileRead",
				"input": map[string]interface{}{"path": "/test/file.txt"},
			},
		},
		"stop_reason": "tool_use",
	}

	toolJSON, _ := json.Marshal(toolResponse)
	result = agentctx.ProviderResult{RawResponse: toolJSON}
	parsed, err = provider.Parse(context.Background(), result)
	if err != nil {
		t.Fatalf("Failed to parse tool response: %v", err)
	}

	if parsed.Text != "Let me help with that." {
		t.Errorf("Expected text 'Let me help with that.', got '%s'", parsed.Text)
	}

	if len(parsed.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(parsed.ToolCalls))
	}

	if parsed.ToolCalls[0].Name != "FileRead" {
		t.Errorf("Expected tool name 'FileRead', got '%s'", parsed.ToolCalls[0].Name)
	}
}
