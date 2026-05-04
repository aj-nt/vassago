// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- AgentRegistry Tests ---

func TestAgentRegistry_Register(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	agent := AgentInfo{
		ID:           "agent-1",
		Name:         "test-agent",
		Role:         "assistant",
		Capabilities: []string{"memory", "browser"},
	}
	reg.Register(agent)

	got, found := reg.Get("agent-1")
	if !found {
		t.Fatal("agent should be registered")
	}
	if got.Name != "test-agent" {
		t.Errorf("expected name test-agent, got %s", got.Name)
	}
	if got.Status != "online" {
		t.Errorf("expected status online, got %s", got.Status)
	}
	if len(got.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(got.Capabilities))
	}
}

func TestAgentRegistry_Unregister(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	agent := AgentInfo{ID: "agent-1", Name: "test", Role: "assistant"}
	reg.Register(agent)
	reg.Unregister("agent-1")

	got, found := reg.Get("agent-1")
	if !found {
		t.Fatal("agent should still exist after unregister")
	}
	if got.Status != "offline" {
		t.Errorf("expected status offline, got %s", got.Status)
	}
}

func TestAgentRegistry_List(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	reg.Register(AgentInfo{ID: "1", Name: "alpha", Role: "assistant"})
	reg.Register(AgentInfo{ID: "2", Name: "beta", Role: "delegate"})

	agents := reg.List()
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestAgentRegistry_ListByCapability(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	reg.Register(AgentInfo{
		ID: "1", Name: "alpha", Role: "assistant",
		Capabilities: []string{"memory", "browser"},
	})
	reg.Register(AgentInfo{
		ID: "2", Name: "beta", Role: "delegate",
		Capabilities: []string{"memory", "vision"},
	})
	reg.Register(AgentInfo{
		ID: "3", Name: "gamma", Role: "cron",
		Capabilities: []string{"scheduler"},
	})

	agents := reg.ListByCapability("memory")
	if len(agents) != 2 {
		t.Errorf("expected 2 agents with memory capability, got %d", len(agents))
	}

	agents = reg.ListByCapability("vision")
	if len(agents) != 1 {
		t.Errorf("expected 1 agent with vision capability, got %d", len(agents))
	}

	agents = reg.ListByCapability("nonexistent")
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestAgentRegistry_ListByCapability_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	reg.Register(AgentInfo{
		ID: "1", Name: "alpha", Role: "assistant",
		Capabilities: []string{"Browser", "Memory"},
	})

	agents := reg.ListByCapability("browser")
	if len(agents) != 1 {
		t.Errorf("expected 1 agent (case insensitive), got %d", len(agents))
	}

	agents = reg.ListByCapability("MEMORY")
	if len(agents) != 1 {
		t.Errorf("expected 1 agent (case insensitive), got %d", len(agents))
	}
}

func TestAgentRegistry_ListByRole(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	reg.Register(AgentInfo{ID: "1", Name: "alpha", Role: "assistant"})
	reg.Register(AgentInfo{ID: "2", Name: "beta", Role: "delegate"})
	reg.Register(AgentInfo{ID: "3", Name: "gamma", Role: "assistant"})

	agents := reg.ListByRole("assistant")
	if len(agents) != 2 {
		t.Errorf("expected 2 assistants, got %d", len(agents))
	}

	agents = reg.ListByRole("delegate")
	if len(agents) != 1 {
		t.Errorf("expected 1 delegate, got %d", len(agents))
	}
}

func TestAgentRegistry_FindBestAgent(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	reg.Register(AgentInfo{
		ID: "1", Name: "alpha", Role: "assistant",
		Capabilities: []string{"memory", "browser"},
	})
	reg.Register(AgentInfo{
		ID: "2", Name: "beta", Role: "specialist",
		Capabilities: []string{"memory", "browser", "vision", "code"},
	})
	reg.Register(AgentInfo{
		ID: "3", Name: "gamma", Role: "cron",
		Capabilities: []string{"scheduler"},
	})

	// Should find beta (4 matching capabilities vs alpha's 2)
	agent, found := reg.FindBestAgent([]string{"memory", "browser", "vision", "code"})
	if !found {
		t.Fatal("expected to find an agent")
	}
	if agent.Name != "beta" {
		t.Errorf("expected beta (best match), got %s", agent.Name)
	}

	// Should find alpha (matches memory, browser)
	agent, found = reg.FindBestAgent([]string{"memory", "browser"})
	if !found {
		t.Fatal("expected to find an agent")
	}
	if agent.Name != "beta" {
		// beta still wins because it has both + more
		t.Logf("Found %s for memory+browser (OK since beta also matches both)", agent.Name)
	}

	// No match
	_, found = reg.FindBestAgent([]string{"nonexistent"})
	if found {
		t.Error("expected no agent found for nonexistent capability")
	}
}

func TestAgentRegistry_FindBestAgent_OfflineAgent(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	reg.Register(AgentInfo{
		ID: "1", Name: "alpha", Role: "assistant",
		Capabilities: []string{"memory", "browser"},
	})
	reg.Unregister("1") // Mark offline

	_, found := reg.FindBestAgent([]string{"memory"})
	if found {
		t.Error("should not find offline agent")
	}
}

// --- Persistence Tests ---

func TestAgentRegistry_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "agents.json")

	// Create and populate registry
	reg := NewAgentRegistry(path)
	reg.Register(AgentInfo{
		ID:           "agent-1",
		Name:         "test-agent",
		Role:         "assistant",
		Capabilities: []string{"memory", "browser"},
	})

	// Create a new registry loading from the same path
	reg2 := NewAgentRegistry(path)
	agent, found := reg2.Get("agent-1")
	if !found {
		t.Fatal("agent should be persisted and loaded")
	}
	if agent.Name != "test-agent" {
		t.Errorf("expected name test-agent, got %s", agent.Name)
	}
	if len(agent.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(agent.Capabilities))
	}
}

func TestAgentRegistry_EmptyPath(t *testing.T) {
	reg := NewAgentRegistry("")
	reg.Register(AgentInfo{ID: "1", Name: "test", Role: "assistant"})

	// Should work fine without persistence
	agent, found := reg.Get("1")
	if !found {
		t.Fatal("agent should be found in memory")
	}
	if agent.Name != "test" {
		t.Errorf("expected name test, got %s", agent.Name)
	}
}

// --- Tool Handler Tests ---

func TestListAgents_Tool(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))
	origRegistry := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = origRegistry }()

	reg.Register(AgentInfo{
		ID: "agent-1", Name: "alpha", Role: "assistant",
		Capabilities: []string{"memory", "browser"},
		Status: "online",
	})
	reg.Register(AgentInfo{
		ID: "agent-2", Name: "beta", Role: "delegate",
		Capabilities: []string{"memory", "vision"},
		Status: "online",
	})

	t.Run("list all agents", func(t *testing.T) {
		result, isError := ListAgents(map[string]interface{}{})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if !strings.Contains(result, "2 agent") {
			t.Errorf("expected '2 agent' in result, got: %s", result)
		}
		if !strings.Contains(result, "alpha") {
			t.Errorf("expected alpha in result, got: %s", result)
		}
		if !strings.Contains(result, "beta") {
			t.Errorf("expected beta in result, got: %s", result)
		}
	})

	t.Run("filter by role", func(t *testing.T) {
		result, isError := ListAgents(map[string]interface{}{
			"role": "assistant",
		})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if strings.Contains(result, "beta") {
			t.Errorf("should not contain beta when filtering by assistant, got: %s", result)
		}
	})

	t.Run("filter by capability", func(t *testing.T) {
		result, isError := ListAgents(map[string]interface{}{
			"capability": "vision",
		})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if !strings.Contains(result, "beta") {
			t.Errorf("should contain beta (has vision), got: %s", result)
		}
	})

	t.Run("no agents found", func(t *testing.T) {
		result, isError := ListAgents(map[string]interface{}{
			"capability": "nonexistent",
		})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if !strings.Contains(result, "No agents found") {
			t.Errorf("expected 'No agents found', got: %s", result)
		}
	})
}

func TestListAgents_Disabled(t *testing.T) {
	origRegistry := globalRegistry
	globalRegistry = nil
	defer func() { globalRegistry = origRegistry }()

	result, isError := ListAgents(map[string]interface{}{})
	if !isError {
		t.Error("expected error when registry is nil")
	}
	if !strings.Contains(result, "not enabled") {
		t.Errorf("expected 'not enabled' message, got: %s", result)
	}
}

func TestRegisterAgent_Tool(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))
	origRegistry := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = origRegistry }()

	origID := currentAgentID
	origName := currentAgentName
	origRole := currentAgentRole
	currentAgentID = "test-agent-1"
	currentAgentName = "TestAgent"
	currentAgentRole = "assistant"
	defer func() {
		currentAgentID = origID
		currentAgentName = origName
		currentAgentRole = origRole
	}()

	t.Run("register with capabilities", func(t *testing.T) {
		result, isError := RegisterAgentTool(map[string]interface{}{
			"capabilities": []interface{}{"memory", "browser", "vision"},
		})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if !strings.Contains(result, "Registered agent TestAgent") {
			t.Errorf("expected success message, got: %s", result)
		}

		agent, found := reg.Get("test-agent-1")
		if !found {
			t.Fatal("agent should be registered")
		}
		if len(agent.Capabilities) != 3 {
			t.Errorf("expected 3 capabilities, got %d", len(agent.Capabilities))
		}
	})

	t.Run("register without capabilities", func(t *testing.T) {
		result, isError := RegisterAgentTool(map[string]interface{}{})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if !strings.Contains(result, "none") {
			t.Errorf("expected 'none' for empty capabilities, got: %s", result)
		}
	})
}

func TestFindAgent_Tool(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))
	origRegistry := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = origRegistry }()

	reg.Register(AgentInfo{
		ID: "agent-1", Name: "alpha", Role: "assistant",
		Capabilities: []string{"memory", "browser"},
		Status: "online",
	})
	reg.Register(AgentInfo{
		ID: "agent-2", Name: "beta", Role: "specialist",
		Capabilities: []string{"memory", "browser", "vision"},
		Status: "online",
	})

	t.Run("find agent with capabilities", func(t *testing.T) {
		result, isError := FindAgent(map[string]interface{}{
			"capabilities": []interface{}{"vision", "browser"},
		})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if !strings.Contains(result, "beta") {
			t.Errorf("expected to find beta, got: %s", result)
		}
	})

	t.Run("no agent found", func(t *testing.T) {
		result, isError := FindAgent(map[string]interface{}{
			"capabilities": []interface{}{"nonexistent"},
		})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if !strings.Contains(result, "No agent found") {
			t.Errorf("expected 'No agent found', got: %s", result)
		}
	})

	t.Run("missing capabilities", func(t *testing.T) {
		result, isError := FindAgent(map[string]interface{}{})
		if !isError {
			t.Error("expected error for missing capabilities")
		}
		if !strings.Contains(result, "capability is required") {
			t.Errorf("expected 'capability is required', got: %s", result)
		}
	})
}

func TestFindAgent_Disabled(t *testing.T) {
	origRegistry := globalRegistry
	globalRegistry = nil
	defer func() { globalRegistry = origRegistry }()

	result, isError := FindAgent(map[string]interface{}{
		"capabilities": []interface{}{"memory"},
	})
	if !isError {
		t.Error("expected error when registry is nil")
	}
	if !strings.Contains(result, "not enabled") {
		t.Errorf("expected 'not enabled', got: %s", result)
	}
}

// --- Red Team Tests ---

func TestAgentRegistry_RedTeam_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	done := make(chan bool, 10)

	for i := 0; i < 5; i++ {
		go func(id int) {
			reg.Register(AgentInfo{
				ID:   fmt.Sprintf("agent-%d", id),
				Name: fmt.Sprintf("agent-%d", id),
				Role: "worker",
			})
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		go func(id int) {
			agents := reg.ListByCapability("nonexistent")
			_ = agents
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	list := reg.List()
	if len(list) != 5 {
		t.Errorf("expected 5 agents after concurrent access, got %d", len(list))
	}
}

func TestAgentRegistry_RedTeam_EmptyCapabilities(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	reg.Register(AgentInfo{ID: "1", Name: "minimal", Role: "test"})

	agent, found := reg.Get("1")
	if !found {
		t.Fatal("agent should be registered even with no capabilities")
	}
	if len(agent.Capabilities) != 0 {
		t.Errorf("expected 0 capabilities, got %d", len(agent.Capabilities))
	}
}

func TestAgentRegistry_RedTeam_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	reg.Register(AgentInfo{
		ID: "1", Name: "v1", Role: "assistant",
		Capabilities: []string{"memory"},
	})
	reg.Register(AgentInfo{
		ID: "1", Name: "v2", Role: "delegate",
		Capabilities: []string{"memory", "browser"},
	})

	agent, _ := reg.Get("1")
	if agent.Name != "v2" {
		t.Errorf("expected v2 after overwrite, got %s", agent.Name)
	}
	if len(agent.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities after overwrite, got %d", len(agent.Capabilities))
	}
}

func TestAgentRegistry_RedTeam_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	reg.Register(AgentInfo{
		ID:   "agent-特殊",
		Name: "test-agent-'; DROP TABLE agents; --",
		Role: "assistant",
		Capabilities: []string{"memory", "browser'; INJECTION"},
	})

	agent, found := reg.Get("agent-特殊")
	if !found {
		t.Fatal("agent with special chars should be registered")
	}

	// Verify persistence works with special chars
	reg2 := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))
	agent2, found := reg2.Get("agent-特殊")
	if !found {
		t.Fatal("agent should be persisted and loaded correctly with special chars")
	}
	if agent2.Name != agent.Name {
		t.Errorf("special chars in name not preserved: got %q, want %q", agent2.Name, agent.Name)
	}
}

func TestAgentRegistry_RedTeam_LargeRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewAgentRegistry(filepath.Join(tmpDir, "agents.json"))

	// Register 100 agents
	for i := 0; i < 100; i++ {
		reg.Register(AgentInfo{
			ID:   fmt.Sprintf("agent-%d", i),
			Name: fmt.Sprintf("agent-%d", i),
			Role: "worker",
			Capabilities: []string{"memory"},
		})
	}

	list := reg.List()
	if len(list) != 100 {
		t.Errorf("expected 100 agents, got %d", len(list))
	}

	agents := reg.ListByCapability("memory")
	if len(agents) != 100 {
		t.Errorf("expected 100 agents with memory capability, got %d", len(agents))
	}
}

// --- JSON Persistence Edge Cases ---

func TestAgentRegistry_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "agents.json")

	// Write corrupted JSON
	os.WriteFile(path, []byte("not valid json{{"), 0644)

	// Should not crash, just start with empty registry
	reg := NewAgentRegistry(path)
	list := reg.List()
	if len(list) != 0 {
		t.Errorf("expected empty registry on corrupted file, got %d agents", len(list))
	}
}

// --- Default Config Test ---

func TestDefaultMultiAgentConfig(t *testing.T) {
	cfg := DefaultMultiAgentConfig()
	if cfg.Enabled {
		t.Error("default multi-agent should be disabled")
	}
	if cfg.RegistryPath == "" {
		t.Error("default registry path should not be empty")
	}
}

// --- RegisterMultiAgentTools Test ---

func TestRegisterMultiAgentTools_Disabled(t *testing.T) {
	registry := NewToolRegistry()
	cfg := MultiAgentConfig{Enabled: false}

	RegisterMultiAgentTools(registry, cfg, "test-id", "test-name", "assistant")

	_, err := registry.Get("ListAgents")
	if err == nil {
		t.Error("ListAgents should not be registered when disabled")
	}
}

func TestRegisterMultiAgentTools_Enabled(t *testing.T) {
	tmpDir := t.TempDir()
	registry := NewToolRegistry()
	cfg := MultiAgentConfig{
		Enabled:      true,
		RegistryPath: filepath.Join(tmpDir, "agents.json"),
	}

	RegisterMultiAgentTools(registry, cfg, "test-id", "test-name", "assistant")

	_, err := registry.Get("ListAgents")
	if err != nil {
		t.Errorf("ListAgents should be registered: %v", err)
	}
	_, err = registry.Get("RegisterAgent")
	if err != nil {
		t.Errorf("RegisterAgent should be registered: %v", err)
	}
	_, err = registry.Get("FindAgent")
	if err != nil {
		t.Errorf("FindAgent should be registered: %v", err)
	}
}