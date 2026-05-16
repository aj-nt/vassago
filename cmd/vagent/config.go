// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the vagent configuration.
type Config struct {
	DaemonAddr string       `yaml:"daemon_addr"` // gRPC address of the Vassago daemon
	AgentID    string       `yaml:"agent_id"`    // Unique identifier for this agent
	AgentName  string       `yaml:"agent_name"`  // Human-readable name
	GitHub     WatchConfig  `yaml:"github"`
	Email      WatchConfig  `yaml:"email"`
	Notify     NotifyConfig `yaml:"notify"`
}

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
	path := os.Getenv("VAGENT_CONFIG")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".vassago", "vagent.yaml")
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
		cfg.AgentID = "vagent-" + strings.ReplaceAll(host, ".local", "")
	}
	if cfg.AgentName == "" {
		cfg.AgentName = "vagent-monitor"
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

func defaultCfg(path string) {
	os.MkdirAll(filepath.Dir(path), 0700)
	cfg := `# vagent configuration — monitoring agent for Vassago daemon

daemon_addr: "localhost:50051"
agent_id: "vagent-macbook"
agent_name: "vagent-monitor"

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
`
	os.WriteFile(path, []byte(cfg), 0600)
	slogInfo("Created default config", "path", path)
}
