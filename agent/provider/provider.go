package provider

import (
	"context"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// Provider is the interface for LLM providers.
// The pipeline dispatches on capabilities, not provider name.
type Provider interface {
	// Name returns the provider name (e.g., "openai", "anthropic", "ollama").
	Name() string

	// Capabilities returns what this provider supports.
	Capabilities() agentctx.ProviderCapabilities

	// Prepare builds the provider-specific request from the prepared request.
	// Returns an opaque request object that Execute accepts.
	Prepare(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error)

	// Execute sends the request to the LLM and returns the raw result.
	Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error)

	// Parse converts the provider-specific result into a canonical ParsedResponse.
	Parse(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error)
}

// ChatMessage is the format for LLM API messages.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall in provider format (mirrors OpenAI structure).
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // always "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
