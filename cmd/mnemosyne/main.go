// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"fmt"
	"os"
)

func main() {
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
	case "agent":
		cmdAgent(args)
	case "chat":
		cmdChat(args)
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
	fmt.Println(`Mnemosyne — AI agent memory and management system

Usage:
  mnemosyne <command> [arguments]

Commands:
  init            First-run setup (create dirs, configs, personality)
  start           Start daemon and agent services
  stop            Stop daemon and agent services
  status          Show daemon and agent status
  daemon          Run the Mnemosyne daemon (background service)
  agent           Run the Mnemosyne agent (interactive or server)
  chat            Start an interactive chat session
  memory          Memory management (add, get, search, list, remove, stats)
  export           Export all memories to JSON file
  import           Import memories from JSON file (with backup)
  hot-block       Show the computed hot block
  sessions        Session management (list, resume, get)
  health          Health check for the daemon
  config          Configuration management (validate, show, edit, dirs)
  version         Show version information
  help            Show this help message

Getting Started:
  mnemosyne init          # Set up directories, configs, and personality
  mnemosyne start         # Start the daemon and agent
  mnemosyne chat          # Start chatting

Use "mnemosyne <command> --help" for more information about a command.`)
}