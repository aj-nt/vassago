// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
        "os"
        "path/filepath"
        "strings"

        "github.com/aj-nt/vassago/internal/ollama"
        "gopkg.in/yaml.v3"
)

// Config holds the watcher configuration.
type Config struct {
	DaemonAddr string       `yaml:"daemon_addr"` // gRPC address of the Vassago daemon
	AgentID    string       `yaml:"agent_id"`    // Unique identifier for this agent
	AgentName  string       `yaml:"agent_name"`  // Human-readable name
	GitHub     WatchConfig  `yaml:"github"`
	Email      WatchConfig  `yaml:"email"`
	Notify     NotifyConfig `yaml:"notify"`
	Worker     WorkerConfig `yaml:"worker"` // Orchestrator task execution
}

// WorkerConfig configures the orchestrator task executor.
// When enabled, watcher polls the daemon for ready tasks assigned to its
// agent types, claims them atomically, executes via LLM, and publishes results.
type WorkerConfig struct {
	Enabled    bool         `yaml:"enabled"`
	AgentTypes []string     `yaml:"agent_types"` // Which task types this agent handles
	PollSec    int          `yaml:"poll_sec"`    // Poll interval in seconds
	MaxTurns   int          `yaml:"max_turns"`   // Max LLM turns per task
	Ollama     OllamaConfig `yaml:"ollama"`      // LLM backend
}

// OllamaConfig is a type alias for the shared ollama.Config.
// This preserves backward compatibility within the watcher config while
// delegating to the shared daemon/internal/ollama package.
type OllamaConfig = ollama.Config

// WatchConfig configures a watcher for an external source.
type WatchConfig struct {
	Enabled      bool     `yaml:"enabled"`
	IntervalSec  int      `yaml:"interval_sec"`  // Poll interval in seconds
	Repos        []string `yaml:"repos"`          // GitHub: owner/repo, Email: not used
	IMAPHost     string   `yaml:"imap_host"`
	IMAPUser     string   `yaml:"imap_user"`
	IMAPPass     string   `yaml:"imap_pass"`
	Mailbox      string   `yaml:"mailbox"`        // IMAP mailbox (default: INBOX)
}

// NotifyConfig configures notification delivery.
type NotifyConfig struct {
	Platform string `yaml:"platform"` // "imessage", "telegram", "discord"
	Target   string `yaml:"target"`   // Recipient identifier
}

// loadConfig loads configuration from file.
func loadConfig() *Config {
	path := os.Getenv("WATCHER_CONFIG")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".vassago", "watcher.yaml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		defaultCfg(path)
		os.Exit(0)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		slogFatal("Failed to parse config", "path", path, "error", err)
	}

	// Defaults
	if cfg.DaemonAddr == "" {
		cfg.DaemonAddr = "localhost:50051"
	}
	if cfg.AgentID == "" {
		host, _ := os.Hostname()
		cfg.AgentID = "watcher-" + strings.ReplaceAll(host, ".local", "")
	}
	if cfg.AgentName == "" {
		cfg.AgentName = "watcher-monitor"
	}
	if cfg.GitHub.IntervalSec == 0 {
		cfg.GitHub.IntervalSec = 300 // 5 minutes
	}
	if cfg.Email.IntervalSec == 0 {
		cfg.Email.IntervalSec = 60 // 1 minute
	}
	if cfg.Email.Mailbox == "" {
		cfg.Email.Mailbox = "INBOX"
	}

	return &cfg
}

// applyWorkerDefaults fills zero-value worker config fields with sensible defaults.
func applyWorkerDefaults(cfg *Config) {
	if cfg.Worker.PollSec == 0 {
		cfg.Worker.PollSec = 5
	}
	if cfg.Worker.MaxTurns == 0 {
		cfg.Worker.MaxTurns = 15
	}
	if len(cfg.Worker.AgentTypes) == 0 {
		cfg.Worker.AgentTypes = []string{"general"}
	}
	if cfg.Worker.Ollama.BaseURL == "" {
		cfg.Worker.Ollama.BaseURL = "http://localhost:11434/v1"
	}
	if cfg.Worker.Ollama.Model == "" {
		cfg.Worker.Ollama.Model = "glm-5.1:cloud"
	}
	if cfg.Worker.Ollama.MaxTokens == 0 {
		cfg.Worker.Ollama.MaxTokens = 2048
	}
	if cfg.Worker.Ollama.Temperature == 0 {
		cfg.Worker.Ollama.Temperature = 0.1
	}
	if cfg.Worker.Ollama.TimeoutSec == 0 {
		cfg.Worker.Ollama.TimeoutSec = 120
	}
}

func defaultCfg(path string) {
	os.MkdirAll(filepath.Dir(path), 0700)
	cfg := `# watcher configuration — monitoring agent for Vassago daemon

daemon_addr: "localhost:50051"
agent_id: "watcher-macbook"
agent_name: "watcher-monitor"

github:
  enabled: false
  interval_sec: 300  # poll every 5 minutes
  repos:
    - "aj-nt/vassago"
    - "aj-nt/vassago-sdk"

email:
  enabled: false
  interval_sec: 60   # poll every minute
  imap_host: ""
  imap_user: ""
  imap_pass: ""
  mailbox: "INBOX"

notify:
  platform: "imessage"
  target: "+13608880110"

worker:
  enabled: false       # Set to true to enable orchestrator task execution
  agent_types:
    - general
  poll_sec: 5          # How often to check for new tasks
  max_turns: 15        # Max LLM turns per task
  ollama:
    base_url: "http://localhost:11434/v1"
    model: "glm-5.1:cloud"
    max_tokens: 2048
    temperature: 0.1
    timeout_sec: 120
`
	os.WriteFile(path, []byte(cfg), 0600)
	slogInfo("Created default config", "path", path)
}
