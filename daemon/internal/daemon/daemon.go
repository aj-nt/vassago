package daemon

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	pb "github.com/aj-nt/mnemosyne/proto"
	daemongrpc "github.com/aj-nt/mnemosyne/daemon/internal/grpc"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc"
)

// Config holds the daemon configuration.
// One file. One source of truth. No deep merge. No fallback chains.
type Config struct {
	Daemon   DaemonConfig  `yaml:"daemon"`
	Store    StoreConfig   `yaml:"database"`
	HotBlock HotBlockConfig `yaml:"hot_block"`
}

type DaemonConfig struct {
	Socket string    `yaml:"socket"`  // Unix socket path
	TCP    string    `yaml:"tcp"`     // TCP listen address (host:port)
	TLS    TLSConfig `yaml:"tls"`
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
	MemoryCharLimit    int32 `yaml:"memory_char_limit"`
	UserCharLimit      int32 `yaml:"user_char_limit"`
	MinPriority        int32 `yaml:"min_priority"`
	ObservationTTLDays int   `yaml:"observation_ttl_days"`
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
	hub    *pubsub.Hub
	server *grpc.Server
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

	// Create gRPC server with interceptors
	unaryInt, streamInt := daemongrpc.Interceptors()
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(daemongrpc.RecoveryInterceptor, unaryInt),
		grpc.StreamInterceptor(streamInt),
	)

	// Create pub/sub hub
	hub := pubsub.NewHub()

	// Register service
	pb.RegisterMnemosyneServer(server, daemongrpc.NewService(st, hub))

	return &Daemon{
		config: cfg,
		store:  st,
		hub:    hub,
		server: server,
	}, nil
}

// Run starts the daemon and blocks until shutdown.
// Sets up gRPC server, listeners, and handles graceful shutdown.
func (d *Daemon) Run() error {
	var listeners []net.Listener

	// Unix socket listener
	if d.config.Daemon.Socket != "" {
		l, err := ListenUnix(d.config.Daemon.Socket)
		if err != nil {
			return fmt.Errorf("unix socket: %w", err)
		}
		listeners = append(listeners, l)
	}

	// TCP listener
	if d.config.Daemon.TCP != "" {
		l, err := ListenTCP(d.config.Daemon.TCP)
		if err != nil {
			// Clean up unix socket on error
			for _, ul := range listeners {
				ul.Close()
			}
			return fmt.Errorf("TCP: %w", err)
		}
		listeners = append(listeners, l)
	}

	// Serve gRPC on all listeners
	errCh := make(chan error, len(listeners))
	for _, l := range listeners {
		go func(lis net.Listener) {
			if err := d.server.Serve(lis); err != nil {
				errCh <- fmt.Errorf("serve on %s: %w", lis.Addr().String(), err)
			}
		}(l)
	}

	slog.Info("mnemosyne daemon started", "db", d.config.Store.Path)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		slog.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		slog.Error("listener error", "error", err)
	}

	// Graceful stop — finish in-flight requests
	slog.Info("stopping gRPC server...")
	d.server.GracefulStop()

	// Cleanup
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

// Server returns the gRPC server (for testing).
func (d *Daemon) Server() *grpc.Server {
	return d.server
}