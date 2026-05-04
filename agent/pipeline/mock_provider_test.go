// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"context"
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
)

// mockProvider implements provider.Provider for testing
type mockProvider struct {
	name         string
	capabilities agentctx.ProviderCapabilities
	prepareFn    func(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error)
	executeFn    func(ctx context.Context, req interface{}) (agentctx.ProviderResult, error)
	parseFn      func(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error)
	streamFn     func(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error)
}

func (m *mockProvider) Name() string                                { return m.name }
func (m *mockProvider) Capabilities() agentctx.ProviderCapabilities   { return m.capabilities }
func (m *mockProvider) Prepare(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	if m.prepareFn != nil {
		return m.prepareFn(ctx, convCtx, prepared)
	}
	return nil, nil
}
func (m *mockProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, req)
	}
	return agentctx.ProviderResult{}, nil
}
func (m *mockProvider) Parse(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	if m.parseFn != nil {
		return m.parseFn(ctx, result)
	}
	return agentctx.ParsedResponse{}, nil
}
func (m *mockProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, req)
	}
	return nil, fmt.Errorf("streaming not supported")
}
func (m *mockProvider) EndpointURL() string { return "" }

// Compile-time check that mockProvider satisfies provider.Provider
var _ provider.Provider = (*mockProvider)(nil)