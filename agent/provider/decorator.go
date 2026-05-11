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

// ============================================================================
// DecoratedProvider — base struct for decorator pattern.
// All Provider methods pass through to Inner by default.
// Decorators embed this and override only the methods they need.
// ============================================================================

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

// ============================================================================
// RetryProvider — retries Execute/Stream on transient failures.
// Uses WithRetry under the hood. RetryConfig controls behavior.
// ============================================================================

type RetryProvider struct {
	DecoratedProvider
	config RetryConfig
}

// NewRetryProvider wraps a provider with retry logic.
func NewRetryProvider(inner Provider, config RetryConfig) *RetryProvider {
	if config.MaxRetries == 0 && config.BaseDelay == 0 {
		config = DefaultRetryConfig()
	}
	return &RetryProvider{
		DecoratedProvider: DecoratedProvider{Inner: inner},
		config:            config,
	}
}

// Execute retries the inner provider's Execute on transient failures.
func (r *RetryProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	return WithRetry(ctx, r.config, func() (agentctx.ProviderResult, error) {
		return r.Inner.Execute(ctx, req)
	})
}

// ============================================================================
// CacheProvider — tracks prompt cache hit/miss metrics.
// Wraps Execute results to increment cache counters based on UsageInfo.
// ============================================================================

type CacheProvider struct {
	DecoratedProvider
}

// NewCacheProvider wraps a provider with cache metric tracking.
func NewCacheProvider(inner Provider) *CacheProvider {
	return &CacheProvider{DecoratedProvider{Inner: inner}}
}

// Execute delegates to inner, then records cache metrics from the result.
func (c *CacheProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	result, err := c.Inner.Execute(ctx, req)
	if err == nil {
		c.recordCacheMetrics(result)
	}
	return result, err
}

func (c *CacheProvider) recordCacheMetrics(result agentctx.ProviderResult) {
	// Cache metrics are recorded by the provider's Parse method (e.g., Anthropic's
	// convertAnthropicUsage). The CacheProvider is a safety net for providers that
	// don't self-report — a future enhancement could introspect raw responses here.
	_ = result
}

// ============================================================================
// MetricsProvider — latency + structured logging for Provider calls.
// Emits provider.call spans with duration, model, and error status.
// ============================================================================

type MetricsProvider struct {
	DecoratedProvider
}

// NewMetricsProvider wraps a provider with latency + structured logging.
func NewMetricsProvider(inner Provider) *MetricsProvider {
	return &MetricsProvider{DecoratedProvider{Inner: inner}}
}

// Execute logs latency and structured metrics for the Execute call.
func (m *MetricsProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	start := time.Now()
	result, err := m.Inner.Execute(ctx, req)
	elapsed := time.Since(start)

	if err != nil {
		agentlog.Warn("provider.execute",
			"provider", m.Inner.Name(),
			"duration_ms", elapsed.Milliseconds(),
			"error", err.Error(),
		)
	} else if result.Error != nil {
		agentlog.Warn("provider.execute",
			"provider", m.Inner.Name(),
			"duration_ms", elapsed.Milliseconds(),
			"result_error", result.Error.Error(),
		)
	}
	return result, err
}

// Stream logs latency for stream initiation.
func (m *MetricsProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	start := time.Now()
	ch, err := m.Inner.Stream(ctx, req)
	elapsed := time.Since(start)

	if err != nil {
		agentlog.Warn("provider.stream_init",
			"provider", m.Inner.Name(),
			"duration_ms", elapsed.Milliseconds(),
			"error", err.Error(),
		)
	}
	return ch, err
}

// ============================================================================
// FallbackProvider — on failure, tries the registry's fallback chain.
//
// This is the key Phase 37 deliverable: it replaces the ad-hoc fallback logic
// in streaming_caller.go (both callNonStreaming and processStreamingEvents).
//
// Execute: if inner fails, iterate through registry's fallbackChain,
// calling Prepare+Execute on each fallback provider.
//
// Stream: if inner's Stream() fails, iterate through fallbackChain calling
// Prepare+Execute (non-streaming) on each.
// ============================================================================

type FallbackProvider struct {
	DecoratedProvider
	registry     *ProviderRegistry
	router       *ModelRouter
	// Saved at Prepare time for use during fallback.
	savedConvCtx *agentctx.ConversationContext
	savedPrepared agentctx.PreparedRequest
}

// NewFallbackProvider wraps a provider with fallback-chain support.
// When the inner provider fails, the fallback chain from the registry is tried.
func NewFallbackProvider(inner Provider, registry *ProviderRegistry, router *ModelRouter) *FallbackProvider {
	return &FallbackProvider{
		DecoratedProvider: DecoratedProvider{Inner: inner},
		registry:          registry,
		router:            router,
	}
}

// Prepare saves context for fallback use, then delegates to inner.
func (f *FallbackProvider) Prepare(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	f.savedConvCtx = convCtx
	f.savedPrepared = prepared
	return f.Inner.Prepare(ctx, convCtx, prepared)
}

// Execute tries inner, then each fallback in the registry's fallback chain.
func (f *FallbackProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	result, err := f.Inner.Execute(ctx, req)
	if err == nil && result.Error == nil {
		return result, nil
	}
	if err == nil && !result.ShouldFallback {
		return result, nil
	}

	agentlog.Info("provider.fallback",
		"failed_provider", f.Inner.Name(),
		"reason", fallbackReason(err, result),
	)
	f.recordFailure(f.Inner.Name())

	// Try each fallback provider
	for _, fbName := range f.registry.fallbackChain {
		if fbName == f.Inner.Name() {
			continue
		}
		fb, getErr := f.registry.Get(fbName)
		if getErr != nil {
			agentlog.Debug("provider.fallback_skip",
				"provider", fbName,
				"reason", "not registered",
			)
			continue
		}

		agentlog.Info("provider.fallback_try", "provider", fbName)

		fbReq, prepErr := fb.Prepare(ctx, f.savedConvCtx, f.savedPrepared)
		if prepErr != nil {
			agentlog.Warn("provider.fallback_prepare_error",
				"provider", fbName,
				"error", prepErr.Error(),
			)
			f.recordFailure(fbName)
			continue
		}

		result, err = fb.Execute(ctx, fbReq)
		if err == nil && result.Error == nil {
			return result, nil
		}
		f.recordFailure(fbName)
	}

	if result.Error != nil || err != nil {
		finalErr := err
		if finalErr == nil {
			finalErr = result.Error
		}
		return agentctx.ProviderResult{}, fmt.Errorf("all providers exhausted (%s + %d fallbacks): %w",
			f.Inner.Name(), len(f.registry.fallbackChain), finalErr)
	}
	return result, err
}

// Stream tries inner.Stream; on failure, falls back to non-streaming Execute
// through the fallback chain.
func (f *FallbackProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	ch, streamErr := f.Inner.Stream(ctx, req)
	if streamErr == nil {
		return ch, nil
	}

	// Stream init failed — try non-streaming through fallback chain
	agentlog.Info("provider.stream_fallback",
		"failed_provider", f.Inner.Name(),
		"error", streamErr.Error(),
	)

	result, execErr := f.Execute(ctx, req)
	if execErr != nil {
		return nil, execErr
	}

	// Wrap the non-streaming result in a one-shot stream channel
	out := make(chan agentctx.StreamEvent, 2)
	go func() {
		defer close(out)
		parsed, parseErr := f.Inner.Parse(ctx, result)
		if parseErr != nil {
			out <- agentctx.StreamEvent{
				Type:  agentctx.StreamError,
				Error: fmt.Errorf("parse after stream fallback: %w", parseErr),
			}
			return
		}
		if parsed.Text != "" {
			out <- agentctx.StreamEvent{
				Type: agentctx.StreamText,
				Text: parsed.Text,
			}
		}
		out <- agentctx.StreamEvent{
			Type: agentctx.StreamDone,
			Done: true,
		}
	}()
	return out, nil
}

func (f *FallbackProvider) recordFailure(name string) {
	if f.router != nil {
		f.router.RecordFailure(name)
	}
}

// ============================================================================
// FallbackExecutor — legacy compatibility wrapper over FallbackProvider.
// Kept for backward compatibility; new code should use FallbackProvider directly.
// ============================================================================

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
	fb := NewFallbackProvider(primary, f.registry, f.router)
	fb.Prepare(ctx, convCtx, prepared)
	return fb.Execute(ctx, nil)
}

// ============================================================================
// ComposeDecorators — apply the standard decorator chain.
//
// Applies: Metrics → Cache → Retry → Fallback (if primary)
// The order matters: Metrics is outermost (measures everything),
// Cache is next, Retry is next, Fallback is innermost.
// ============================================================================

// DecoratorConfig controls which decorators to apply.
type DecoratorConfig struct {
	// Retry enables retry logic on transient failures.
	Retry RetryConfig
	// EnableFallback enables the FallbackProvider (for the primary provider).
	EnableFallback bool
}

// ComposeDecorators wraps a provider with the standard decorator chain.
//   - inner: the raw provider
//   - registry: needed for FallbackProvider (can be nil if EnableFallback=false)
//   - router: needed for failure demotion (can be nil)
//   - config: controls which decorators to apply
func ComposeDecorators(inner Provider, registry *ProviderRegistry, router *ModelRouter, config DecoratorConfig) Provider {
	p := inner

	// Innermost: FallbackProvider (tries fallback chain on failure)
	if config.EnableFallback && registry != nil {
		p = NewFallbackProvider(p, registry, router)
	}

	// RetryProvider (retries on transient errors)
	if config.Retry.MaxRetries > 0 || config.Retry.BaseDelay > 0 {
		p = NewRetryProvider(p, config.Retry)
	}

	// CacheProvider (tracks cache metrics)
	p = NewCacheProvider(p)

	// Outermost: MetricsProvider (latency + structured logging)
	p = NewMetricsProvider(p)

	return p
}

func fallbackReason(err error, result agentctx.ProviderResult) string {
	if err != nil {
		return err.Error()
	}
	if result.Error != nil {
		return result.Error.Error()
	}
	return "unknown"
}
