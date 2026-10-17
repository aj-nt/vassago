// This file is part of Vassago.
// See LICENSE for license information.

package daemon

// ReplicationConfig configures daemon-to-daemon memory replication.
type ReplicationConfig struct {
	Enabled         bool         `yaml:"enabled"`
	PollIntervalSec int          `yaml:"poll_interval_sec"` // default: 10
	Peers           []PeerConfig `yaml:"peers"`
}

// PeerConfig defines a peer daemon to replicate memories with.
type PeerConfig struct {
	ID         string   `yaml:"id"`         // Unique peer identifier (e.g., "studio")
	Addr       string   `yaml:"addr"`       // gRPC address (e.g., "studio.local:50051")
	Namespaces []string `yaml:"namespaces"` // Namespaces to sync (e.g., ["default"])
}
