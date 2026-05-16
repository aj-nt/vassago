// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package daemon

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads the configuration from a YAML file.
// If the file does not exist, it returns the default configuration.
// If the file exists but has partial fields, it merges with defaults.
func LoadConfig(path string) (*Config, error) {
	// Start with defaults
	config := DefaultConfig()

	// If config file doesn't exist, return defaults
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return config, nil
	}

	// Read YAML file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Parse YAML into a temporary config
	var fileConfig Config
	if err := yaml.Unmarshal(data, &fileConfig); err != nil {
		return nil, fmt.Errorf("parse config YAML: %w", err)
	}

	// Merge non-zero fields from file config into defaults
	mergeConfig(config, &fileConfig)

	// Validate the final configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return config, nil
}

// mergeConfig merges non-zero fields from src into dst
func mergeConfig(dst, src *Config) {
	if src.Daemon.Socket != "" {
		dst.Daemon.Socket = src.Daemon.Socket
	}
	if src.Daemon.TCP != "" {
		dst.Daemon.TCP = src.Daemon.TCP
	}
	if src.Daemon.HTTP != "" {
		dst.Daemon.HTTP = src.Daemon.HTTP
	}
	if src.Daemon.TLS.Cert != "" {
		dst.Daemon.TLS.Cert = src.Daemon.TLS.Cert
	}
	if src.Daemon.TLS.Key != "" {
		dst.Daemon.TLS.Key = src.Daemon.TLS.Key
	}
	if src.Store.Path != "" {
		dst.Store.Path = src.Store.Path
	}
	if src.Store.BusyTimeout != 0 {
		dst.Store.BusyTimeout = src.Store.BusyTimeout
	}
	dst.Store.WALMode = src.Store.WALMode

	if src.HotBlock.MemoryCharLimit != 0 {
		dst.HotBlock.MemoryCharLimit = src.HotBlock.MemoryCharLimit
	}
	if src.HotBlock.UserCharLimit != 0 {
		dst.HotBlock.UserCharLimit = src.HotBlock.UserCharLimit
	}
	if src.HotBlock.MinPriority != 0 {
		dst.HotBlock.MinPriority = src.HotBlock.MinPriority
	}
	if src.HotBlock.ObservationTTLDays != 0 {
		dst.HotBlock.ObservationTTLDays = src.HotBlock.ObservationTTLDays
	}

	// Replication section
	if src.Replication.PollIntervalSec != 0 {
		dst.Replication.PollIntervalSec = src.Replication.PollIntervalSec
	}
	if len(src.Replication.Peers) > 0 {
		dst.Replication.Peers = src.Replication.Peers
	}
	dst.Replication.Enabled = src.Replication.Enabled
}
