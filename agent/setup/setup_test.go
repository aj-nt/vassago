// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package setup

import (
	"os"
	"testing"

	memoclient "github.com/aj-nt/mnemosyne/client"
	"github.com/aj-nt/mnemosyne/agent"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		inputs []string
		expect string
	}{
		{[]string{"", "", ""}, ""},
		{[]string{"", "hello", "world"}, "hello"},
		{[]string{"first", "second", "third"}, "first"},
		{[]string{"", "", "last"}, "last"},
	}

	for _, tt := range tests {
		got := FirstNonEmpty(tt.inputs...)
		if got != tt.expect {
			t.Errorf("FirstNonEmpty(%v) = %q, want %q", tt.inputs, got, tt.expect)
		}
	}
}

func TestAnyEnabled(t *testing.T) {
	enabled := []string{"AddMemory", "SearchMemories", "FileRead", "Execute"}

	if !AnyEnabled(enabled, "AddMemory") {
		t.Error("AddMemory should be enabled")
	}
	if !AnyEnabled(enabled, "AddMemory", "SearchMemories") {
		t.Error("AddMemory or SearchMemories should be enabled")
	}
	if AnyEnabled(enabled, "Browse") {
		t.Error("Browse should not be enabled")
	}
	if AnyEnabled(enabled, "Browse", "AnalyzeImage") {
		t.Error("Neither Browse nor AnalyzeImage should be enabled")
	}
	if !AnyEnabled(enabled, "Execute", "Browse") {
		t.Error("Execute is enabled, so anyEnabled should return true")
	}
}

func TestAnyEnabledEmpty(t *testing.T) {
	if AnyEnabled(nil, "AddMemory") {
		t.Error("Empty list should not enable anything")
	}
	if AnyEnabled([]string{}, "AddMemory") {
		t.Error("Empty slice should not enable anything")
	}
}

func TestProvidersRequiresAPIKey(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("OLLAMA_API_KEY")

	cfg := &agent.ProviderSection{
		Name:         "openai",
		Model:        "glm-5.1:cloud",
		OpenAIModel:  "glm-5.1:cloud",
		OpenAIBaseURL: "http://localhost:11434/v1",
	}

	registry := Providers(cfg)
	if registry == nil {
		t.Fatal("Registry should not be nil")
	}
	if _, err := registry.Get("openai"); err != nil {
		t.Errorf("OpenAI provider should be registered: %v", err)
	}
	if _, err := registry.Get("anthropic"); err == nil {
		t.Error("Anthropic provider should NOT be registered without API key")
	}
}

func TestProvidersWithAnthropic(t *testing.T) {
	os.Setenv("TEST_ANTHROPIC_KEY_19E", "sk-test-key")
	defer os.Unsetenv("TEST_ANTHROPIC_KEY_19E")

	cfg := &agent.ProviderSection{
		Name:               "anthropic",
		Model:              "claude-sonnet-4-6",
		AnthropicModel:     "claude-sonnet-4-6",
		AnthropicAPIKeyEnv: "TEST_ANTHROPIC_KEY_19E",
		Fallback1:          "openai",
		OpenAIModel:        "glm-5.1:cloud",
		OpenAIBaseURL:      "http://localhost:11434/v1",
	}

	registry := Providers(cfg)
	if _, err := registry.Get("anthropic"); err != nil {
		t.Errorf("Anthropic provider should be registered: %v", err)
	}
	if _, err := registry.Get("openai"); err != nil {
		t.Errorf("OpenAI provider should be registered: %v", err)
	}
}

func TestRouterCreation(t *testing.T) {
	os.Setenv("TEST_ROUTER_KEY_19E", "sk-test-key")
	defer os.Unsetenv("TEST_ROUTER_KEY_19E")

	cfg := &agent.ProviderSection{
		Name:               "anthropic",
		Model:              "claude-sonnet-4-6",
		AnthropicModel:     "claude-sonnet-4-6",
		AnthropicAPIKeyEnv: "TEST_ROUTER_KEY_19E",
		Fallback1:          "openai",
		OpenAIModel:        "glm-5.1:cloud",
		OpenAIBaseURL:      "http://localhost:11434/v1",
	}

	registry := Providers(cfg)
	routingCfg := &agent.RoutingSection{Strategy: "smart"}
	router := Router(routingCfg, cfg, registry)
	if router == nil {
		t.Fatal("Router should not be nil")
	}
}

func TestProvidersDefaultOpenAIURL(t *testing.T) {
	cfg := &agent.ProviderSection{
		Name:  "openai",
		Model: "gpt-4o-mini",
	}
	registry := Providers(cfg)
	if registry == nil {
		t.Fatal("Registry should not be nil")
	}
}

// --- Tools() tests ---

func TestTools_AllEnabledByDefault(t *testing.T) {
	// When ToolsSection.Enabled is empty, all standard tools are registered.
	mnemo := &memoclient.NullMnemo{}
	toolReg := Tools(ToolsConfig{
		ToolsSection:      &agent.ToolsSection{},
		DelegationSection: &agent.DelegationSection{},
		BrowserSection:    &agent.BrowserSection{},
		VisionSection:     &agent.VisionSection{},
		ImageGenSection:   &agent.ImageGenSection{},
		MultiAgentSection: &agent.MultiAgentSection{},
		MemorySafetySection:    &agent.MemorySafetySection{},
		BackgroundProcessSection: &agent.BackgroundProcessSection{},
		CodeExecSection:   &agent.CodeExecSection{},
		MnemoClient:       mnemo,
		AgentID:            "test-agent",
		AgentName:          "Test",
		AgentRole:          "assistant",
		ProviderRegistry:   provider.NewProviderRegistry(),
	})

	// Core tools should always be present
	for _, name := range []string{"AddMemory", "SearchMemories", "FileRead", "FileSearch", "Execute"} {
		if _, err := toolReg.Get(name); err != nil {
			t.Errorf("tool %q should be registered with empty enabled list: %v", name, err)
		}
	}
}

func TestTools_SelectiveEnabled(t *testing.T) {
	// When ToolsSection.Enabled specifies tools, only those are registered.
	mnemo := &memoclient.NullMnemo{}
	toolReg := Tools(ToolsConfig{
		ToolsSection: &agent.ToolsSection{
			Enabled: []string{"AddMemory", "SearchMemories"},
		},
		DelegationSection:        &agent.DelegationSection{},
		BrowserSection:            &agent.BrowserSection{},
		VisionSection:             &agent.VisionSection{},
		ImageGenSection:           &agent.ImageGenSection{},
		MultiAgentSection:         &agent.MultiAgentSection{},
		MemorySafetySection:       &agent.MemorySafetySection{},
		BackgroundProcessSection: &agent.BackgroundProcessSection{},
		CodeExecSection:           &agent.CodeExecSection{},
		MnemoClient:     mnemo,
		AgentID:         "test-agent",
		ProviderRegistry: provider.NewProviderRegistry(),
	})

	// Selected tools should be present
	if _, err := toolReg.Get("AddMemory"); err != nil {
		t.Errorf("AddMemory should be registered: %v", err)
	}

	// Unselected tools should NOT be present
	for _, name := range []string{"FileRead", "FileSearch", "Execute", "Browse"} {
		if _, err := toolReg.Get(name); err == nil {
			t.Errorf("tool %q should NOT be registered with selective enabled list", name)
		}
	}
}

func TestTools_DelegationEnabled(t *testing.T) {
	mnemo := &memoclient.NullMnemo{}
	toolReg := Tools(ToolsConfig{
		ToolsSection: &agent.ToolsSection{},
		DelegationSection: &agent.DelegationSection{Enabled: true, MaxDepth: 1, MaxTurns: 10},
		BrowserSection:    &agent.BrowserSection{},
		VisionSection:     &agent.VisionSection{},
		ImageGenSection:   &agent.ImageGenSection{},
		MultiAgentSection: &agent.MultiAgentSection{},
		MemorySafetySection:       &agent.MemorySafetySection{},
		BackgroundProcessSection: &agent.BackgroundProcessSection{},
		CodeExecSection:           &agent.CodeExecSection{},
		MnemoClient:     mnemo,
		AgentID:         "test-agent",
		SubAgentRunner:  tools.SubAgentRunner(nil),
		ProviderRegistry: provider.NewProviderRegistry(),
	})

	if _, err := toolReg.Get("Delegate"); err != nil {
		t.Errorf("Delegate should be registered when delegation is enabled: %v", err)
	}
}

func TestTools_BrowserEnabled(t *testing.T) {
	mnemo := &memoclient.NullMnemo{}
	toolReg := Tools(ToolsConfig{
		ToolsSection: &agent.ToolsSection{},
		DelegationSection: &agent.DelegationSection{},
		BrowserSection: &agent.BrowserSection{Enabled: true},
		VisionSection: &agent.VisionSection{},
		ImageGenSection: &agent.ImageGenSection{},
		MultiAgentSection: &agent.MultiAgentSection{},
		MemorySafetySection:       &agent.MemorySafetySection{},
		BackgroundProcessSection: &agent.BackgroundProcessSection{},
		CodeExecSection:           &agent.CodeExecSection{},
		MnemoClient:     mnemo,
		AgentID:         "test-agent",
		ProviderRegistry: provider.NewProviderRegistry(),
	})

	if _, err := toolReg.Get("Browse"); err != nil {
		t.Errorf("Browse should be registered when browser is enabled: %v", err)
	}
}

// --- Router() tests ---

func TestRouter_NoModels(t *testing.T) {
	routingCfg := &agent.RoutingSection{Strategy: "always-primary"}
	providerCfg := &agent.ProviderSection{}
	registry := provider.NewProviderRegistry()

	router := Router(routingCfg, providerCfg, registry)
	if router == nil {
		t.Fatal("Router should not be nil even with no models")
	}
}

func TestRouter_WithModels(t *testing.T) {
	routingCfg := &agent.RoutingSection{
		Strategy: "smart",
		Models: []provider.ModelConfig{
			{Name: "primary-model", Provider: "openai"},
			{Name: "fallback-model", Provider: "openai"},
		},
	}
	providerCfg := &agent.ProviderSection{
		Name:  "openai",
		Model: "primary-model",
	}
	registry := Providers(providerCfg)

	router := Router(routingCfg, providerCfg, registry)
	if router == nil {
		t.Fatal("Router should not be nil")
	}
	primary := router.Primary()
	if primary == "" {
		t.Error("Router should have a primary model")
	}
}

// --- createToolRegistry() tests ---

func TestCreateToolRegistry(t *testing.T) {
	mnemo := &memoclient.NullMnemo{}
	cfg := &agent.AgentConfig{
		Tools: agent.ToolsSection{},
		Delegation: agent.DelegationSection{},
		Browser: agent.BrowserSection{},
		Vision: agent.VisionSection{},
		ImageGen: agent.ImageGenSection{},
		MultiAgent: agent.MultiAgentSection{},
		MemorySafety: agent.MemorySafetySection{},
		BackgroundProcess: agent.BackgroundProcessSection{},
		CodeExec: agent.CodeExecSection{},
	}
	subRunner := agent.SubAgentRunnerImpl{Mnemo: mnemo, Registry: provider.NewProviderRegistry()}

	toolReg := createToolRegistry(cfg, mnemo, &subRunner, provider.NewProviderRegistry())
	if toolReg == nil {
		t.Fatal("createToolRegistry should return a non-nil registry")
	}
	// Should have memory tools at minimum
	if _, err := toolReg.Get("AddMemory"); err != nil {
		t.Errorf("AddMemory should be registered: %v", err)
	}
}

// --- Close() tests ---

func TestClose_CallsMnemoClose(t *testing.T) {
	// Close should call Mnemo.Close() without paning even when other components are nil.
	mnemo := &memoclient.NullMnemo{}
	r := &AgentResult{
		Agent:  &agent.Agent{Config: agent.Config{AgentID: "test"}},
		Mnemo:  mnemo,
	}
	// Should not panic
	r.Close()
}

func TestClose_WithNilComponents(t *testing.T) {
	// Close should handle nil Bridge, GRPCServer, Scheduler gracefully.
	mnemo := &memoclient.NullMnemo{}
	r := &AgentResult{
		Agent:  &agent.Agent{Config: agent.Config{AgentID: "test"}},
		Mnemo:  mnemo,
		Bridge:       nil,
		GRPCServer:    nil,
		Scheduler:     nil,
	}
	// Should not panic
	r.Close()
}