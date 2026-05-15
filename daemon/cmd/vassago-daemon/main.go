// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/aj-nt/vassago/daemon/internal/daemon"
)

func main() {
	// Suppress gRPC C-core INFO logs about fork() handlers.
	// These fire whenever a process with active gRPC threads calls
	// exec.Command (e.g., iMessage adapter, daemon spawn).
	os.Setenv("GRPC_VERBOSITY", "ERROR")

	configPath := flag.String("config", "", "Path to vassago.yaml (default: ~/.vassago/vassago.yaml)")
	flag.Parse()

	// Load config
	var err error
	var cfg *daemon.Config
	if *configPath != "" {
		cfg, err = daemon.LoadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
			os.Exit(1)
		}
	} else {
		cfg = daemon.DefaultConfig()
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
