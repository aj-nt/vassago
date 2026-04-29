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

	// Stream sends the request and returns a channel of streaming events.
	// Providers that don't support streaming should return an error.
	// The channel is closed when the stream is done.
	Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error)
}