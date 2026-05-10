// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package provider

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// --- Test helpers ---

// countingProvider records how many times Execute was called.
type countingProvider struct {
	name      string
	callCount atomic.Int32
	result    agentctx.ProviderResult
	err       error
}

func (c *countingProvider) Name() string                                    { return c.name }
func (c *countingProvider) Capabilities() agentctx.ProviderCapabilities      { return agentctx.ProviderCapabilities{MaxContextTokens: 128000} }
func (c *countingProvider) EndpointURL() string                              { return "" }
func (c *countingProvider) Prepare(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	return &struct{}{}, nil
}
func (c *countingProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	c.callCount.Add(1)
	return c.result, c.err
}
func (c *countingProvider) Parse(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	return agentctx.ParsedResponse{}, nil
}
func (c *countingProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	return nil, errors.New("not implemented")
}
func (c *countingProvider) Calls() int { return int(c.callCount.Load()) }

// ============================================================================
// Test: FallbackExecutor
// ============================================================================

func TestFallbackExecutor_SuccessOnPrimary(t *testing.T) {
	primary := &countingProvider{name: "primary", result: agentctx.ProviderResult{RawResponse: []byte("ok")}}

	registry := NewProviderRegistry()
	registry.Register("primary", primary)
	registry.SetPrimary("primary")

	exec := NewFallbackExecutor(registry, nil)

	_, err := exec.Execute(context.Background(), primary, &agentctx.ConversationContext{}, agentctx.PreparedRequest{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if primary.Calls() != 1 {
		t.Errorf("primary called %d times, want 1 (no fallback needed)", primary.Calls())
	}
}

func TestFallbackExecutor_FallsBackOnPrimaryFailure(t *testing.T) {
	primary := &countingProvider{name: "primary", err: errors.New("primary down")}
	fallback := &countingProvider{name: "fallback", result: agentctx.ProviderResult{RawResponse: []byte("fb ok")}}

	registry := NewProviderRegistry()
	registry.Register("primary", primary)
	registry.Register("fallback", fallback)
	registry.SetPrimary("primary")
	registry.SetFallbackChain([]string{"fallback"})

	exec := NewFallbackExecutor(registry, nil)

	_, err := exec.Execute(context.Background(), primary, &agentctx.ConversationContext{}, agentctx.PreparedRequest{})

	if err != nil {
		t.Fatalf("unexpected error after fallback: %v", err)
	}
	if primary.Calls() != 1 {
		t.Errorf("primary called %d times, want 1", primary.Calls())
	}
	if fallback.Calls() != 1 {
		t.Errorf("fallback called %d times, want 1 (should have been invoked)", fallback.Calls())
	}
}

func TestFallbackExecutor_AllFail(t *testing.T) {
	primary := &countingProvider{name: "primary", err: errors.New("primary down")}
	fallback := &countingProvider{name: "fallback", err: errors.New("fallback also down")}

	registry := NewProviderRegistry()
	registry.Register("primary", primary)
	registry.Register("fallback", fallback)
	registry.SetPrimary("primary")
	registry.SetFallbackChain([]string{"fallback"})

	exec := NewFallbackExecutor(registry, nil)

	_, err := exec.Execute(context.Background(), primary, &agentctx.ConversationContext{}, agentctx.PreparedRequest{})

	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if primary.Calls() != 1 {
		t.Errorf("primary called %d times, want 1", primary.Calls())
	}
	if fallback.Calls() != 1 {
		t.Errorf("fallback called %d times, want 1 (should have been attempted)", fallback.Calls())
	}
}

func TestFallbackExecutor_SkipsNonexistentFallback(t *testing.T) {
	primary := &countingProvider{name: "primary", err: errors.New("primary down")}

	registry := NewProviderRegistry()
	registry.Register("primary", primary)
	registry.SetPrimary("primary")
	registry.SetFallbackChain([]string{"nonexistent"})

	exec := NewFallbackExecutor(registry, nil)

	_, err := exec.Execute(context.Background(), primary, &agentctx.ConversationContext{}, agentctx.PreparedRequest{})

	if err == nil {
		t.Fatal("expected error when primary fails and fallback doesn't exist")
	}
	if primary.Calls() != 1 {
		t.Errorf("primary called %d times, want 1", primary.Calls())
	}
}

// ============================================================================
// Test: MetricsProvider
// ============================================================================

type passthroughProvider struct {
	name string
	result agentctx.ProviderResult
	err    error
	caps   agentctx.ProviderCapabilities
}

func (p *passthroughProvider) Name() string                          { return p.name }
func (p *passthroughProvider) Capabilities() agentctx.ProviderCapabilities {
	if p.caps.MaxContextTokens > 0 { return p.caps }
	return agentctx.ProviderCapabilities{SupportsStreaming: true, MaxContextTokens: 64000}
}
func (p *passthroughProvider) EndpointURL() string                    { return "" }
func (p *passthroughProvider) Prepare(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	return &struct{}{}, nil
}
func (p *passthroughProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	return p.result, p.err
}
func (p *passthroughProvider) Parse(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	return agentctx.ParsedResponse{}, nil
}
func (p *passthroughProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	ch := make(chan agentctx.StreamEvent)
	close(ch)
	return ch, nil
}

func TestMetricsProvider_ExecutePassThrough(t *testing.T) {
	inner := &passthroughProvider{name: "inner", result: agentctx.ProviderResult{RawResponse: []byte("data")}}
	metrics := NewMetricsProvider(inner)

	result, err := metrics.Execute(context.Background(), &struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RawResponse == nil {
		t.Error("result was lost by decorator — expected pass-through")
	}
}

func TestMetricsProvider_ExecuteErrorPassThrough(t *testing.T) {
	innerErr := errors.New("inner failed")
	inner := &passthroughProvider{name: "inner", err: innerErr}
	metrics := NewMetricsProvider(inner)

	_, err := metrics.Execute(context.Background(), &struct{}{})

	if !errors.Is(err, innerErr) {
		t.Errorf("expected inner error to pass through, got: %v", err)
	}
}

func TestMetricsProvider_NamePassThrough(t *testing.T) {
	inner := &passthroughProvider{name: "test-name"}
	metrics := NewMetricsProvider(inner)

	if got := metrics.Name(); got != "test-name" {
		t.Errorf("Name() = %q, want %q", got, "test-name")
	}
}

func TestMetricsProvider_CapabilitiesPassThrough(t *testing.T) {
	caps := agentctx.ProviderCapabilities{
		SupportsStreaming: true,
		MaxContextTokens:  64000,
	}
	inner := &passthroughProvider{name: "inner", caps: caps}
	metrics := NewMetricsProvider(inner)

	got := metrics.Capabilities()
	if got.MaxContextTokens != 64000 {
		t.Errorf("Capabilities() lost context tokens: got %d, want 64000", got.MaxContextTokens)
	}
	if !got.SupportsStreaming {
		t.Error("Capabilities() lost streaming support")
	}
}
