// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"testing"

	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/pipeline"
)

func TestNewAgentBuilder_Default(t *testing.T) {
	cfg := Config{
		AgentID:          "test-agent",
		AgentName:        "Test Agent",
		AgentRole:        "assistant",
		Personality:      "direct",
		MaxTurns:          50,
		MaxContextTokens:  128000,
	}
	registry := setupTestRegistry()
	toolReg := setupTestToolReg()

	agent := NewAgentBuilder(cfg, registry, toolReg, nil).Build()
	if agent == nil {
		t.Fatal("Expected non-nil agent from builder")
	}
	if agent.EventBus == nil {
		t.Error("Expected EventBus to be created by builder")
	}
	if agent.Stage1 == nil {
		t.Error("Expected Stage1 to be wired")
	}
	if agent.Stage2 == nil {
		t.Error("Expected Stage2 to be wired")
	}
	if agent.Stage3 == nil {
		t.Error("Expected Stage3 to be wired")
	}
	if agent.Stage4 == nil {
		t.Error("Expected Stage4 to be wired")
	}
	if agent.Stage5 == nil {
		t.Error("Expected Stage5 to be wired")
	}
	if agent.Router != nil {
		t.Error("Expected Router to be nil for builder without WithRouter")
	}
}

func TestNewAgentBuilder_WithRouter(t *testing.T) {
	cfg := Config{
		AgentID:          "test-agent",
		AgentName:        "Test Agent",
		AgentRole:        "assistant",
		Personality:      "direct",
		MaxTurns:          50,
		MaxContextTokens:  128000,
	}
	registry := setupTestRegistry()
	toolReg := setupTestToolReg()

	// Test that WithRouter stores the router
	builder := NewAgentBuilder(cfg, registry, toolReg, nil)
	if builder == nil {
		t.Fatal("Expected non-nil builder")
	}
	// WithRouter stores the router — we verify it's set on the built agent
	router := setupTestRouter()
	agent := builder.WithRouter(router).Build()
	if agent == nil {
		t.Fatal("Expected non-nil agent from builder with router")
	}
	if agent.Router != router {
		t.Error("Expected Router to be set with WithRouter")
	}
}

func TestNewAgentBuilder_StageOverrides(t *testing.T) {
	cfg := Config{
		AgentID:          "test-agent",
		AgentName:        "Test Agent",
		AgentRole:        "assistant",
		Personality:      "direct",
		MaxTurns:          50,
		MaxContextTokens:  128000,
	}
	registry := setupTestRegistry()
	toolReg := setupTestToolReg()

	// Override Stage5 with a custom one
	customStage5 := pipeline.NewContextManagementStage(999)
	agent := NewAgentBuilder(cfg, registry, toolReg, nil).
		WithStage5(customStage5).
		Build()

	if agent.Stage5 != customStage5 {
		t.Error("Expected Stage5 to be overridden with custom stage")
	}
}

func TestNewAgentBuilder_WithEventBus(t *testing.T) {
	cfg := Config{
		AgentID:          "test-agent",
		AgentName:        "Test Agent",
		AgentRole:        "assistant",
		Personality:      "direct",
		MaxTurns:          50,
		MaxContextTokens:  128000,
	}
	registry := setupTestRegistry()
	toolReg := setupTestToolReg()

	// Use a shared event bus
	bus := eventbus.NewEventBus()
	agent := NewAgentBuilder(cfg, registry, toolReg, nil).
		WithEventBus(bus).
		Build()

	if agent.EventBus != bus {
		t.Error("Expected EventBus to be the shared bus we provided")
	}
}