// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"testing"
)

// mockSubAgentRunner implements SubAgentRunner for testing.
type mockSubAgentRunner struct {
	result *SubAgentResult
	err    error
	calls  int
}

func (m *mockSubAgentRunner) RunSubAgent(prompt string, agentID string, maxTurns int) (*SubAgentResult, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestDelegateTool_RequiresPrompt(t *testing.T) {
	runner := &mockSubAgentRunner{}
	reg := NewToolRegistry()
	RegisterDelegationTool(reg, "test-agent", runner, DelegateConfig{Enabled: true, MaxDepth: 1, MaxTurns: 10})

	handler, err := reg.Get("Delegate")
	if err != nil {
		t.Fatalf("Delegate tool not registered: %v", err)
	}

	result, isErr := handler(map[string]interface{}{})
	if !isErr {
		t.Error("Expected error for missing prompt")
	}
	if result != "prompt is required" {
		t.Errorf("Expected 'prompt is required', got %q", result)
	}
}

func TestDelegateTool_Disabled(t *testing.T) {
	runner := &mockSubAgentRunner{}
	reg := NewToolRegistry()
	RegisterDelegationTool(reg, "test-agent", runner, DelegateConfig{Enabled: false})

	handler, err := reg.Get("Delegate")
	if err != nil {
		t.Fatalf("Delegate tool not registered: %v", err)
	}

	result, isErr := handler(map[string]interface{}{"prompt": "do something"})
	if !isErr {
		t.Error("Expected error when delegation is disabled")
	}
	if result != "delegation is disabled in agent config" {
		t.Errorf("Expected disabled message, got %q", result)
	}

	if runner.calls != 0 {
		t.Error("Runner should not have been called when disabled")
	}
}

func TestDelegateTool_Success(t *testing.T) {
	runner := &mockSubAgentRunner{
		result: &SubAgentResult{Text: "Research complete: found 3 items", IsError: false, Iterations: 2},
	}
	reg := NewToolRegistry()
	RegisterDelegationTool(reg, "test-agent", runner, DelegateConfig{Enabled: true, MaxDepth: 1, MaxTurns: 10})

	handler, err := reg.Get("Delegate")
	if err != nil {
		t.Fatalf("Delegate tool not registered: %v", err)
	}

	result, isErr := handler(map[string]interface{}{"prompt": "research X"})
	if isErr {
		t.Errorf("Expected success, got error: %s", result)
	}
	if result != "Research complete: found 3 items" {
		t.Errorf("Expected sub-agent result, got %q", result)
	}
	if runner.calls != 1 {
		t.Errorf("Expected 1 runner call, got %d", runner.calls)
	}
}

func TestDelegateTool_SubAgentError(t *testing.T) {
	runner := &mockSubAgentRunner{
		result: &SubAgentResult{Text: "sub-agent error: timeout", IsError: true},
	}
	reg := NewToolRegistry()
	RegisterDelegationTool(reg, "test-agent", runner, DelegateConfig{Enabled: true, MaxDepth: 1, MaxTurns: 10})

	handler, err := reg.Get("Delegate")
	if err != nil {
		t.Fatalf("Delegate tool not registered: %v", err)
	}

	result, isErr := handler(map[string]interface{}{"prompt": "research X"})
	if !isErr {
		t.Error("Expected isErr=true when sub-agent result has IsError=true")
	}
	if result != "sub-agent error: timeout" {
		t.Errorf("Expected error text, got %q", result)
	}
}

func TestDelegateTool_EmptyResponse(t *testing.T) {
	runner := &mockSubAgentRunner{
		result: &SubAgentResult{Text: "", IsError: false},
	}
	reg := NewToolRegistry()
	RegisterDelegationTool(reg, "test-agent", runner, DelegateConfig{Enabled: true, MaxDepth: 1, MaxTurns: 10})

	handler, err := reg.Get("Delegate")
	if err != nil {
		t.Fatalf("Delegate tool not registered: %v", err)
	}

	result, isErr := handler(map[string]interface{}{"prompt": "research X"})
	if isErr {
		t.Errorf("Empty response shouldn't be an error, got: %s", result)
	}
	if result != "Delegate sub-agent returned no response" {
		t.Errorf("Expected no-response message, got %q", result)
	}
}

func TestDelegateSchema(t *testing.T) {
	if DelegateSchema["name"] != "Delegate" {
		t.Errorf("Expected name=Delegate, got %v", DelegateSchema["name"])
	}
	schema, ok := DelegateSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters is not a map")
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not []string")
	}
	if len(required) != 1 || required[0] != "prompt" {
		t.Errorf("Expected required=['prompt'], got %v", required)
	}
}

func TestDefaultDelegateConfig(t *testing.T) {
	cfg := DefaultDelegateConfig()
	if !cfg.Enabled {
		t.Error("Expected Enabled=true")
	}
	if cfg.MaxDepth != 1 {
		t.Errorf("Expected MaxDepth=1, got %d", cfg.MaxDepth)
	}
	if cfg.MaxTurns != 10 {
		t.Errorf("Expected MaxTurns=10, got %d", cfg.MaxTurns)
	}
}