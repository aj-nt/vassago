// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
)

func TestSubAgentRunnerImpl_Success(t *testing.T) {
	mockResponses := []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "Sub-agent research complete: found 3 results",
				FinishReason: "stop",
			},
		},
	}
	mockProvider := provider.NewMockProvider("mock", mockResponses)
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	runner := &SubAgentRunnerImpl{
		Mnemo:    nil, // no daemon
		Registry: registry,
		Router:   nil,
	}

	result, err := runner.RunSubAgent("research the topic of quantum computing", "test-sub-1", 5)
	if err != nil {
		t.Fatalf("RunSubAgent failed: %v", err)
	}
	if result.Text != "Sub-agent research complete: found 3 results" {
		t.Errorf("Expected sub-agent response, got %q", result.Text)
	}
	if result.IsError {
		t.Error("Expected IsError=false")
	}
}

func TestSubAgentRunnerImpl_GeneratesAgentID(t *testing.T) {
	mockResponses := []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "Done",
				FinishReason: "stop",
			},
		},
	}
	mockProvider := provider.NewMockProvider("mock", mockResponses)
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	runner := &SubAgentRunnerImpl{
		Mnemo:    nil,
		Registry: registry,
		Router:   nil,
	}

	result, err := runner.RunSubAgent("test task", "test-sub-2", 3)
	if err != nil {
		t.Fatalf("RunSubAgent failed: %v", err)
	}
	if result.Text != "Done" {
		t.Errorf("Expected 'Done', got %q", result.Text)
	}
}

func TestSubAgentRunnerImpl_SubAgentHasMemoryTools(t *testing.T) {
	// Sub-agent should have memory tools but NOT Delegate
	mockResponses := []provider.CannedResponse{
		{
			Response: agentctx.ParsedResponse{
				Text:         "I searched and found nothing",
				FinishReason: "stop",
			},
		},
	}
	mockProvider := provider.NewMockProvider("mock", mockResponses)
	registry := provider.NewProviderRegistry()
	registry.Register("mock", mockProvider)
	registry.SetPrimary("mock")

	runner := &SubAgentRunnerImpl{
		Mnemo:    nil,
		Registry: registry,
		Router:   nil,
	}

	result, err := runner.RunSubAgent("search for something", "test-sub-3", 5)
	if err != nil {
		t.Fatalf("RunSubAgent failed: %v", err)
	}
	if result.Text != "I searched and found nothing" {
		t.Errorf("Expected sub-agent response, got %q", result.Text)
	}
}

func TestSubAgentRunnerImpl_ProviderError(t *testing.T) {
	// A nil primary provider should cause an error
	registry := provider.NewProviderRegistry()
	// Don't register any provider — primary lookup will fail

	runner := &SubAgentRunnerImpl{
		Mnemo:    nil,
		Registry: registry,
		Router:   nil,
	}

	_, err := runner.RunSubAgent("test task", "test-sub-4", 3)
	if err == nil {
		t.Error("Expected error when no primary provider is registered")
	}
}