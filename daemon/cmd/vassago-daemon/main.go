// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/aj-nt/vassago/daemon/internal/daemon"
)

func main() {
	// Suppress gRPC C-core INFO logs about fork() handlers.
	os.Setenv("GRPC_VERBOSITY", "ERROR")

	configPath := flag.String("config", "", "Path to vassago.yaml (default: ~/.vassago/config.yaml)")
	flag.Parse()

	// Load config (use defaults for logging setup if config doesn't exist yet)
	var cfg *daemon.Config
	var err error
	if *configPath != "" {
		cfg, err = daemon.LoadConfig(*configPath)
	} else {
		cfg = daemon.DefaultConfig()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Set up logging before anything else
	setupLogging(cfg)

	slog.Info("vassago-daemon starting", "version", "0.1.0")

	// Create and run daemon
	d, err := daemon.New(cfg)
	if err != nil {
		slog.Error("failed to create daemon", "error", err)
		os.Exit(1)
	}

	if err := d.Run(); err != nil {
		slog.Error("daemon exited with error", "error", err)
		os.Exit(1)
	}
}

// setupLogging configures slog to write to the configured log file AND stderr.
// The log file is rotated by launchd, not us — we just open for append.
func setupLogging(cfg *daemon.Config) {
	logPath := cfg.Logging.Path
	if logPath == "" {
		home, _ := os.UserHomeDir()
		logPath = filepath.Join(home, ".vassago", "daemon.log")
	}

	// Ensure log directory exists
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log directory: %v\n", err)
	}

	// Open log file for append
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		// Log file unavailable — fall back to stderr-only, but don't crash
		fmt.Fprintf(os.Stderr, "failed to open log file %s: %v — logging to stderr only\n", logPath, err)
		return
	}

	// Multi-writer: both file and stderr
	w := io.MultiWriter(os.Stderr, f)

	// Parse log level
	level := slog.LevelInfo
	switch cfg.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))

	slog.Info("logging initialized", "path", logPath, "level", level.String())
}
