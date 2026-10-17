// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the worker configuration.
type Config struct {
	DaemonAddr string      `yaml:"daemon_addr"`
	AgentID    string      `yaml:"agent_id"`
	AgentName  string      `yaml:"agent_name"`
	AgentTypes []string    `yaml:"agent_types"`
	PollSec    int         `yaml:"poll_sec"`
	MaxTurns   int         `yaml:"max_turns"`
	Ollama     OllamaConfig `yaml:"ollama"`
}

// OllamaConfig holds the LLM backend configuration.
type OllamaConfig struct {
	BaseURL     string `yaml:"base_url"`
	Model       string `yaml:"model"`
	MaxTokens   int    `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
	TimeoutSec  int    `yaml:"timeout_sec"`
}

// loadConfig loads configuration from file.
func loadConfig() *Config {
	path := os.Getenv("WORKER_CONFIG")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".vassago", "worker.yaml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		defaultConfig(path)
		os.Exit(0)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		slog.Error("Failed to parse config", "path", path, "error", err)
		os.Exit(1)
	}

	// Defaults
	if cfg.DaemonAddr == "" {
		cfg.DaemonAddr = "localhost:50051"
	}
	if cfg.AgentID == "" {
		host, _ := os.Hostname()
		cfg.AgentID = "worker-" + strings.ReplaceAll(host, ".local", "")
	}
	if cfg.AgentName == "" {
		cfg.AgentName = "worker"
	}
	if len(cfg.AgentTypes) == 0 {
		cfg.AgentTypes = []string{"general"}
	}
	if cfg.PollSec == 0 {
		cfg.PollSec = 5
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 15
	}
	if cfg.Ollama.BaseURL == "" {
		cfg.Ollama.BaseURL = "http://localhost:11434/v1"
	}
	if cfg.Ollama.Model == "" {
		cfg.Ollama.Model = "glm-5.1:cloud"
	}
	if cfg.Ollama.MaxTokens == 0 {
		cfg.Ollama.MaxTokens = 2048
	}
	if cfg.Ollama.Temperature == 0 {
		cfg.Ollama.Temperature = 0.1
	}
	if cfg.Ollama.TimeoutSec == 0 {
		cfg.Ollama.TimeoutSec = 120
	}

	return &cfg
}

func defaultConfig(path string) {
	os.MkdirAll(filepath.Dir(path), 0700)
	cfg := `# worker configuration — autonomous task executor for Vassago daemon

daemon_addr: "localhost:50051"
agent_id: "worker-hera"
agent_name: "worker"
agent_types:
  - general
  - static_analysis
  - quality_scan
poll_sec: 5
max_turns: 15

ollama:
  base_url: "http://localhost:11434/v1"
  model: "glm-5.1:cloud"
  max_tokens: 2048
  temperature: 0.1
  timeout_sec: 120
`
	os.WriteFile(path, []byte(cfg), 0600)
	slog.Info("Created default config", "path", path)
}
