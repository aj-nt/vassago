// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

// vagent is a lightweight monitoring agent for Vassago daemon.
// It watches external sources (GitHub, email) and stores observations
// as memories in the daemon, with optional notification delivery.
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

	slog.Info("vagent starting", "daemon", cfg.DaemonAddr)

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

	// Block until signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	slog.Info("vagent running. Press Ctrl+C to stop.")
	<-sigCh

	slog.Info("Shutting down...")
	for _, w := range watchers {
		w.stop()
	}
	slog.Info("vagent stopped.")
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
