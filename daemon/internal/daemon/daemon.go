package daemon

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	daemongrpc "github.com/aj-nt/mnemosyne/daemon/internal/grpc"
	"github.com/aj-nt/mnemosyne/daemon/internal/messaging"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Config holds the daemon configuration.
// One file. One source of truth. No deep merge. No fallback chains.
type Config struct {
	Daemon     DaemonConfig      `yaml:"daemon"`
	Store      StoreConfig       `yaml:"database"`
	HotBlock   HotBlockConfig    `yaml:"hot_block"`
	Messaging  MessagingConfig   `yaml:"messaging"`
}

type MessagingConfig struct {
	Discord  DiscordMessagingConfig  `yaml:"discord"`
	Telegram TelegramMessagingConfig `yaml:"telegram"`
	IMessage IMessageMessagingConfig `yaml:"imessage"`
}

type DiscordMessagingConfig struct {
	Token           string   `yaml:"token"`
	Guilds          []string `yaml:"guilds"`
	AllowedChannels []string `yaml:"allowed_channels"`
}

type TelegramMessagingConfig struct {
	Token       string  `yaml:"token"`
	AllowedChats []int64 `yaml:"allowed_chats"`
}

type IMessageMessagingConfig struct {
	Enabled           bool     `yaml:"enabled"`
	AllowedRecipients []string `yaml:"allowed_recipients"`
	DBPath            string   `yaml:"db_path"`
	PollInterval      int      `yaml:"poll_interval"` // seconds, default 5
}

type DaemonConfig struct {
	Socket  string    `yaml:"socket"`  // Unix socket path
	TCP     string    `yaml:"tcp"`     // TCP listen address (host:port)
	HTTP    string    `yaml:"http"`    // HTTP server address (host:port)
	TLS     TLSConfig `yaml:"tls"`
	APIKey  string    `yaml:"api_key"` // Optional API key for authentication
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

// Validate checks the config for invalid values.
func (c *Config) Validate() error {
	// Daemon section validation
	if c.Daemon.Socket == "" && c.Daemon.TCP == "" {
		return fmt.Errorf("daemon.socket or daemon.tcp must be configured")
	}

	// TLS validation - if one is set, both must be set
	if (c.Daemon.TLS.Cert != "") != (c.Daemon.TLS.Key != "") {
		return fmt.Errorf("daemon.tls.cert and daemon.tls.key must both be set or both be empty")
	}

	// Store section validation
	if c.Store.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	if c.Store.BusyTimeout < 1000 || c.Store.BusyTimeout > 30000 {
		return fmt.Errorf("database.busy_timeout must be between 1000 and 30000 ms, got %d", c.Store.BusyTimeout)
	}

	// HotBlock section validation
	if c.HotBlock.MemoryCharLimit < 1000 || c.HotBlock.MemoryCharLimit > 20000 {
		return fmt.Errorf("hot_block.memory_char_limit must be between 1000 and 20000, got %d", c.HotBlock.MemoryCharLimit)
	}
	if c.HotBlock.UserCharLimit < 500 || c.HotBlock.UserCharLimit > 10000 {
		return fmt.Errorf("hot_block.user_char_limit must be between 500 and 10000, got %d", c.HotBlock.UserCharLimit)
	}
	if c.HotBlock.MinPriority < 1 || c.HotBlock.MinPriority > 5 {
		return fmt.Errorf("hot_block.min_priority must be between 1 and 5, got %d", c.HotBlock.MinPriority)
	}
	if c.HotBlock.ObservationTTLDays < 7 || c.HotBlock.ObservationTTLDays > 365 {
		return fmt.Errorf("hot_block.observation_ttl_days must be between 7 and 365, got %d", c.HotBlock.ObservationTTLDays)
	}

	return nil
}

// Daemon represents the Mnemosyne memory daemon.
type Daemon struct {
	config *Config
	store  *store.Store
	hub    *pubsub.Hub
	server *grpc.Server
	httpServer *http.Server
}

// transportCredentials returns TLS credentials if configured, otherwise nil.
// nolint:unused // This function is kept for potential future use
func (d *Daemon) transportCredentials() (credentials.TransportCredentials, error) {
	if d.config.Daemon.TLS.Cert == "" || d.config.Daemon.TLS.Key == "" {
		return nil, nil
	}

	// Load TLS certificate and key
	cert, err := tls.LoadX509KeyPair(d.config.Daemon.TLS.Cert, d.config.Daemon.TLS.Key)
	if err != nil {
		return nil, fmt.Errorf("load TLS key pair: %w", err)
	}

	return credentials.NewServerTLSFromCert(&cert), nil
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

	// Create gRPC server with interceptors and hardening options
	unaryInt, streamInt := daemongrpc.Interceptors(cfg.Daemon.APIKey)
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(daemongrpc.RecoveryInterceptor, unaryInt),
		grpc.StreamInterceptor(streamInt),
		grpc.MaxRecvMsgSize(10*1024*1024),     // 10MB max message size
		grpc.MaxConcurrentStreams(uint32(100)),  // Limit concurrent streams
	)

	// Create pub/sub hub
	hub := pubsub.NewHub()

	// Create messaging service
	messagingService := daemongrpc.NewMessagingService()

	// Add Discord adapter if token is configured
	if cfg.Messaging.Discord.Token != "" {
		discordAdapter := messaging.NewDiscordAdapter(messaging.DiscordConfig{
			Token:           cfg.Messaging.Discord.Token,
			Guilds:          cfg.Messaging.Discord.Guilds,
			AllowedChannels: cfg.Messaging.Discord.AllowedChannels,
		})
		messagingService.AddAdapter("discord", discordAdapter)
	}

	// Add Telegram adapter if token is configured
	if cfg.Messaging.Telegram.Token != "" {
		telegramAdapter := messaging.NewTelegramAdapter(messaging.TelegramConfig{
			Token:        cfg.Messaging.Telegram.Token,
			AllowedChats: cfg.Messaging.Telegram.AllowedChats,
		})
		messagingService.AddAdapter("telegram", telegramAdapter)
	}

	// Add iMessage adapter if enabled (macOS only)
	if cfg.Messaging.IMessage.Enabled {
		pollInterval := time.Duration(cfg.Messaging.IMessage.PollInterval) * time.Second
		imessageAdapter := messaging.NewIMessageAdapter(messaging.IMessageConfig{
			AllowedRecipients: cfg.Messaging.IMessage.AllowedRecipients,
			DBPath:             cfg.Messaging.IMessage.DBPath,
			PollInterval:       pollInterval,
		})
		messagingService.AddAdapter("imessage", imessageAdapter)
	}

	// Register services
	pb.RegisterMnemosyneServer(server, daemongrpc.NewService(st, hub, messagingService))
	pb.RegisterMessagingServer(server, messagingService)

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

	// HTTP server setup
	// Error channel for all servers
	errCh := make(chan error, len(listeners)+1) // +1 for HTTP server

	if d.config.Daemon.HTTP != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/chat", d.handleChat)

		d.httpServer = &http.Server{
			Addr:    d.config.Daemon.HTTP,
			Handler: mux,
		}

		go func() {
			slog.Info("starting HTTP server", "address", d.config.Daemon.HTTP)
			if err := d.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("HTTP server: %w", err)
			}
		}()
	}
	for _, l := range listeners {
		go func(lis net.Listener) {
			if err := d.server.Serve(lis); err != nil {
				errCh <- fmt.Errorf("serve on %s: %w", lis.Addr().String(), err)
			}
		}(l)
	}

	slog.Info("mnemosyne daemon started", "db", d.config.Store.Path)

	// Startup: expire stale memories
	if removed, err := d.store.ExpireStaleMemories(); err != nil {
		slog.Error("startup expiry sweep failed", "error", err)
	} else if removed > 0 {
		slog.Info("startup: expired stale memories", "count", removed)
	}

	// Background: periodic consolidation (expired + idle sweep every hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.runAutoConsolidation(ctx)

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

	// Stop HTTP server if it's running
	if d.httpServer != nil {
		slog.Info("stopping HTTP server...")
		if err := d.httpServer.Shutdown(ctx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}

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

// handleChat handles HTTP chat requests
func (d *Daemon) handleChat(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement chat functionality
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Chat endpoint - to be implemented"))
}

// runAutoConsolidation runs periodic consolidation (expired + idle sweep) every hour.
func (d *Daemon) runAutoConsolidation(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("auto-consolidation stopped")
			return
		case <-ticker.C:
			if result, err := d.store.Consolidate(store.ConsolidateScopeExpired, "", 0); err != nil {
				slog.Error("auto-consolidation (expired) failed", "error", err)
			} else if result.Removed > 0 {
				slog.Info("auto-consolidation: expired memories removed", "count", result.Removed)
			}
			if result, err := d.store.Consolidate(store.ConsolidateScopeIdle, "", 0); err != nil {
				slog.Error("auto-consolidation (idle) failed", "error", err)
			} else if result.Removed > 0 {
				slog.Info("auto-consolidation: idle memories removed", "count", result.Removed)
			}
		}
	}
}