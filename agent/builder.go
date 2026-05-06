// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/pipeline"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
	memoclient "github.com/aj-nt/mnemosyne/client"
)

// AgentBuilder constructs an Agent using the Factory Method pattern.
// It eliminates the duplication between NewAgent, NewAgentWithRouter,
// and SubAgentRunnerImpl by using a fluent builder that composes
// the same wiring steps in one place.
//
// Usage:
//
//	agent := NewAgentBuilder(cfg, registry, toolReg, mnemo).
//	    WithRouter(router).
//	    Build()
type AgentBuilder struct {
	cfg      Config
	registry *provider.ProviderRegistry
	router   *provider.ModelRouter // optional
	toolReg  *tools.ToolRegistry
	mnemo memoclient.MnemoClient
	bus      *eventbus.EventBus // optional: created if not set

	// Stage overrides for testing
	stage1Override *pipeline.RequestPrepStage
	stage2Override *pipeline.ProviderCallStage
	stage3Override *pipeline.ResponseProcessingStage
	stage4Override *pipeline.ToolDispatchStage
	stage5Override *pipeline.ContextManagementStage
}

// NewAgentBuilder starts building an Agent with the required dependencies.
func NewAgentBuilder(cfg Config, registry *provider.ProviderRegistry, toolReg *tools.ToolRegistry, mnemo memoclient.MnemoClient) *AgentBuilder {
	return &AgentBuilder{
		cfg:      cfg,
		registry: registry,
		toolReg:  toolReg,
		mnemo:    mnemo,
	}
}

// WithRouter adds a model router to the agent being built.
func (b *AgentBuilder) WithRouter(router *provider.ModelRouter) *AgentBuilder {
	b.router = router
	return b
}

// WithEventBus uses an existing EventBus instead of creating a new one.
// Useful for sub-agents that share the parent's event bus.
func (b *AgentBuilder) WithEventBus(bus *eventbus.EventBus) *AgentBuilder {
	b.bus = bus
	return b
}

// WithStage1 overrides Stage 1 (for testing).
func (b *AgentBuilder) WithStage1(s *pipeline.RequestPrepStage) *AgentBuilder {
	b.stage1Override = s
	return b
}

// WithStage2 overrides Stage 2 (for testing).
func (b *AgentBuilder) WithStage2(s *pipeline.ProviderCallStage) *AgentBuilder {
	b.stage2Override = s
	return b
}

// WithStage3 overrides Stage 3 (for testing).
func (b *AgentBuilder) WithStage3(s *pipeline.ResponseProcessingStage) *AgentBuilder {
	b.stage3Override = s
	return b
}

// WithStage4 overrides Stage 4 (for testing).
func (b *AgentBuilder) WithStage4(s *pipeline.ToolDispatchStage) *AgentBuilder {
	b.stage4Override = s
	return b
}

// WithStage5 overrides Stage 5 (for testing).
func (b *AgentBuilder) WithStage5(s *pipeline.ContextManagementStage) *AgentBuilder {
	b.stage5Override = s
	return b
}

// Build constructs the Agent. All wiring happens in one place.
func (b *AgentBuilder) Build() *Agent {
	// Create EventBus if not provided
	bus := b.bus
	if bus == nil {
		bus = eventbus.NewEventBus()
	}

	a := &Agent{
		Config:       b.cfg,
		Registry:     b.registry,
		Router:       b.router,
		ToolRegistry: b.toolReg,
		EventBus:     bus,
		Mnemo:        b.mnemo,
	}

	// Wire pipeline stages — allow overrides for testing
	if b.stage1Override != nil {
		a.Stage1 = b.stage1Override
	} else {
		a.Stage1 = pipeline.NewRequestPrepStage(b.cfg.Personality)
	}

	if b.stage2Override != nil {
		a.Stage2 = b.stage2Override
	} else if b.router != nil {
		a.Stage2 = pipeline.NewProviderCallStageWithRouter(b.registry, b.router)
	} else {
		a.Stage2 = pipeline.NewProviderCallStage(b.registry)
	}

	if b.stage3Override != nil {
		a.Stage3 = b.stage3Override
	} else {
		a.Stage3 = pipeline.NewResponseProcessingStage(b.registry)
	}

	if b.stage4Override != nil {
		a.Stage4 = b.stage4Override
	} else {
		a.Stage4 = pipeline.NewToolDispatchStage(b.toolReg, bus)
	}

	if b.stage5Override != nil {
		a.Stage5 = b.stage5Override
	} else {
		a.Stage5 = pipeline.NewContextManagementStage(b.cfg.MaxContextTokens)
		a.Stage5.SetEventBus(bus)
	}

	return a
}