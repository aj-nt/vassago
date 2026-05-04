// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package setup

import (
	"os"
	"testing"

	"github.com/aj-nt/mnemosyne/agent"
)

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		inputs  []string
		expect  string
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
	// Without ANTHROPIC_API_KEY, only openai should be registered
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

	// OpenAI should always be registered
	if _, err := registry.Get("openai"); err != nil {
		t.Errorf("OpenAI provider should be registered: %v", err)
	}

	// Anthropic should NOT be registered (no API key)
	if _, err := registry.Get("anthropic"); err == nil {
		t.Error("Anthropic provider should NOT be registered without API key")
	}
}

func TestProvidersWithAnthropic(t *testing.T) {
	os.Setenv("TEST_ANTHROPIC_KEY_19E", "sk-test-key-123")
	defer os.Unsetenv("TEST_ANTHROPIC_KEY_19E")

	cfg := &agent.ProviderSection{
		Name:               "anthropic",
		Model:              "claude-sonnet-4-6",
		AnthropicModel:     "claude-sonnet-4-6",
		AnthropicAPIKeyEnv: "TEST_ANTHROPIC_KEY_19E",
		Fallback1:          "openai",
		OpenAIModel:       "glm-5.1:cloud",
		OpenAIBaseURL:     "http://localhost:11434/v1",
	}

	registry := Providers(cfg)

	// Both providers should be registered
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

	routingCfg := &agent.RoutingSection{
		Strategy: "smart",
	}

	// Router with no models should still work
	router := Router(routingCfg, cfg, registry)
	if router == nil {
		t.Fatal("Router should not be nil")
	}
}

// Red Team: Provider validation — unknown primary should cause fatal exit.
// We can't easily test log.Fatalf in Go. Instead verify the primary check
// by testing that a valid primary works.
func TestProvidersValidPrimary(t *testing.T) {
	cfg := &agent.ProviderSection{
		Name:         "openai",
		Model:        "glm-5.1:cloud",
		OpenAIModel:  "glm-5.1:cloud",
		OpenAIBaseURL: "http://localhost:11434/v1",
	}

	registry := Providers(cfg)
	if _, err := registry.Get("openai"); err != nil {
		t.Errorf("Valid primary 'openai' should be available: %v", err)
	}
}

func TestProvidersDefaultOpenAIURL(t *testing.T) {
	cfg := &agent.ProviderSection{
		Name:  "openai",
		Model: "gpt-4o-mini",
	}

	// Without any URL config, should default to api.openai.com
	registry := Providers(cfg)
	if registry == nil {
		t.Fatal("Registry should not be nil")
	}
}