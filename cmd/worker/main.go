// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

// worker is a lightweight task executor for Vassago daemon.
// It polls for ready tasks matching its configured agent types,
// claims them atomically, executes using an LLM (via Ollama's
// OpenAI-compatible API), and reports results back to the daemon.
//
// Built as a single static binary — runs on any Linux/macOS host
// with network access to the daemon and an Ollama instance.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := loadConfig()

	slog.Info("worker starting",
		"daemon", cfg.DaemonAddr,
		"types", cfg.AgentTypes,
		"model", cfg.Ollama.Model,
	)

	worker, err := NewWorker(cfg)
	if err != nil {
		slog.Error("Failed to create worker", "error", err)
		os.Exit(1)
	}
	defer worker.Close()

	// Register with daemon
	agentID, isNew, err := worker.Register(context.Background())
	if err != nil {
		slog.Error("Failed to register agent", "error", err)
		os.Exit(1)
	}
	if isNew {
		slog.Info("Registered new agent", "agent_id", agentID)
	} else {
		slog.Info("Reconnected as existing agent", "agent_id", agentID)
	}

	// Heartbeat
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.HeartbeatLoop(ctx)

	// Start worker loop
	go worker.Run(ctx)

	// Block until signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	slog.Info("worker running. Press Ctrl+C to stop.")
	<-sigCh

	slog.Info("Shutting down...")
	cancel()
	slog.Info("worker stopped.")
}
