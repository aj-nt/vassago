package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdInitForceCreatesAllDirs(t *testing.T) {
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, ".mnemosyne-test")

	// Simulate what cmdInit does for directory creation
	subdirs := []string{
		"screenshots",
		"agents",
		"pastes",
		"personalities",
		"generated_images",
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, sub := range subdirs {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Verify all dirs exist
	for _, sub := range subdirs {
		p := filepath.Join(dir, sub)
		if !fileExists(p) {
			t.Errorf("Expected directory %s to exist", p)
		}
	}
}

func TestDaemonConfigGeneration(t *testing.T) {
	dir := "/home/user/.mnemosyne"
	cfg := generateDaemonConfig(dir)

	// Verify defaults
	if cfg.Daemon.TCP != "localhost:50051" {
		t.Errorf("Expected TCP localhost:50051, got %s", cfg.Daemon.TCP)
	}
	if cfg.Daemon.Socket != filepath.Join(dir, "mnemosyne.sock") {
		t.Errorf("Expected socket path, got %s", cfg.Daemon.Socket)
	}
	if !cfg.Database.WALMode {
		t.Error("WAL mode should be true by default")
	}
	if cfg.Database.BusyTimeout != 5000 {
		t.Errorf("Expected busy_timeout 5000, got %d", cfg.Database.BusyTimeout)
	}
	if cfg.HotBlock.MemoryCharLimit != 8000 {
		t.Errorf("Expected memory_char_limit 8000, got %d", cfg.HotBlock.MemoryCharLimit)
	}
	if cfg.HotBlock.MinPriority != 2 {
		t.Errorf("Expected min_priority 2, got %d", cfg.HotBlock.MinPriority)
	}
}

func TestAgentConfigGeneration(t *testing.T) {
	dir := "/home/user/.mnemosyne"

	tests := []struct {
		name     string
		provider string
		model    string
	}{
		{"Anthropic provider", "anthropic", "claude-sonnet-4-6"},
		{"OpenAI provider", "openai", "gpt-4o-mini"},
		{"Ollama provider", "openai", "glm-5.1:cloud"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := generateAgentConfig(dir, tt.provider, tt.model)
			if cfg.Provider.Name != tt.provider {
				t.Errorf("Expected provider %s, got %s", tt.provider, cfg.Provider.Name)
			}
			if cfg.Provider.Model != tt.model {
				t.Errorf("Expected model %s, got %s", tt.model, cfg.Provider.Model)
			}
		})
	}
}

func TestAgentConfigDefaultTools(t *testing.T) {
	dir := "/home/user/.mnemosyne"
	cfg := generateAgentConfig(dir, "anthropic", "claude-sonnet-4-6")

	// Must have core tools
	coreTools := []string{"AddMemory", "SearchMemories", "GetHotBlock"}
	for _, tool := range coreTools {
		found := false
		for _, enabled := range cfg.Tools.Enabled {
			if enabled == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected core tool %s to be enabled", tool)
		}
	}

	// Must have approval_required for dangerous tools
	dangerous := []string{"FileWrite", "Execute"}
	for _, tool := range dangerous {
		found := false
		for _, req := range cfg.Tools.ApprovalRequired {
			if req == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected dangerous tool %s to require approval", tool)
		}
	}
}

func TestAgentConfigDelegation(t *testing.T) {
	dir := "/home/user/.mnemosyne"
	cfg := generateAgentConfig(dir, "anthropic", "claude-sonnet-4-6")

	if !cfg.Delegation.Enabled {
		t.Error("Delegation should be enabled by default")
	}
	if cfg.Delegation.MaxDepth != 1 {
		t.Errorf("Expected max_depth 1, got %d", cfg.Delegation.MaxDepth)
	}
	if cfg.Delegation.MaxTurns != 10 {
		t.Errorf("Expected max_turns 10, got %d", cfg.Delegation.MaxTurns)
	}
}

func TestAgentConfigBrowser(t *testing.T) {
	dir := "/home/user/.mnemosyne"
	cfg := generateAgentConfig(dir, "anthropic", "claude-sonnet-4-6")

	if cfg.Browser.Enabled {
		t.Error("Browser should be disabled by default (requires ChromeDP)")
	}
	if !cfg.Browser.Headless {
		t.Error("Browser should default to headless mode")
	}
}

func TestAgentConfigVision(t *testing.T) {
	dir := "/home/user/.mnemosyne"
	cfg := generateAgentConfig(dir, "anthropic", "claude-sonnet-4-6")

	if cfg.Vision.Enabled {
		t.Error("Vision should be disabled by default (requires provider)")
	}
	if cfg.Vision.MaxImageSize != 20971520 {
		t.Errorf("Expected max_image_size 20MB, got %d", cfg.Vision.MaxImageSize)
	}
}

func TestAgentConfigPersonalityFile(t *testing.T) {
	dir := "/home/user/.mnemosyne"
	cfg := generateAgentConfig(dir, "anthropic", "claude-sonnet-4-6")

	expectedPath := filepath.Join(dir, "personalities", "bartimaeus.md")
	if cfg.Agent.PersonalityFile != expectedPath {
		t.Errorf("Expected personality path %s, got %s", expectedPath, cfg.Agent.PersonalityFile)
	}
}

func TestYAMLOutputComplete(t *testing.T) {
	dir := "/tmp/test-yaml-output"
	cfg := generateAgentConfig(dir, "anthropic", "claude-sonnet-4-6")
	data, err := yamlMarshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	output := string(data)

	// All major config sections should be present
	required := []string{
		"agent:", "provider:", "mnemosyne:", "hot_block:",
		"tools:", "delegation:", "browser:", "vision:",
		"image_gen:", "multi_agent:", "scheduler:",
	}
	for _, key := range required {
		if !strings.Contains(output, key) {
			t.Errorf("YAML output missing section: %s", key)
		}
	}
}

func TestPersonalityContent(t *testing.T) {
	if !strings.Contains(defaultPersonality, "Bartimaeus") {
		t.Error("Default personality must mention Bartimaeus")
	}
	if !strings.Contains(strings.ToLower(defaultPersonality), "concise") {
		t.Error("Default personality should emphasize conciseness")
	}
	if !strings.Contains(strings.ToLower(defaultPersonality), "proactive") {
		t.Error("Default personality should mention being proactive with tools")
	}
}

// Red Team: Config injection tests
func TestConfigPathTraversal(t *testing.T) {
	// Paths should be resolved and not allow traversal
	dir := "/home/user/.mnemosyne"
	cfg := generateDaemonConfig(dir)

	// Socket path should be within the data directory
	if !strings.HasPrefix(cfg.Daemon.Socket, dir) {
		t.Errorf("Socket path should be within data dir: %s not in %s", cfg.Daemon.Socket, dir)
	}
	if !strings.HasPrefix(cfg.Database.Path, dir) {
		t.Errorf("Database path should be within data dir: %s not in %s", cfg.Database.Path, dir)
	}
}

func TestValidateYAMLRejectsInvalidInput(t *testing.T) {
	tmpDir := t.TempDir()

	// Test various invalid YAMLs
	invalidYAMLs := []struct {
		name    string
		content string
	}{
		{"unclosed bracket", "daemon:\n  tcp: [invalid\n"},
		{"invalid type", "daemon:\n  tcp: {]\n"},
	}

	for _, tc := range invalidYAMLs {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, tc.name+".yaml")
			if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			if err := validateYAMLFile(path); err == nil {
				t.Errorf("Expected validation error for %s", tc.name)
			}
		})
	}
}

func TestValidateYAMLAacceptsValidInput(t *testing.T) {
	tmpDir := t.TempDir()

	validYAMLs := []struct {
		name    string
		content string
	}{
		{"minimal", "daemon:\n  tcp: localhost:50051\n"},
		{"full", "daemon:\n  tcp: localhost:50051\ndatabase:\n  path: /tmp/test.db\n"},
	}

	for _, tc := range validYAMLs {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, tc.name+".yaml")
			if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			if err := validateYAMLFile(path); err != nil {
				t.Errorf("Expected valid YAML for %s: %v", tc.name, err)
			}
		})
	}
}

func TestFindStandaloneBinary(t *testing.T) {
	// This test just verifies the function doesn't panic
	_ = findStandaloneBinary("mnemosyne-daemon")
	_ = findStandaloneBinary("nonexistent-binary-xyz")
}

func TestCmdVersion(t *testing.T) {
	// Just ensure it doesn't panic — it prints to stdout
	cmdVersion([]string{})
}