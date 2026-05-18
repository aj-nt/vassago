// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

// watcher is a lightweight monitoring agent for Vassago daemon.
// It watches external sources (GitHub, email) and stores observations
// as memories in the daemon, with optional notification delivery.
// It also supports orchestrator task execution when the worker is enabled.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aj-nt/vassago-sdk/client"
)

func main() {
	cfg := loadConfig()
	applyWorkerDefaults(cfg)

	slog.Info("watcher starting", "daemon", cfg.DaemonAddr)

	// Connect to daemon
	ctx := context.Background()
	c, err := client.Connect(ctx, client.Config{Address: cfg.DaemonAddr})
	if err != nil {
		slog.Error("Failed to connect to daemon", "error", err)
		os.Exit(1)
	}
	defer c.Close()

	// Register as an agent
	agentID, isNew, err := c.RegisterAgent(ctx, cfg.AgentID, cfg.AgentName, "monitor")
	if err != nil {
		slog.Error("Failed to register agent", "error", err)
		os.Exit(1)
	}
	if isNew {
		slog.Info("Registered new agent", "agent_id", agentID)
	} else {
		slog.Info("Reconnected as existing agent", "agent_id", agentID)
	}

	// Heartbeat loop
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go heartbeatLoop(hbCtx, c, agentID, 60*time.Second)

	// Start watchers
	watchers := startWatchers(c, agentID, cfg)

	// Start worker if enabled (orchestrator task execution)
	if cfg.Worker.Enabled {
		slog.Info("Worker enabled — starting orchestrator task execution",
			"agent_types", cfg.Worker.AgentTypes,
			"poll_sec", cfg.Worker.PollSec,
			"max_turns", cfg.Worker.MaxTurns,
			"model", cfg.Worker.Ollama.Model,
		)
		workerCtx, workerCancel := context.WithCancel(ctx)
		defer workerCancel()
		go startWorker(workerCtx, c, agentID, cfg.Worker)
	}

	// Block until signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	slog.Info("watcher running. Press Ctrl+C to stop.")
	<-sigCh

	slog.Info("Shutting down...")
	for _, w := range watchers {
		w.stop()
	}
	slog.Info("watcher stopped.")
}

func heartbeatLoop(ctx context.Context, c client.MnemoClient, agentID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.Heartbeat(ctx, agentID); err != nil {
				slog.Warn("Heartbeat failed", "error", err)
			}
		}
	}
}

// Watcher is something that polls an external source and pushes to the daemon.
type Watcher interface {
	start()
	stop()
}
