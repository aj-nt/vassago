// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the orchestrator configuration.
type Config struct {
	DaemonAddr      string    `yaml:"daemon_addr"`
	RepoPath        string    `yaml:"repo_path"`
	PollIntervalSec int       `yaml:"poll_interval_sec"`
	MaxWaitSec      int       `yaml:"max_wait_sec"`
	Decomposer      LLMConfig `yaml:"decomposer"`
	Synthesizer     LLMConfig `yaml:"synthesizer"`
}

// LLMConfig holds the configuration for an LLM backend.
type LLMConfig struct {
	Provider    string  `yaml:"provider"`
	Model       string  `yaml:"model"`
	BaseURL     string  `yaml:"base_url"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
	TimeoutSec  int     `yaml:"timeout_sec"`
}

// DefaultConfig returns an orchestrator config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DaemonAddr:      "localhost:50051",
		PollIntervalSec: 3,
		MaxWaitSec:      300,
		Decomposer: LLMConfig{
			Provider:    "ollama",
			Model:       "glm-5.1:cloud",
			BaseURL:     "http://localhost:11434/v1",
			MaxTokens:   2000,
			Temperature: 0.1,
			TimeoutSec:  120,
		},
		Synthesizer: LLMConfig{
			Provider:    "ollama",
			Model:       "glm-5.1:cloud",
			BaseURL:     "http://localhost:11434/v1",
			MaxTokens:   4096,
			Temperature: 0.2,
			TimeoutSec:  120,
		},
	}
}

// loadConfig loads the orchestrator config from file.
// If the config file doesn't exist, creates a default one and returns the default config.
// The caller (main) decides whether to exit after config creation.
func loadConfig() *Config {
	path := os.Getenv("ORCHESTRATOR_CONFIG")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".vassago", "orchestrator.yaml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		writeDefaultConfig(path)
		cfg := DefaultConfig()
		return cfg
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		slog.Error("Failed to parse config", "path", path, "error", err)
		os.Exit(1)
	}

	applyDefaults(&cfg)
	return &cfg
}

// applyDefaults fills zero-value fields with sensible defaults.
func applyDefaults(cfg *Config) {
	if cfg.DaemonAddr == "" {
		cfg.DaemonAddr = "localhost:50051"
	}
	if cfg.PollIntervalSec <= 0 {
		cfg.PollIntervalSec = 3
	}
	if cfg.MaxWaitSec <= 0 {
		cfg.MaxWaitSec = 300
	}
	applyLLMDefaults(&cfg.Decomposer, 2000, 0.1)
	applyLLMDefaults(&cfg.Synthesizer, 4096, 0.2)
}

func applyLLMDefaults(cfg *LLMConfig, defaultTokens int, defaultTemp float64) {
	if cfg.Provider == "" {
		cfg.Provider = "ollama"
	}
	if cfg.Model == "" {
		cfg.Model = "glm-5.1:cloud"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434/v1"
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = defaultTokens
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = defaultTemp
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 120
	}
}

func writeDefaultConfig(path string) {
	os.MkdirAll(filepath.Dir(path), 0700)
	cfg := `# orchestrator configuration — goal decomposition and synthesis agent for Vassago daemon

daemon_addr: "localhost:50051"
repo_path: ""
poll_interval_sec: 3
max_wait_sec: 300

decomposer:
  provider: "ollama"
  model: "glm-5.1:cloud"
  base_url: "http://localhost:11434/v1"
  max_tokens: 2000
  temperature: 0.1
  timeout_sec: 120

synthesizer:
  provider: "ollama"
  model: "glm-5.1:cloud"
  base_url: "http://localhost:11434/v1"
  max_tokens: 4096
  temperature: 0.2
  timeout_sec: 120
`
	os.WriteFile(path, []byte(cfg), 0600)
	slog.Info("Created default orchestrator config", "path", path)
}
