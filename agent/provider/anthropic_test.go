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
