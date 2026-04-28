package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/aj-nt/mnemosyne/daemon/internal/daemon"
)

func main() {
	configPath := flag.String("config", "", "Path to mnemosyne.yaml (default: ~/.mnemosyne/mnemosyne.yaml)")
	flag.Parse()

	// Load config
	cfg := daemon.DefaultConfig()
	if *configPath != "" {
		// TODO: Parse YAML config file, override defaults
		slog.Info("config file specified but YAML parsing not yet implemented", "path", *configPath)
	}

	// Create and run daemon
	d, err := daemon.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create daemon: %v\n", err)
		os.Exit(1)
	}

	if err := d.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
}