// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package provider

import (
	"context"
	"fmt"
	"time"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	agentlog "github.com/aj-nt/mnemosyne/agent/log"
)

// --- FallbackExecutor ---
//
// Encapsulates the "try primary provider, fall back through registry chain
// on failure" pattern. Replaces duplicated fallback logic in streaming_caller.go.

type FallbackExecutor struct {
	registry *ProviderRegistry
	router   *ModelRouter
}

func NewFallbackExecutor(registry *ProviderRegistry, router *ModelRouter) *FallbackExecutor {
	return &FallbackExecutor{registry: registry, router: router}
}

// Execute tries primary, then each fallback in the registry's fallback chain.
func (f *FallbackExecutor) Execute(
	ctx context.Context,
	primary Provider,
	convCtx *agentctx.ConversationContext,
	prepared agentctx.PreparedRequest,
) (agentctx.ProviderResult, error) {
	req, prepErr := primary.Prepare(ctx, convCtx, prepared)
	if prepErr != nil {
		_ = prepErr
		return agentctx.ProviderResult{}, fmt.Errorf("prepare: %w", prepErr)
	}

	result, err := primary.Execute(ctx, req)
	if err == nil && result.Error == nil {
		return result, nil
	}

	agentlog.Printf("[fallback] %s failed, trying fallback chain", primary.Name())
	f.recordFailure(primary.Name())

	for _, fbName := range f.registry.fallbackChain {
		if fbName == primary.Name() {
			continue
		}
		fb, getErr := f.registry.Get(fbName)
		if getErr != nil {
			continue
		}
		agentlog.Printf("[fallback] trying %s...", fbName)
		result, err = fb.Execute(ctx, req)
		if err == nil && result.Error == nil {
			return result, nil
		}
		f.recordFailure(fbName)
	}

	return agentctx.ProviderResult{}, fmt.Errorf("all providers exhausted: %w", err)
}

func (f *FallbackExecutor) recordFailure(name string) {
	if f.router != nil {
		f.router.RecordFailure(name)
	}
}

// --- MetricsProvider ---
//
// Wraps a Provider with latency logging for Execute and Stream calls.
// All other methods pass through unchanged via the embedded DecoratedProvider.

type DecoratedProvider struct {
	Inner Provider
}

func (d *DecoratedProvider) Name() string                                    { return d.Inner.Name() }
func (d *DecoratedProvider) Capabilities() agentctx.ProviderCapabilities      { return d.Inner.Capabilities() }
func (d *DecoratedProvider) EndpointURL() string                              { return d.Inner.EndpointURL() }
func (d *DecoratedProvider) Prepare(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	return d.Inner.Prepare(ctx, convCtx, prepared)
}
func (d *DecoratedProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	return d.Inner.Execute(ctx, req)
}
func (d *DecoratedProvider) Parse(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	return d.Inner.Parse(ctx, result)
}
func (d *DecoratedProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	return d.Inner.Stream(ctx, req)
}

type MetricsProvider struct {
	DecoratedProvider
}

func NewMetricsProvider(inner Provider) *MetricsProvider {
	return &MetricsProvider{DecoratedProvider{Inner: inner}}
}

func (m *MetricsProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	start := time.Now()
	result, err := m.Inner.Execute(ctx, req)
	elapsed := time.Since(start)
	if err != nil {
		agentlog.Printf("[metrics] %s.Execute failed after %v: %v", m.Inner.Name(), elapsed, err)
	} else if result.Error != nil {
		agentlog.Printf("[metrics] %s.Execute returned error after %v: %v", m.Inner.Name(), elapsed, result.Error)
	}
	return result, err
}

func (m *MetricsProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	start := time.Now()
	ch, err := m.Inner.Stream(ctx, req)
	elapsed := time.Since(start)
	if err != nil {
		agentlog.Printf("[metrics] %s.Stream failed after %v: %v", m.Inner.Name(), elapsed, err)
	}
	return ch, err
}
