// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aj-nt/mnemosyne/agent/provider"
)

// --- applyDefaults() tests ---

func TestApplyDefaults_SetsMaxTurns(t *testing.T) {
	cfg := &AgentConfig{}
	cfg.applyDefaults()
	if cfg.Agent.MaxTurns != 50 {
		t.Errorf("default MaxTurns = %d, want 50", cfg.Agent.MaxTurns)
	}
}

func TestApplyDefaults_PreservesExplicitMaxTurns(t *testing.T) {
	cfg := &AgentConfig{Agent: AgentSection{MaxTurns: 100}}
	cfg.applyDefaults()
	if cfg.Agent.MaxTurns != 100 {
		t.Errorf("explicit MaxTurns = %d, want 100", cfg.Agent.MaxTurns)
	}
}

func TestApplyDefaults_SetsTemperature(t *testing.T) {
	cfg := &AgentConfig{}
	cfg.applyDefaults()
	if cfg.Provider.Temperature != 0.7 {
		t.Errorf("default Temperature = %f, want 0.7", cfg.Provider.Temperature)
	}
}

func TestApplyDefaults_SetsHotBlockDefaults(t *testing.T) {
	cfg := &AgentConfig{}
	cfg.applyDefaults()
	if cfg.HotBlock.MemoryCharLimit != 4000 {
		t.Errorf("default MemoryCharLimit = %d, want 4000", cfg.HotBlock.MemoryCharLimit)
	}
	if cfg.HotBlock.UserCharLimit != 4000 {
		t.Errorf("default UserCharLimit = %d, want 4000", cfg.HotBlock.UserCharLimit)
	}
	if cfg.HotBlock.MinPriority != 2 {
		t.Errorf("default MinPriority = %d, want 2", cfg.HotBlock.MinPriority)
	}
}

func TestApplyDefaults_SetsDelegationDefaults(t *testing.T) {
	cfg := &AgentConfig{}
	cfg.applyDefaults()
	if cfg.Delegation.MaxDepth != 1 {
		t.Errorf("default MaxDepth = %d, want 1", cfg.Delegation.MaxDepth)
	}
	if cfg.Delegation.MaxTurns != 10 {
		t.Errorf("default MaxTurns = %d, want 10", cfg.Delegation.MaxTurns)
	}
}

func TestApplyDefaults_SetsSchedulerInterval(t *testing.T) {
	cfg := &AgentConfig{}
	cfg.applyDefaults()
	if cfg.Scheduler.Interval != 3600 {
		t.Errorf("default Scheduler.Interval = %d, want 3600", cfg.Scheduler.Interval)
	}
}

func TestApplyDefaults_BrowserHeadless(t *testing.T) {
	cfg := &AgentConfig{Browser: BrowserSection{Enabled: true}}
	cfg.applyDefaults()
	if !cfg.Browser.Headless {
		t.Error("Browser should default to headless when enabled")
	}
}

func TestApplyDefaults_BrowserExplicitHeadless(t *testing.T) {
	cfg := &AgentConfig{Browser: BrowserSection{Enabled: true, Headless: false}}
	cfg.applyDefaults()
	// When explicitly set to false, this should NOT be overridden
	// (the zero value of Headless matches "false", so this tests that
	// we don't overwrite an explicit false when Browser.Enabled is also false)
	// Actually: the code checks !c.Browser.Headless, so it forces headless.
	// If the user explicitly sets Headless: false with Enabled: true, the
	// default overrides it. That's the current behavior.
	if cfg.Browser.Headless != true {
		t.Error("Browser with Enabled=true and Headless=false should still default to headless")
	}
}

func TestApplyDefaults_BrowserDisabled(t *testing.T) {
	cfg := &AgentConfig{Browser: BrowserSection{Enabled: false}}
	cfg.applyDefaults()
	if cfg.Browser.Headless {
		t.Error("Browser should NOT default to headless when disabled")
	}
}

func TestApplyDefaults_VisionDefaults(t *testing.T) {
	cfg := &AgentConfig{}
	cfg.applyDefaults()
	if cfg.Vision.MaxImageSize != 20*1024*1024 {
		t.Errorf("default MaxImageSize = %d, want %d", cfg.Vision.MaxImageSize, 20*1024*1024)
	}
}

func TestApplyDefaults_ImageGenDefaults(t *testing.T) {
	cfg := &AgentConfig{}
	cfg.applyDefaults()
	if cfg.ImageGen.Provider != "openai" {
		t.Errorf("default ImageGen.Provider = %q, want %q", cfg.ImageGen.Provider, "openai")
	}
	if cfg.ImageGen.Model != "dall-e-3" {
		t.Errorf("default ImageGen.Model = %q, want %q", cfg.ImageGen.Model, "dall-e-3")
	}
	if cfg.ImageGen.MaxPromptLength != 1000 {
		t.Errorf("default ImageGen.MaxPromptLength = %d, want 1000", cfg.ImageGen.MaxPromptLength)
	}
	if cfg.ImageGen.APIKeyEnv != "OPENAI_API_KEY" {
		t.Errorf("default ImageGen.APIKeyEnv = %q, want %q", cfg.ImageGen.APIKeyEnv, "OPENAI_API_KEY")
	}
}

func TestApplyDefaults_BackgroundProcessDefaults(t *testing.T) {
	cfg := &AgentConfig{}
	cfg.applyDefaults()
	if cfg.BackgroundProcess.MaxProcesses != 5 {
		t.Errorf("default MaxProcesses = %d, want 5", cfg.BackgroundProcess.MaxProcesses)
	}
	if cfg.BackgroundProcess.MaxOutputKB != 64 {
		t.Errorf("default MaxOutputKB = %d, want 64", cfg.BackgroundProcess.MaxOutputKB)
	}
	if cfg.BackgroundProcess.MaxAgeMin != 30 {
		t.Errorf("default MaxAgeMin = %d, want 30", cfg.BackgroundProcess.MaxAgeMin)
	}
}

func TestApplyDefaults_CodeExecDefaults(t *testing.T) {
	cfg := &AgentConfig{}
	cfg.applyDefaults()
	if cfg.CodeExec.Timeout != 300 {
		t.Errorf("default CodeExec.Timeout = %d, want 300", cfg.CodeExec.Timeout)
	}
	if cfg.CodeExec.MaxOutputKB != 50 {
		t.Errorf("default CodeExec.MaxOutputKB = %d, want 50", cfg.CodeExec.MaxOutputKB)
	}
	if cfg.CodeExec.MaxCodeChars != 50000 {
		t.Errorf("default CodeExec.MaxCodeChars = %d, want 50000", cfg.CodeExec.MaxCodeChars)
	}
}

func TestApplyDefaults_MultiAgentRegistryPath(t *testing.T) {
	cfg := &AgentConfig{MultiAgent: MultiAgentSection{Enabled: true}}
	cfg.applyDefaults()
	if cfg.MultiAgent.RegistryPath == "" {
		t.Error("MultiAgent.RegistryPath should be set when Enabled=true")
	}
	// Should end with .mnemosyne/agents.json
	if filepath.Base(cfg.MultiAgent.RegistryPath) != "agents.json" {
		t.Errorf("MultiAgent.RegistryPath base = %q, want agents.json", filepath.Base(cfg.MultiAgent.RegistryPath))
	}
}

func TestApplyDefaults_MultiAgentDisabled(t *testing.T) {
	cfg := &AgentConfig{MultiAgent: MultiAgentSection{Enabled: false}}
	cfg.applyDefaults()
	if cfg.MultiAgent.RegistryPath != "" {
		t.Error("MultiAgent.RegistryPath should not be set when Enabled=false")
	}
}

// --- validateTools() tests ---

func TestValidateTools_EmptyEnabled(t *testing.T) {
	cfg := &AgentConfig{Tools: ToolsSection{Enabled: nil}}
	cfg.validateTools() // Should not panic or produce warnings
}

func TestValidateTools_ValidTools(t *testing.T) {
	cfg := &AgentConfig{Tools: ToolsSection{Enabled: []string{"AddMemory", "SearchMemories", "Execute"}}}
	// Should not produce warnings for valid tool names
	cfg.validateTools()
}

func TestValidateTools_UnknownTool(t *testing.T) {
	cfg := &AgentConfig{Tools: ToolsSection{Enabled: []string{"FakeTool", "AddMemory"}}}
	// Should produce a warning to stderr for FakeTool, but not error
	// Testing side effect (stderr output) is hard without capturing,
	// but we verify it doesn't panic or modify the list
	cfg.validateTools()
	if len(cfg.Tools.Enabled) != 2 {
		t.Errorf("validateTools should not remove unknown tools, got %d", len(cfg.Tools.Enabled))
	}
}

// --- loadPersonality() tests (deprecated - personality file feature removed) ---

// These tests are intentionally removed as part of the personality file feature removal.
// Personality is now always a string from config.yaml (can be multi-line).

// --- LoadConfig() tests ---

func TestLoadConfig_ValidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
agent:
  id: test-agent
  name: Test
  role: assistant
  max_turns: 50
provider:
  name: openai
  model: glm-5.1:cloud
mnemosyne:
  address: "localhost:50051"
hot_block:
  memory_char_limit: 4000
  user_char_limit: 4000
  min_priority: 2
`
	configFile := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Agent.ID != "test-agent" {
		t.Errorf("Agent.ID = %q, want %q", cfg.Agent.ID, "test-agent")
	}
	if cfg.Provider.Name != "openai" {
		t.Errorf("Provider.Name = %q, want %q", cfg.Provider.Name, "openai")
	}
}

func TestLoadConfig_AppliesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
agent:
  id: test-agent
  name: Test
  role: assistant
provider:
  name: openai
  model: glm-5.1:cloud
mnemosyne:
  address: "localhost:50051"
`
	configFile := filepath.Join(tmpDir, "defaults.yaml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	// Check that defaults were applied
	if cfg.Agent.MaxTurns != 50 {
		t.Errorf("default MaxTurns = %d, want 50", cfg.Agent.MaxTurns)
	}
	if cfg.Provider.Temperature != 0.7 {
		t.Errorf("default Temperature = %f, want 0.7", cfg.Provider.Temperature)
	}
	if cfg.HotBlock.MemoryCharLimit != 4000 {
		t.Errorf("default MemoryCharLimit = %d, want 4000", cfg.HotBlock.MemoryCharLimit)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("LoadConfig should return error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
agent:
  id: test
  name: [
invalid yaml here
`
	configFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configFile)
	if err == nil {
		t.Error("LoadConfig should return error for invalid YAML")
	}
}

func TestLoadConfig_ValidationErrors(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing agent id",
			yaml: `
agent:
  name: Test
provider:
  name: openai
  model: test
mnemosyne:
  address: "localhost:50051"
`,
			wantErr: "agent.id is required",
		},
		{
			name: "missing provider name",
			yaml: `
agent:
  id: test
provider:
  model: test
mnemosyne:
  address: "localhost:50051"
`,
			wantErr: "provider.name is required",
		},
		{
			name: "missing provider model",
			yaml: `
agent:
  id: test
provider:
  name: openai
mnemosyne:
  address: "localhost:50051"
`,
			wantErr: "provider.model is required",
		},
		{
			name: "invalid routing strategy",
			yaml: `
agent:
  id: test
provider:
  name: openai
  model: test
  temperature: 0.7
mnemosyne:
  address: "localhost:50051"
hot_block:
  memory_char_limit: 4000
  user_char_limit: 4000
  min_priority: 2
routing:
  strategy: "invalid"
`,
			wantErr: "routing.strategy must be one of",
		},
		{
			name: "temperature too high",
			yaml: `
agent:
  id: test
provider:
  name: openai
  model: test
  temperature: 3.0
mnemosyne:
  address: "localhost:50051"
`,
			wantErr: "provider.temperature must be between 0 and 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFile := filepath.Join(tmpDir, tt.name+".yaml")
			if err := os.WriteFile(configFile, []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(configFile)

			_, err := LoadConfig(configFile)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
			} else if !contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// TestLoadConfig_PersonalityStringOnly verifies personality comes from config string
func TestLoadConfig_PersonalityStringOnly(t *testing.T) {
	tmpDir := t.TempDir()
	personalityContent := "You are a creative writer."

	yamlContent := `
agent:
  id: test-agent
  name: Test
  role: assistant
  personality: "` + personalityContent + `"
provider:
  name: openai
  model: glm-5.1:cloud
  temperature: 0.7
mnemosyne:
  address: "localhost:50051"
hot_block:
  memory_char_limit: 4000
  user_char_limit: 4000
  min_priority: 2
`
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Agent.Personality != personalityContent {
		t.Errorf("Personality = %q, want %q", cfg.Agent.Personality, personalityContent)
	}
}



// --- Validate() tests ---

func TestValidate_ValidConfig(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass validation: %v", err)
	}
}

func TestValidate_MissingAgentID(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.ID = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing agent id")
	}
}

func TestValidate_MaxTurnsTooLow(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.MaxTurns = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for max_turns < 1")
	}
}

func TestValidate_MaxTurnsTooHigh(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.MaxTurns = 300
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for max_turns > 200")
	}
}

func TestValidate_TemperatureTooHigh(t *testing.T) {
	cfg := validConfig()
	cfg.Provider.Temperature = 2.5
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for temperature > 2")
	}
}

func TestValidate_TemperatureNegative(t *testing.T) {
	cfg := validConfig()
	cfg.Provider.Temperature = -0.5
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative temperature")
	}
}

func TestValidate_HotBlockLimits(t *testing.T) {
	cfg := validConfig()
	cfg.HotBlock.MemoryCharLimit = 500 // too low
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for memory_char_limit too low")
	}

	cfg = validConfig()
	cfg.HotBlock.MemoryCharLimit = 50000 // too high
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for memory_char_limit too high")
	}

	cfg = validConfig()
	cfg.HotBlock.UserCharLimit = 100 // too low
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for user_char_limit too low")
	}

	cfg = validConfig()
	cfg.HotBlock.MinPriority = 0 // too low
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for min_priority too low")
	}
}

func TestValidate_DelegationLimits(t *testing.T) {
	cfg := validConfig()
	cfg.Delegation.MaxDepth = 0 // too low
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for max_depth too low")
	}

	cfg = validConfig()
	cfg.Delegation.MaxDepth = 10 // too high
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for max_depth too high")
	}

	cfg = validConfig()
	cfg.Delegation.MaxTurns = 0 // too low
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for max_turns too low")
	}
}

func TestValidate_InvalidRoutingStrategy(t *testing.T) {
	cfg := validConfig()
	cfg.Routing.Strategy = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid routing strategy")
	}
}

func TestValidate_ValidRoutingStrategies(t *testing.T) {
	for _, strategy := range []string{"always-primary", "cost-optimized", "quality-optimized", "smart"} {
		cfg := validConfig()
		cfg.Routing.Strategy = strategy
		if err := cfg.Validate(); err != nil {
			t.Errorf("routing strategy %q should be valid: %v", strategy, err)
		}
	}
}

func TestValidate_VisionLimits(t *testing.T) {
	cfg := validConfig()
	cfg.Vision.Enabled = true
	cfg.Vision.MaxImageSize = 1024 // too low (less than 1MB)
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for MaxImageSize too low")
	}

	cfg = validConfig()
	cfg.Vision.MaxImageSize = 200 * 1024 * 1024 // too high (> 100MB)
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for MaxImageSize too high")
	}
}

func TestValidate_ImageGenConfig(t *testing.T) {
	cfg := validConfig()
	cfg.ImageGen.Enabled = true
	cfg.ImageGen.Provider = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid image_gen provider")
	}

	cfg = validConfig()
	cfg.ImageGen.Enabled = true
	cfg.ImageGen.MaxPromptLength = 10 // too low
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for max_prompt_length too low")
	}
}

func TestValidate_CodeExecConfig(t *testing.T) {
	cfg := validConfig()
	cfg.CodeExec.Enabled = true
	cfg.CodeExec.Timeout = 1 // too low
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for code_exec timeout too low")
	}

	cfg = validConfig()
	cfg.CodeExec.Enabled = true
	cfg.CodeExec.MaxOutputKB = 0 // too low
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for code_exec max_output_kb too low")
	}

	cfg = validConfig()
	cfg.CodeExec.Enabled = true
	cfg.CodeExec.MaxCodeChars = 100 // too low
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for code_exec max_code_chars too low")
	}
}

func TestValidate_EmptyRoutingStrategy(t *testing.T) {
	cfg := validConfig()
	cfg.Routing.Strategy = ""
	// Empty strategy should be valid (means "use default routing")
	if err := cfg.Validate(); err != nil {
		t.Errorf("empty routing strategy should be valid: %v", err)
	}
}

// --- ToAgentConfig() tests ---

func TestToAgentConfig_ContextWindowFromModels(t *testing.T) {
	cfg := validConfig()
	cfg.Routing.Models = []provider.ModelConfig{
		{Name: "small", Provider: "openai", ContextWindow: 8000},
		{Name: "large", Provider: "openai", ContextWindow: 128000},
	}
	agentCfg := cfg.ToAgentConfig()
	// Should use the smallest context window
	if agentCfg.MaxContextTokens != 8000 {
		t.Errorf("MaxContextTokens = %d, want 8000 (smallest model)", agentCfg.MaxContextTokens)
	}
}

func TestToAgentConfig_DefaultContextWindow(t *testing.T) {
	cfg := validConfig()
	// No models = default context window
	agentCfg := cfg.ToAgentConfig()
	if agentCfg.MaxContextTokens != 128000 {
		t.Errorf("default MaxContextTokens = %d, want 128000", agentCfg.MaxContextTokens)
	}
}

func TestToAgentConfig_Fields(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.ID = "test-id"
	cfg.Agent.Name = "TestName"
	cfg.Agent.Role = "assistant"
	cfg.Agent.Personality = "friendly"
	cfg.Provider.Temperature = 0.5

	agentCfg := cfg.ToAgentConfig()
	if agentCfg.AgentID != "test-id" {
		t.Errorf("AgentID = %q, want %q", agentCfg.AgentID, "test-id")
	}
	if agentCfg.AgentName != "TestName" {
		t.Errorf("AgentName = %q, want %q", agentCfg.AgentName, "TestName")
	}
	if agentCfg.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5", agentCfg.Temperature)
	}
}

// TestRequestPrepStage_MultiLinePersonalityFromConfig verifies multi-line personality in YAML works
func TestRequestPrepStage_MultiLinePersonalityFromConfig(t *testing.T) {
	cfg := validConfig()
	multiLinePersonality := `You are a helpful assistant.
You like to help people.
You are concise and direct.`
	cfg.Agent.Personality = multiLinePersonality

	agentCfg := cfg.ToAgentConfig()
	if agentCfg.Personality != multiLinePersonality {
		t.Errorf("Personality = %q, want %q", agentCfg.Personality, multiLinePersonality)
	}
}

// --- helper functions ---

func validConfig() *AgentConfig {
	return &AgentConfig{
		Agent: AgentSection{
			ID:       "test",
			Name:     "Test",
			Role:     "assistant",
			MaxTurns: 50,
		},
		Provider: ProviderSection{
			Name:        "openai",
			Model:       "glm-5.1:cloud",
			Temperature: 0.7,
		},
		Mnemosyne: MnemosyneSection{
			Address: "localhost:50051",
		},
		HotBlock: HotBlockSection{
			MemoryCharLimit: 4000,
			UserCharLimit:   4000,
			MinPriority:     2,
		},
		Delegation: DelegationSection{
			MaxDepth: 1,
			MaxTurns: 10,
		},
		Scheduler: SchedulerSection{
			Interval: 3600,
		},
		Vision: VisionSection{
			MaxImageSize: 20 * 1024 * 1024,
		},
		ImageGen: ImageGenSection{
			Provider:        "openai",
			Model:           "dall-e-3",
			MaxPromptLength: 1000,
			APIKeyEnv:       "OPENAI_API_KEY",
		},
		BackgroundProcess: BackgroundProcessSection{
			MaxProcesses: 5,
			MaxOutputKB:  64,
			MaxAgeMin:    30,
		},
		CodeExec: CodeExecSection{
			Timeout:      300,
			MaxOutputKB:  50,
			MaxCodeChars: 50000,
		},
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}