package provider

import (
	"context"
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// CannedResponse is a pre-configured response for the mock provider.
type CannedResponse struct {
	Response agentctx.ParsedResponse
	Error    error
}

// MockProvider is a test provider that returns canned responses.
// It enables testing the full agent loop without real LLM calls.
type MockProvider struct {
	name          string
	capabilities  agentctx.ProviderCapabilities
	responses     []CannedResponse
	callCount     int
	defaultResp   agentctx.ParsedResponse
}

// NewMockProvider creates a mock provider with the given name and responses.
func NewMockProvider(name string, responses []CannedResponse) *MockProvider {
	return &MockProvider{
		name: name,
		capabilities: agentctx.ProviderCapabilities{
			SupportsStreaming:       false,
			SupportsFunctionCalling: true,
			SupportsPromptCaching:  false,
			SupportsVision:         false,
			MaxContextTokens:       128000,
		},
		responses:   responses,
		defaultResp: agentctx.ParsedResponse{Text: "done", FinishReason: "stop"},
	}
}

// Name returns the mock provider name.
func (m *MockProvider) Name() string { return m.name }

// Capabilities returns the mock capabilities.
func (m *MockProvider) Capabilities() agentctx.ProviderCapabilities { return m.capabilities }

// SetCapabilities allows tests to customize capabilities.
func (m *MockProvider) SetCapabilities(c agentctx.ProviderCapabilities) {
	m.capabilities = c
}

// SetDefaultResponse sets the default response for calls beyond the canned responses.
func (m *MockProvider) SetDefaultResponse(resp agentctx.ParsedResponse) {
	m.defaultResp = resp
}


// CallCount returns how many times Execute was called.
func (m *MockProvider) CallCount() int { return m.callCount }

// Prepare returns the prepared request as-is (no transformation needed for mock).
func (m *MockProvider) Prepare(_ context.Context, _ *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	return prepared, nil
}

// Execute returns the next canned response.
func (m *MockProvider) Execute(_ context.Context, _ interface{}) (agentctx.ProviderResult, error) {
	m.callCount++
	if m.callCount <= len(m.responses) {
		canned := m.responses[m.callCount-1]
		if canned.Error != nil {
			return agentctx.ProviderResult{Error: canned.Error}, nil
		}
		return agentctx.ProviderResult{
			RawResponse: canned.Response,
		}, nil
	}
	// Default: return a simple text response (end the loop)
	return agentctx.ProviderResult{
		RawResponse: m.defaultResp,
	}, nil
}

// Parse converts the raw response to ParsedResponse.
func (m *MockProvider) Parse(_ context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	if result.Error != nil {
		return agentctx.ParsedResponse{}, result.Error
	}
	resp, ok := result.RawResponse.(agentctx.ParsedResponse)
	if !ok {
		return agentctx.ParsedResponse{}, fmt.Errorf("mock: unexpected raw response type %T", result.RawResponse)
	}
	return resp, nil
}

// Stream is not supported by the mock provider — returns an error.
func (m *MockProvider) Stream(_ context.Context, _ interface{}) (<-chan agentctx.StreamEvent, error) {
	return nil, fmt.Errorf("mock provider does not support streaming")
}

// EndpointURL returns empty for the mock provider (assumed reachable).
func (m *MockProvider) EndpointURL() string { return "" }
