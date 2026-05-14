// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"fmt"
	"os"
)

func main() {
	// Suppress gRPC C-core INFO logs about fork() handlers.
	os.Setenv("GRPC_VERBOSITY", "ERROR")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "init":
		cmdInit(args)
	case "start":
		cmdStart(args)
	case "stop":
		cmdStop(args)
	case "status":
		cmdStatus(args)
	case "daemon":
		cmdDaemon(args)
	case "config":
		cmdConfig(args)
	case "memory":
		cmdMemory(args)
	case "export":
		cmdMemoryExport(args)
	case "import":
		cmdMemoryImport(args)
	case "hot-block":
		cmdHotBlock(args)
	case "sessions":
		cmdSessions(args)
	case "health":
		cmdHealth(args)
	case "version":
		cmdVersion(args)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Mnemosyne — Shared memory daemon for AI agents

Usage:
  mnemosyne <command> [arguments]

Commands:
  init            First-run setup (create dirs, configs)
  start           Start the daemon service
  stop            Stop the daemon service
  status          Show daemon status
  daemon          Run the Mnemosyne daemon (foreground)
  memory          Memory management (add, get, search, list, remove, stats)
  export          Export all memories to JSON file
  import          Import memories from JSON file (with backup)
  hot-block       Show the computed hot block
  sessions        Session management (list, resume, get)
  health          Health check for the daemon
  config          Configuration management (validate, show, edit, dirs)
  version         Show version information
  help            Show this help message

Getting Started:
  mnemosyne init          # Set up directories and configs
  mnemosyne start         # Start the daemon
  mnemosyne status        # Check daemon is running

Use "mnemosyne <command> --help" for more information about a command.`)
}
