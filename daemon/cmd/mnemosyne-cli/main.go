package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/aj-nt/mnemosyne/daemon/internal/daemon"
)

func main() {
	configPath := flag.String("config", "", "Path to mnemosyne.yaml")
	flag.Parse()

	cfg := daemon.DefaultConfig()
	if *configPath != "" {
		fmt.Fprintf(os.Stderr, "YAML parsing not yet implemented\n")
	}

	d, err := daemon.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create daemon: %v\n", err)
		os.Exit(1)
	}

	s := d.Store()
	fmt.Printf("Mnemosyne CLI — connected to %s\n\n", cfg.Store.Path)

	// TODO: Implement CLI commands:
	//   search <query>     — FTS5 search
	//   list [target]      — list memories
	//   hot-block [target] — show computed hot block
	//   add <target> <category> <key> <content> — add memory
	//   remove <id>       — remove memory
	//   stats             — memory statistics
	//   sessions          — list sessions
	//   agents            — list registered agents

	_ = s // Use store when CLI commands are implemented
	fmt.Println("CLI commands not yet implemented. Use gRPC client for now.")
}