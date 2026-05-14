// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

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

	"github.com/aj-nt/mnemosyne/daemon/internal/cron"
	"github.com/aj-nt/mnemosyne/daemon/internal/extraction"
	"github.com/aj-nt/mnemosyne/daemon/internal/github"
	pb "github.com/aj-nt/mnemosyne/proto"
	daemongrpc "github.com/aj-nt/mnemosyne/daemon/internal/grpc"
	"github.com/aj-nt/mnemosyne/daemon/internal/messaging"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

// Config holds the daemon configuration.
// One file. One source of truth. No deep merge. No fallback chains.
type Config struct {
	Daemon     DaemonConfig      `yaml:"daemon"`
	Store      StoreConfig       `yaml:"database"`
	HotBlock   HotBlockConfig    `yaml:"hot_block"`
	Messaging  MessagingConfig   `yaml:"messaging"`
	Cron       CronConfig        `yaml:"cron"`
	GitHub     GitHubConfig      `yaml:"github"`
	Extraction ExtractionConfig  `yaml:"extraction"`
}

type GitHubConfig struct {
	Enabled       bool   `yaml:"enabled"`
	WebhookSecret string `yaml:"webhook_secret"`
}

type CronConfig struct {
	Enabled  bool `yaml:"enabled"`  // Whether the cron runner is enabled
	Interval int  `yaml:"interval"` // Check interval in seconds (default: 60)
	MaxJobs  int  `yaml:"max_jobs"` // Max jobs per agent (default: 100)
}

type MessagingConfig struct {
    Discord  DiscordMessagingConfig  `yaml:"discord"`
    Telegram TelegramMessagingConfig `yaml:"telegram"`
    IMessage IMessageMessagingConfig `yaml:"imessage"`
    Email    EmailMessagingConfig    `yaml:"email"`
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

type EmailMessagingConfig struct {
	Enabled      bool     `yaml:"enabled"`
	IMAPHost     string   `yaml:"imap_host"`
	SMTPHost     string   `yaml:"smtp_host"`
	Username     string   `yaml:"username"`
	Password     string   `yaml:"password"`
	FromAddress  string   `yaml:"from_address"`
	AllowedFrom  []string `yaml:"allowed_from"`
	Folders      []string `yaml:"folders"`
	PollInterval int      `yaml:"poll_interval"` // seconds, default 30
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
		Cron: CronConfig{
			Interval: 60,
			MaxJobs:  100,
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

	// Cron section validation
	if c.Cron.Enabled {
		if c.Cron.Interval < 10 || c.Cron.Interval > 3600 {
			return fmt.Errorf("cron.interval must be between 10 and 3600 seconds, got %d", c.Cron.Interval)
		}
		if c.Cron.MaxJobs < 1 || c.Cron.MaxJobs > 1000 {
			return fmt.Errorf("cron.max_jobs must be between 1 and 1000, got %d", c.Cron.MaxJobs)
		}
	}

	return nil
}

// Daemon represents the Mnemosyne memory daemon.
type Daemon struct {
	config             *Config
	store              *store.Store
	hub                *pubsub.Hub
	server             *grpc.Server
	httpServer         *http.Server
	cronRunner         *cron.CronRunner
	extractionScheduler *extraction.Scheduler
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

// setupMessagingAdapters registers platform adapters on the messaging service
// based on the daemon config. Only platforms with configured tokens/credentials
// get registered; others are silently skipped.
func setupMessagingAdapters(cfg *Config, messagingService *daemongrpc.MessagingService) {
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
			DBPath:            cfg.Messaging.IMessage.DBPath,
			PollInterval:      pollInterval,
		})
		messagingService.AddAdapter("imessage", imessageAdapter)
	}

	// Add Email adapter if enabled
	if cfg.Messaging.Email.Enabled {
		pollInterval := time.Duration(cfg.Messaging.Email.PollInterval) * time.Second
		emailAdapter := messaging.NewEmailAdapter(messaging.EmailConfig{
			IMAPHost:     cfg.Messaging.Email.IMAPHost,
			SMTPHost:     cfg.Messaging.Email.SMTPHost,
			Username:     cfg.Messaging.Email.Username,
			Password:     cfg.Messaging.Email.Password,
			FromAddress:  cfg.Messaging.Email.FromAddress,
			AllowedFrom:  cfg.Messaging.Email.AllowedFrom,
			Folders:      cfg.Messaging.Email.Folders,
			PollInterval: pollInterval,
		})
		messagingService.AddAdapter("email", emailAdapter)
	}
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

	// Create messaging service and register platform adapters
	messagingService := daemongrpc.NewMessagingService()
	setupMessagingAdapters(cfg, messagingService)

	// Create gRPC service
	grpcSvc := daemongrpc.NewService(st, hub, messagingService)

	// Register services
	pb.RegisterMnemosyneServer(server, grpcSvc)
	pb.RegisterMessagingServer(server, messagingService)
	pb.RegisterCronServiceServer(server, daemongrpc.NewCronService(st))

	// Enable gRPC reflection
	reflection.Register(server)

	// Create cron runner if enabled
	var cronRunner *cron.CronRunner
	if cfg.Cron.Enabled {
		cronAdapter := newCronStoreAdapter(st)
		fireCallback := newCronFireCallback(st, hub)
		cronRunner = cron.NewCronRunner(cronAdapter, fireCallback, cron.Config{
			Enabled:  cfg.Cron.Enabled,
			Interval: cfg.Cron.Interval,
			MaxJobs:  cfg.Cron.MaxJobs,
		})
	}

	// Wire autonomous memory extraction
	var extractionScheduler *extraction.Scheduler
	if cfg.Extraction.Enabled {
		extCfg := extraction.Config{
			DebounceInterval:         cfg.Extraction.DebounceInterval,
			HardCapInterval:          cfg.Extraction.HardCapInterval,
			MaxMessagesPerExtraction: 50,
			MinMessagesForExtraction: 4,
			Priority:                 2,
			Model:                    cfg.Extraction.Model,
		}
		if extCfg.DebounceInterval <= 0 {
			extCfg.DebounceInterval = 30
		}
		if extCfg.HardCapInterval <= 0 {
			extCfg.HardCapInterval = 900
		}

		client := extraction.NewHTTPClient(extraction.HTTPClientConfig{
			Model:    cfg.Extraction.Model,
			BaseURL:  cfg.Extraction.BaseURL,
			APIKey:   cfg.Extraction.APIKey,
		})

		// Reuse the LLM client for session summaries on EndSession
		grpcSvc.SetSummaryClient(client)

		pipeline := extraction.NewPipeline(extCfg, newExtractionStore(st))
		pipeline.SetClient(client)

		extractionScheduler = extraction.NewScheduler(extCfg, func(sessionID string) {
			msgs, err := st.GetSessionMessages(sessionID)
			if err != nil {
				slog.Error("extraction: failed to get session messages", "session_id", sessionID, "error", err)
				return
			}
			if len(msgs) < extCfg.MinMessagesForExtraction {
				return
			}
			extMsgs := make([]extraction.SessionMessage, len(msgs))
			for i, m := range msgs {
				extMsgs[i] = extraction.SessionMessage{
					Role:     m.Role,
					Content:  m.Content,
					ToolName: m.ToolName,
				}
				if m.ToolName != "" && m.Role == "assistant" {
					extMsgs[i].ToolCalls = []extraction.ToolCall{{Name: m.ToolName}}
				}
			}
			persisted, err := pipeline.ExtractAndPersist("default", "mnemosyne-daemon", extMsgs)
			if err != nil {
				slog.Error("extraction: pipeline failed", "session_id", sessionID, "error", err)
				return
			}
			if persisted > 0 {
				slog.Info("extraction: new memories persisted", "session_id", sessionID, "count", persisted)
			}
		})

		// Wire the AddMessages hook
		grpcSvc.OnMessagesAdded = func(sessionID, owner string) {
			extractionScheduler.Notify(sessionID)
		}
	}

	return &Daemon{
		config:              cfg,
		store:               st,
		hub:                 hub,
		server:              server,
		cronRunner:          cronRunner,
		extractionScheduler: extractionScheduler,
	}, nil
}

// setupListeners creates network listeners from the daemon config.
// Supports Unix domain sockets and TCP. If neither is configured,
// returns an error.
func setupListeners(cfg DaemonConfig) ([]net.Listener, error) {
	var listeners []net.Listener

	if cfg.Socket != "" {
		l, err := ListenUnix(cfg.Socket)
		if err != nil {
			return nil, fmt.Errorf("unix socket: %w", err)
		}
		listeners = append(listeners, l)
	}

	if cfg.TCP != "" {
		l, err := ListenTCP(cfg.TCP)
		if err != nil {
			// Clean up already-opened listeners on error
			for _, ul := range listeners {
				ul.Close()
			}
			return nil, fmt.Errorf("TCP: %w", err)
		}
		listeners = append(listeners, l)
	}

	if len(listeners) == 0 {
		return nil, fmt.Errorf("no listener configured: set daemon.socket or daemon.tcp")
	}

	return listeners, nil
}

// Run starts the daemon and blocks until shutdown.
// Sets up gRPC server, listeners, and handles graceful shutdown.
func (d *Daemon) Run() error {
	listeners, err := setupListeners(d.config.Daemon)
	if err != nil {
		return err
	}

	// HTTP server setup
	// Error channel for all servers
	errCh := make(chan error, len(listeners)+1) // +1 for HTTP server

	if d.config.Daemon.HTTP != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/chat", d.handleChat)

		// GitHub webhook integration
		if d.config.GitHub.Enabled {
			github.WebhookSecret = d.config.GitHub.WebhookSecret
			webhookHandler := github.NewHandler(func(eventType string, payload []byte) (string, error) {
				memory, err := d.processGitHubEvent(eventType, payload)
				if err != nil {
					slog.Error("github webhook: event processing failed", "event", eventType, "error", err)
					return "", err
				}
				return memory, nil
			})
			mux.Handle("/github/webhook", webhookHandler)
			slog.Info("GitHub webhook enabled", "path", "/github/webhook")
		}

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
	if removed, err := d.store.ExpireStaleMemories("default"); err != nil {
		slog.Error("startup expiry sweep failed", "error", err)
	} else if removed > 0 {
		slog.Info("startup: expired stale memories", "count", removed)
	}

	// Background: periodic consolidation (expired + idle sweep every hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.runAutoConsolidation(ctx)

	// Start cron runner if configured
	if d.cronRunner != nil {
		d.cronRunner.Start(ctx)
		defer d.cronRunner.Stop()
		// Recompute next_fire_at for any jobs that haven't been initialized
		if err := d.cronRunner.RecomputeNextFireTimes(ctx); err != nil {
			slog.Error("cron runner recompute failed", "error", err)
		}
	}

	// Start extraction scheduler if configured
	if d.extractionScheduler != nil {
		d.extractionScheduler.Start(ctx)
		defer d.extractionScheduler.Stop()
		slog.Info("memory extraction enabled",
			"debounce", d.config.Extraction.DebounceInterval,
			"hard_cap", d.config.Extraction.HardCapInterval)
	}

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

// handleChat handles HTTP chat requests.
// Not yet implemented — the primary chat interface is via gRPC (ChatStream).
func (d *Daemon) handleChat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"chat via HTTP not implemented; use gRPC ChatStream instead"}`))
}

// processGitHubEvent converts a GitHub webhook event into a memory and persists it.
func (d *Daemon) processGitHubEvent(eventType string, payload []byte) (string, error) {
	var memory string
	var err error

	switch eventType {
	case "issues":
		memory, err = github.ParseIssueEvent(payload)
	case "pull_request":
		memory, err = github.ParsePREvent(payload)
	default:
		// ping, push, star, etc. — silently skip
		return "", nil
	}

	if err != nil {
		return "", err
	}
	if memory == "" {
		return "", nil
	}

	// Persist as a memory
	if _, saveErr := d.store.AddMemory(store.AddMemoryParams{
		Owner:       "default",
		Content:     memory,
		Category:    "observation",
		Priority:    3,
		SourceAgent: "github-webhook",
	}); saveErr != nil {
		return "", fmt.Errorf("save memory: %w", saveErr)
	}

	// Emit telepathy event so agents invalidate their hot blocks
	d.hub.Publish(pubsub.Event{
		Type:    pb.UpdateEvent_UPDATED,
		Target:  "memory",
		AgentID: "daemon",
	})

	slog.Info("github webhook: memory saved", "event", eventType, "chars", len(memory))
	return memory, nil
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
			if result, err := d.store.Consolidate("default", store.ConsolidateScopeExpired, "", 0); err != nil {
				slog.Error("auto-consolidation (expired) failed", "error", err)
			} else if result.Removed > 0 {
				slog.Info("auto-consolidation: expired memories removed", "count", result.Removed)
			}
			if result, err := d.store.Consolidate("default", store.ConsolidateScopeIdle, "", 0); err != nil {
				slog.Error("auto-consolidation (idle) failed", "error", err)
			} else if result.Removed > 0 {
				slog.Info("auto-consolidation: idle memories removed", "count", result.Removed)
			}
		}
	}
}