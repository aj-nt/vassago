// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_WorkerDefaults(t *testing.T) {
	// Create a minimal config with worker enabled (no Ollama model = use defaults)
	cfg := &Config{
		DaemonAddr: "localhost:50051",
		AgentID:    "test-agent",
		AgentName:  "Test Agent",
		Worker: WorkerConfig{
			Enabled:    true,
			AgentTypes: []string{"general"},
			PollSec:    5,
			MaxTurns:   10,
			Ollama: OllamaConfig{
				BaseURL: "http://localhost:11434/v1",
				Model:   "test-model",
			},
		},
	}

	if !cfg.Worker.Enabled {
		t.Error("Worker should be enabled")
	}
	if len(cfg.Worker.AgentTypes) != 1 || cfg.Worker.AgentTypes[0] != "general" {
		t.Errorf("expected AgentTypes=[general], got %v", cfg.Worker.AgentTypes)
	}
	if cfg.Worker.PollSec != 5 {
		t.Errorf("expected PollSec=5, got %d", cfg.Worker.PollSec)
	}
	if cfg.Worker.MaxTurns != 10 {
		t.Errorf("expected MaxTurns=10, got %d", cfg.Worker.MaxTurns)
	}
	if cfg.Worker.Ollama.Model != "test-model" {
		t.Errorf("expected model=test-model, got %s", cfg.Worker.Ollama.Model)
	}
}

func TestConfig_WorkerDisabledByDefault(t *testing.T) {
	cfg := &Config{}
	if cfg.Worker.Enabled {
		t.Error("Worker should be disabled by default")
	}
}

func TestConfig_OllamaDefaults_Zeros(t *testing.T) {
	cfg := &Config{
		Worker: WorkerConfig{
			Enabled: true,
			Ollama:  OllamaConfig{}, // all zeros
		},
	}

	applyWorkerDefaults(cfg)

	if cfg.Worker.Ollama.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("expected default BaseURL, got %s", cfg.Worker.Ollama.BaseURL)
	}
	if cfg.Worker.Ollama.Model != "glm-5.1:cloud" {
		t.Errorf("expected default model, got %s", cfg.Worker.Ollama.Model)
	}
	if cfg.Worker.Ollama.MaxTokens != 2048 {
		t.Errorf("expected default MaxTokens=2048, got %d", cfg.Worker.Ollama.MaxTokens)
	}
	if cfg.Worker.Ollama.Temperature != 0.1 {
		t.Errorf("expected default Temperature=0.1, got %f", cfg.Worker.Ollama.Temperature)
	}
	if cfg.Worker.Ollama.TimeoutSec != 120 {
		t.Errorf("expected default TimeoutSec=120, got %d", cfg.Worker.Ollama.TimeoutSec)
	}
}

func TestConfig_WorkerDefaults_Zeros(t *testing.T) {
	cfg := &Config{
		Worker: WorkerConfig{
			Enabled: true,
			// PollSec, MaxTurns, AgentTypes all zero
		},
	}

	applyWorkerDefaults(cfg)

	if cfg.Worker.PollSec != 5 {
		t.Errorf("expected default PollSec=5, got %d", cfg.Worker.PollSec)
	}
	if cfg.Worker.MaxTurns != 15 {
		t.Errorf("expected default MaxTurns=15, got %d", cfg.Worker.MaxTurns)
	}
	if len(cfg.Worker.AgentTypes) != 1 || cfg.Worker.AgentTypes[0] != "general" {
		t.Errorf("expected default AgentTypes=[general], got %v", cfg.Worker.AgentTypes)
	}
}

func TestConfig_WorkerDefaults_RespectsExistingValues(t *testing.T) {
	cfg := &Config{
		Worker: WorkerConfig{
			Enabled:    true,
			PollSec:    30,
			MaxTurns:   3,
			AgentTypes: []string{"static_analysis", "quality_scan"},
			Ollama: OllamaConfig{
				Model:     "custom-model",
				MaxTokens: 4096,
			},
		},
	}

	applyWorkerDefaults(cfg)

	if cfg.Worker.PollSec != 30 {
		t.Errorf("PollSec should remain 30, got %d", cfg.Worker.PollSec)
	}
	if cfg.Worker.MaxTurns != 3 {
		t.Errorf("MaxTurns should remain 3, got %d", cfg.Worker.MaxTurns)
	}
	if len(cfg.Worker.AgentTypes) != 2 {
		t.Errorf("AgentTypes should remain 2, got %d", len(cfg.Worker.AgentTypes))
	}
	if cfg.Worker.Ollama.Model != "custom-model" {
		t.Errorf("Model should remain custom-model, got %s", cfg.Worker.Ollama.Model)
	}
	if cfg.Worker.Ollama.MaxTokens != 4096 {
		t.Errorf("MaxTokens should remain 4096, got %d", cfg.Worker.Ollama.MaxTokens)
	}
	if cfg.Worker.Ollama.BaseURL == "" {
		t.Errorf("BaseURL should have default, got empty")
	}
}

func TestDefaultConfig_ContainsWorkerSection(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vagent.yaml")
	defaultCfg(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read default config: %v", err)
	}

	content := string(data)
	// Should contain worker section
	if !contains(content, "worker:") {
		t.Error("default config should contain 'worker:' section")
	}
	// Worker should be disabled by default (don't auto-execute tasks)
	if !contains(content, "enabled: false") {
		t.Error("worker should be disabled by default in config template")
	}
	// Should contain Ollama config subsection
	if !contains(content, "ollama:") {
		t.Error("default config should contain 'ollama:' subsection")
	}
	// Should include agent_types
	if !contains(content, "agent_types:") {
		t.Error("default config should contain 'agent_types:'")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
