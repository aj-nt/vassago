// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

// Orchestrator decomposes high-level goals into task DAGs via LLM, dispatches
// them to the Vassago daemon for worker execution, polls for completion,
// synthesizes results, and publishes a unified report.
//
// Usage:
//
//	orchestrator "audit the vassago codebase"
//	orchestrator --dry-run "review security posture"
//	orchestrator --addr=studio:50051 --repo=/path/to/project "analyze the codebase"
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aj-nt/vassago-sdk/client"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Show decomposition without dispatching tasks")
	addr := flag.String("addr", "", "Vassago daemon address (overrides config)")
	repo := flag.String("repo", "", "Repository path to analyze")
	timeout := flag.Int("timeout", 0, "Max wait for workers in seconds (overrides config)")

	flag.Parse()

	goal := flag.Arg(0)
	if goal == "" {
		slog.Error("Usage: orchestrator [flags] <goal>")
		os.Exit(1)
	}

	cfg := loadConfig()

	if *addr != "" {
		cfg.DaemonAddr = *addr
	}
	if *repo != "" {
		cfg.RepoPath = *repo
	}
	if *timeout > 0 {
		cfg.MaxWaitSec = *timeout
	}

	slog.Info("Orchestrator starting",
		"goal", goal,
		"daemon", cfg.DaemonAddr,
		"repo", cfg.RepoPath,
		"dry_run", *dryRun,
		"max_wait_sec", cfg.MaxWaitSec,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("Received shutdown signal")
		cancel()
	}()

	// Connect to daemon
	c, err := client.Connect(ctx, client.Config{Address: cfg.DaemonAddr})
	if err != nil {
		slog.Error("Failed to connect to daemon", "addr", cfg.DaemonAddr, "error", err)
		os.Exit(1)
	}
	defer c.Close()

	_ = c // used in Task 4 pipeline

	slog.Info("Connected to daemon", "addr", cfg.DaemonAddr)

	if *dryRun {
		slog.Info("Dry run mode — decomposition only (not yet implemented)")
		os.Exit(0)
	}

	slog.Info("Pipeline not yet implemented (Task 4)")
}
