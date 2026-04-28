package daemon

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/aj-nt/mnemosyne/daemon/internal/store"
)

// Config holds the daemon configuration.
// One file. One source of truth. No deep merge. No fallback chains.
type Config struct {
	Daemon  DaemonConfig  `yaml:"daemon"`
	Store   StoreConfig   `yaml:"database"`
	HotBlock HotBlockConfig `yaml:"hot_block"`
}

type DaemonConfig struct {
	Socket string      `yaml:"socket"`  // Unix socket path
	TCP    string      `yaml:"tcp"`     // TCP listen address (host:port)
	TLS    TLSConfig   `yaml:"tls"`
}

type TLSConfig struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type StoreConfig struct {
	Path        string `yaml:"path"`
	WALMode     bool   `yaml:"wal_mode"`
	BusyTimeout int    `yaml:"busy_timeout"` // ms
}

type HotBlockConfig struct {
	MemoryCharLimit   int32 `yaml:"memory_char_limit"`
	UserCharLimit     int32 `yaml:"user_char_limit"`
	MinPriority       int32 `yaml:"min_priority"`
	ObservationTTLDays int  `yaml:"observation_ttl_days"`
}

// DefaultConfig returns sensible defaults.
// These are the code constants — the only other source is the config file.
// No fallback chains. No deep merge. If it's not in the config file, you get these.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Daemon: DaemonConfig{
			Socket: filepath.Join(home, ".mnemosyne", "mnemosyne.sock"),
			TCP:    "localhost:50051",
		},
		Store: StoreConfig{
			Path:        filepath.Join(home, ".mnemosyne", "mnemosyne.db"),
			WALMode:     true,
			BusyTimeout: 5000,
		},
		HotBlock: HotBlockConfig{
			MemoryCharLimit:    4000,
			UserCharLimit:      2000,
			MinPriority:        2,
			ObservationTTLDays: 30,
		},
	}
}

// Daemon represents the Mnemosyne memory daemon.
type Daemon struct {
	config *Config
	store  *store.Store
}

// New creates a new daemon instance.
func New(cfg *Config) (*Daemon, error) {
	// Ensure database directory exists
	dbDir := filepath.Dir(cfg.Store.Path)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// Open database
	st, err := store.New(cfg.Store.Path)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	return &Daemon{
		config: cfg,
		store:  st,
	}, nil
}

// Run starts the daemon and blocks until shutdown.
// Sets up gRPC server, listeners, and handles graceful shutdown.
func (d *Daemon) Run() error {
	// TODO: Create gRPC server with interceptors
	// TODO: Register Mnemosyne service

	// Start listeners
	var listeners []net.Listener

	// Unix socket listener
	if d.config.Daemon.Socket != "" {
		// Remove stale socket
		os.Remove(d.config.Daemon.Socket)

		sockDir := filepath.Dir(d.config.Daemon.Socket)
		if err := os.MkdirAll(sockDir, 0700); err != nil {
			return fmt.Errorf("create socket directory: %w", err)
		}

		l, err := net.Listen("unix", d.config.Daemon.Socket)
		if err != nil {
			return fmt.Errorf("listen on unix socket: %w", err)
		}
		// Set socket permissions
		os.Chmod(d.config.Daemon.Socket, 0600)
		listeners = append(listeners, l)
		slog.Info("listening on unix socket", "path", d.config.Daemon.Socket)
	}

	// TCP listener
	if d.config.Daemon.TCP != "" {
		l, err := net.Listen("tcp", d.config.Daemon.TCP)
		if err != nil {
			return fmt.Errorf("listen on TCP: %w", err)
		}
		listeners = append(listeners, l)
		slog.Info("listening on TCP", "address", d.config.Daemon.TCP)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("mnemosyne daemon started", "db", d.config.Store.Path)
	<-sigCh
	slog.Info("shutting down...")

	// Cleanup
	for _, l := range listeners {
		l.Close()
	}
	if d.config.Daemon.Socket != "" {
		os.Remove(d.config.Daemon.Socket)
	}
	d.store.Close()

	slog.Info("mnemosyne daemon stopped")
	return nil
}

// Store returns the underlying store (for testing and CLI access).
func (d *Daemon) Store() *store.Store {
	return d.store
}