// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cmdInit handles "vassago init" — first-run interactive setup.
func cmdInit(args []string) {
	flags := parseInitFlags(args)
	if flags.help {
		printInitHelp()
		return
	}

	reader := bufio.NewReader(os.Stdin)

	printSetupHeader()

	// Step 1: Data Directory
	dir := handleDataDirectorySelection(flags.dir, flags.force, flags.nonInteractive, reader)

	// Create directories
	createDirectories(dir)

	// Check for existing configs
	daemonConfigPath, agentConfigPath := getConfigPaths(dir)
	daemonConfigExists, agentConfigExists := checkExistingConfigs(daemonConfigPath, agentConfigPath)

	// Step 2: Provider Detection
	primaryProvider, primaryModel, anthropicKey, openaiKey := handleProviderDetection(flags.nonInteractive, reader)

	// Step 3: API Key Entry (if missing)
	anthropicKey, openaiKey = handleAPIKeyEntry(flags.nonInteractive, primaryProvider, anthropicKey, openaiKey, reader)

	// Model selection
	primaryModel = handleModelSelection(flags.nonInteractive, primaryModel, reader)

	// Step 4: Tool Selection
	enabledTools, toolNames := handleToolSelection(flags.nonInteractive, reader)

	// Step 5: Personality (now handled directly in config)
	// personalityContent is now embedded directly in the agent config

	// Step 6: Write Daemon Config
	writeDaemonConfig(daemonConfigPath, dir, daemonConfigExists, flags.force)

	// Step 7: Write Agent Config
	writeAgentConfig(agentConfigPath, dir, primaryProvider, primaryModel, toolNames, agentConfigExists, flags.force)

	// Step 8: Write Personality (deprecated - personality is now in config)

	// Step 9: launchd plists
	handleLaunchdPlists(dir)

	// Summary
	printSetupSummary(dir, daemonConfigPath, agentConfigPath, primaryProvider, primaryModel, enabledTools, anthropicKey, openaiKey)

	// API key persistence reminder
	handleAPIKeyPersistenceReminder(flags.nonInteractive, anthropicKey, openaiKey)
}

// parseInitFlags parses command line flags for init command
func parseInitFlags(args []string) struct {
	dir            string
	force          bool
	nonInteractive bool
	help           bool
} {
	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".vassago")

	result := struct {
		dir            string
		force          bool
		nonInteractive bool
		help           bool
	}{
		dir:            defaultDir,
		force:          false,
		nonInteractive: false,
		help:           false,
	}

	for i, arg := range args {
		switch arg {
		case "--dir":
			if i+1 < len(args) {
				result.dir = args[i+1]
			}
		case "--force", "-f":
			result.force = true
		case "--non-interactive", "-y":
			result.nonInteractive = true
		case "--help", "-h":
			result.help = true
		}
	}

	return result
}

// printInitHelp prints help information for init command
func printInitHelp() {
	fmt.Println(`Usage: vassago init [options]

Options:
  --dir PATH           Data directory (default: ~/.vassago)
  --force, -f          Overwrite existing configs (default: skip)
  --non-interactive, -y  Accept all defaults, no prompts
  --help, -h           Show this help

Description:
  Sets up Vassago for first use. Walks through:
  1. Data directory selection
  2. Provider & API key configuration
  3. Model selection
  4. Tool enable/disable

  Use --non-interactive to accept all defaults without prompts.
  Use --force to regenerate configs that already exist.`)
}

// printSetupHeader prints the setup header
func printSetupHeader() {
	fmt.Println("╭─────────────────────────────────────────────╮")
	fmt.Println("│  Vassago First-Run Setup                  │")
	fmt.Println("╰─────────────────────────────────────────────╯")
	fmt.Println()
}

// getConfigPaths returns paths for configuration files
func getConfigPaths(dir string) (string, string) {
	daemonConfigPath := filepath.Join(dir, "config.yaml")
	agentConfigPath := filepath.Join(dir, "agent.yaml")
	return daemonConfigPath, agentConfigPath
}

// checkExistingConfigs checks if configuration files already exist
func checkExistingConfigs(daemonConfigPath, agentConfigPath string) (bool, bool) {
	daemonConfigExists := fileExists(daemonConfigPath)
	agentConfigExists := fileExists(agentConfigPath)
	return daemonConfigExists, agentConfigExists
}

// groupsToToolNames maps enabled tool groups to individual tool names.
func groupsToToolNames(groups []string) []string {
	enabled := map[string]bool{}
	for _, g := range groups {
		enabled[g] = true
	}

	var names []string

	// Memory tools
	if enabled["memory"] {
		names = append(names,
			"AddMemory", "SearchMemories", "ReplaceMemory", "RemoveMemory",
			"Consolidate", "GetHotBlock",
		)
	}

	// File tools
	if enabled["files"] {
		names = append(names, "FileRead", "FileSearch", "FileWrite")
	}

	// Terminal
	if enabled["terminal"] {
		names = append(names, "Execute")
	}

	// Web tools
	if enabled["web"] {
		names = append(names, "SearchWeb", "FetchURL")
	}

	// Session tools
	if enabled["sessions"] {
		names = append(names, "SearchSessions", "ListSessions", "GetSession")
	}

	// Todo tools
	if enabled["todos"] {
		names = append(names, "AddTodo", "ListTodos", "CompleteTodo", "RemoveTodo")
	}

	// Cron tools
	if enabled["cron"] {
		names = append(names, "ScheduleCron")
	}

	// Messaging tools
	if enabled["messaging"] {
		names = append(names, "SendMessage")
	}

	// Delegation tools
	if enabled["delegation"] {
		names = append(names, "Delegate")
	}

	// Browser tools
	if enabled["browse"] {
		names = append(names, "Browse")
	}

	// Vision tools
	if enabled["vision"] {
		names = append(names, "AnalyzeImage")
	}

	// Image gen tools
	if enabled["image_gen"] {
		names = append(names, "GenerateImage")
	}

	// Multi-agent tools
	if enabled["multi_agent"] {
		names = append(names, "ListAgents", "RegisterAgent", "FindAgent")
	}

	return names
}

// maskKey returns a masked version of an API key for display.
func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// checkOllama tries to connect to Ollama on localhost:11434.
func checkOllama() bool {
	conn, err := net.DialTimeout("tcp", "localhost:11434", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// expandHome expands ~ to the user's home directory.
func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}
