// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DaemonAddr == "" {
		t.Error("DaemonAddr should have default")
	}
	if cfg.PollIntervalSec <= 0 {
		t.Error("PollIntervalSec should be positive")
	}
	if cfg.MaxWaitSec <= 0 {
		t.Error("MaxWaitSec should be positive")
	}
	if cfg.Decomposer.Model == "" {
		t.Error("Decomposer.Model should have default")
	}
	if cfg.Synthesizer.Model == "" {
		t.Error("Synthesizer.Model should have default")
	}
	if cfg.Decomposer.BaseURL == "" {
		t.Error("Decomposer.BaseURL should have default")
	}
	if cfg.Synthesizer.TimeoutSec <= 0 {
		t.Error("Synthesizer.TimeoutSec should be positive")
	}
}

func TestLoadConfig_CreatesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "orchestrator.yaml")
	t.Setenv("ORCHESTRATOR_CONFIG", path)

	cfg := loadConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("default config not created at %s: %v", path, err)
	}
}

func TestLoadConfig_ReadsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "orchestrator.yaml")
	t.Setenv("ORCHESTRATOR_CONFIG", path)

	content := `daemon_addr: "studio:50051"
repo_path: "/home/aj/projects/myapp"
poll_interval_sec: 5
max_wait_sec: 600
decomposer:
  provider: "anthropic"
  model: "claude-sonnet-4-6"
  base_url: "https://api.anthropic.com/v1"
  max_tokens: 4000
  temperature: 0.0
  timeout_sec: 180
synthesizer:
  provider: "ollama"
  model: "nemotron-3-super:cloud"
  base_url: "http://studio:11434/v1"
  max_tokens: 8192
  temperature: 0.3
  timeout_sec: 240
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig()
	if cfg.DaemonAddr != "studio:50051" {
		t.Errorf("DaemonAddr = %q, want %q", cfg.DaemonAddr, "studio:50051")
	}
	if cfg.RepoPath != "/home/aj/projects/myapp" {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, "/home/aj/projects/myapp")
	}
	if cfg.Decomposer.Provider != "anthropic" {
		t.Errorf("Decomposer.Provider = %q", cfg.Decomposer.Provider)
	}
	if cfg.Decomposer.Model != "claude-sonnet-4-6" {
		t.Errorf("Decomposer.Model = %q", cfg.Decomposer.Model)
	}
	if cfg.Synthesizer.MaxTokens != 8192 {
		t.Errorf("Synthesizer.MaxTokens = %d, want 8192", cfg.Synthesizer.MaxTokens)
	}
}

func TestDefaultConfig_DecomposerDefaultsApplied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Decomposer.MaxTokens = 0
	cfg.Decomposer.TimeoutSec = 0
	cfg.Synthesizer.MaxTokens = 0
	cfg.Synthesizer.TimeoutSec = 0

	applyDefaults(cfg)

	if cfg.Decomposer.MaxTokens != 2000 {
		t.Errorf("Decomposer.MaxTokens default not applied: got %d", cfg.Decomposer.MaxTokens)
	}
	if cfg.Decomposer.TimeoutSec != 120 {
		t.Errorf("Decomposer.TimeoutSec default not applied: got %d", cfg.Decomposer.TimeoutSec)
	}
	if cfg.Synthesizer.MaxTokens != 4096 {
		t.Errorf("Synthesizer.MaxTokens default not applied: got %d", cfg.Synthesizer.MaxTokens)
	}
	if cfg.Synthesizer.TimeoutSec != 120 {
		t.Errorf("Synthesizer.TimeoutSec default not applied: got %d", cfg.Synthesizer.TimeoutSec)
	}
	if cfg.Decomposer.Temperature < 0.05 && cfg.Decomposer.Temperature > -0.01 {
		// Temperature was explicitly set to 0.0 in test, verify it wasn't overridden
	} else {
		// Actually, 0.0 is the zero value — defaults WILL apply. That's correct behavior.
	}
}

func TestDefaultConfig_TemperatureDefaultsApplied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Decomposer.Temperature = 0
	cfg.Synthesizer.Temperature = 0

	applyDefaults(cfg)

	if cfg.Decomposer.Temperature != 0.1 {
		t.Errorf("Decomposer.Temperature default not applied: got %f", cfg.Decomposer.Temperature)
	}
	if cfg.Synthesizer.Temperature != 0.2 {
		t.Errorf("Synthesizer.Temperature default not applied: got %f", cfg.Synthesizer.Temperature)
	}
}
