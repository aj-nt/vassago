// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

// contextOverflowProvider returns an error simulating context overflow
// from the LLM API (e.g., "prompt too long; exceeded max context length").
type contextOverflowProvider struct {
	callCount int
}

func (p *contextOverflowProvider) Name() string { return "overflow" }
func (p *contextOverflowProvider) Capabilities() agentctx.ProviderCapabilities {
	return agentctx.ProviderCapabilities{
		SupportsStreaming:       false,
		SupportsFunctionCalling: true,
		SupportsPromptCaching:  false,
		SupportsVision:         false,
		MaxContextTokens:       128000,
	}
}
func (p *contextOverflowProvider) Prepare(_ context.Context, _ *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	return prepared, nil
}
func (p *contextOverflowProvider) Execute(_ context.Context, _ interface{}) (agentctx.ProviderResult, error) {
	p.callCount++
	return agentctx.ProviderResult{
		Error:          fmt.Errorf("Error code: 400 - prompt too long; exceeded max context length by 25 tokens"),
		ShouldRetry:    false,
		ShouldFallback: false,
	}, nil
}
func (p *contextOverflowProvider) Parse(_ context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	return agentctx.ParsedResponse{}, result.Error
}
func (p *contextOverflowProvider) Stream(_ context.Context, _ interface{}) (<-chan agentctx.StreamEvent, error) {
	return nil, fmt.Errorf("streaming not supported")
}
func (p *contextOverflowProvider) EndpointURL() string { return "" }

// TestRunStreaming_ContextOverflowStopsLoop verifies that when the LLM returns a
// context overflow error, the agent does NOT loop infinitely. Previously, the
// agent would retry forever because the iteration counter never incremented on error.
func TestRunStreaming_ContextOverflowStopsLoop(t *testing.T) {
	overflowProvider := &contextOverflowProvider{}

	registry := provider.NewProviderRegistry()
	registry.Register("overflow", overflowProvider)
	registry.SetPrimary("overflow")

	toolReg := tools.NewToolRegistry()
	cfg := Config{
		AgentID:          "test-agent",
		MaxTurns:         50, // high limit — should terminate via error, not iterations
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	// Use RunStreaming to get events
	convCtx := &agentctx.ConversationContext{
		AgentID:       "test-agent",
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 50,
		Iteration:     0,
		Interrupt:     agentctx.NewInterruptSignal(),
		Temperature:   0.7,
	}

	eventCh := make(chan StreamEvent, 64)
	done := make(chan struct{})

	go func() {
		a.RunStreaming(context.Background(), convCtx, eventCh)
		close(done)
	}()

	// Wait for RunStreaming to complete first, then drain all buffered events
	timeout := time.After(5 * time.Second)
	select {
	case <-done:
		// Good — RunStreaming completed
	case <-timeout:
		t.Fatal("Test timed out — agent is in an infinite loop on context overflow")
	}

	// Drain all buffered events from the channel
	var events []StreamEvent
	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				goto drained1
			}
			events = append(events, event)
		default:
			goto drained1
		}
	}
drained1:

	// The agent should have stopped with an error, not hung infinitely
	// Overflow provider should have been called only a few times before the consecutive error limit kicks in
	if overflowProvider.callCount > 3 {
		t.Errorf("Provider was called %d times — expected at most 3 (context overflow should stop the loop)", overflowProvider.callCount)
	}

	// Verify we got an error event
	hasError := false
	for _, e := range events {
		if e.Error != nil {
			hasError = true
		}
	}
	if !hasError {
		t.Error("Expected an error event from context overflow, got none")
	}
}

// TestRunWithContext_ContextOverflowReturnsError verifies the non-streaming
// path also handles context overflow gracefully.
func TestRunWithContext_ContextOverflowReturnsError(t *testing.T) {
	overflowProvider := &contextOverflowProvider{}

	registry := provider.NewProviderRegistry()
	registry.Register("overflow", overflowProvider)
	registry.SetPrimary("overflow")

	toolReg := tools.NewToolRegistry()
	cfg := Config{
		AgentID:          "test-agent",
		MaxTurns:         50,
		MaxContextTokens: 128000,
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	// Non-streaming path: RunWithContext should return an error
	convCtx := &agentctx.ConversationContext{
		AgentID:       "test-agent",
		Messages:      []agentctx.Message{{Role: "user", Content: "hello"}},
		MaxIterations: 50,
		Iteration:     0,
		Interrupt:     agentctx.NewInterruptSignal(),
		Temperature:   0.7,
	}

	_, err := a.RunWithContext(context.Background(), convCtx)

	// Should get an error
	if err == nil {
		t.Error("Expected error from context overflow, got nil")
	}

	// Should not have called the provider more than a few times
	if overflowProvider.callCount > 3 {
		t.Errorf("Provider was called %d times — context overflow should stop the loop", overflowProvider.callCount)
	}
}

// TestRunStreaming_ContextLimitYields tests that when the context exceeds the
// token limit at the start of a turn, the agent YIELDs gracefully with an
// informative message rather than silently terminating or looping.
func TestRunStreaming_ContextLimitYields(t *testing.T) {
	// Create a provider that would respond normally, but we'll set a very low
	// context limit so it terminates before calling the provider
	mockProvider := provider.NewMockProvider("mock", []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "should not be reached",
				FinishReason: "stop",
			},
		},
	})
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	toolReg := tools.NewToolRegistry()
	cfg := Config{
		AgentID:          "test-agent",
		MaxTurns:         50,
		MaxContextTokens: 10, // Extremely low — any message exceeds this
	}
	a := NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	// Create a context with messages that exceed the token limit
	longMsg := "this is a longer message that definitely exceeds our very low token limit of ten tokens"
	convCtx := &agentctx.ConversationContext{
		AgentID:       "test-agent",
		Messages:      []agentctx.Message{{Role: "user", Content: longMsg}},
		MaxIterations: 50,
		Iteration:     1, // past first turn
		Interrupt:     agentctx.NewInterruptSignal(),
		Temperature:   0.7,
	}

	eventCh := make(chan StreamEvent, 64)
	done := make(chan struct{})

	go func() {
		a.RunStreaming(context.Background(), convCtx, eventCh)
		close(done)
	}()

	// Wait for RunStreaming to complete first, then drain all buffered events
	timeout := time.After(5 * time.Second)
	select {
	case <-done:
		// Good — RunStreaming completed
	case <-timeout:
		t.Fatal("Test timed out — agent is in an infinite loop on context limit")
	}

	// Drain all buffered events from the channel
	var events []StreamEvent
	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				goto drained
			}
			events = append(events, event)
		default:
			goto drained
		}
	}
drained:

	// Should have gotten a StreamDone event
	hasDone := false
	for _, e := range events {
		if e.Type == agentctx.StreamDone {
			hasDone = true
		}
	}
	if !hasDone {
		t.Error("Expected StreamDone event when context limit exceeded")
	}

	// The provider should NOT have been called (we were over limit before calling)
	if mockProvider.CallCount() > 0 {
		t.Errorf("Provider should not be called when context is over limit, was called %d times", mockProvider.CallCount())
	}
}