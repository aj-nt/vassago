package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	memoclient "github.com/aj-nt/mnemosyne/client"
)

// cmdDaemon handles "mnemosyne daemon" — runs the daemon process.
func cmdDaemon(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to daemon config (default: ~/.mnemosyne/config.yaml)")
	fs.Parse(args)

	if *configPath == "" {
		home, _ := os.UserHomeDir()
		*configPath = filepath.Join(home, ".mnemosyne", "config.yaml")
	}

	daemonBin := findStandaloneBinary("mnemosyne-daemon")
	if daemonBin != "" {
		runBinaryForeground(daemonBin, "--config", *configPath)
	} else {
		fmt.Fprintf(os.Stderr, "Error: mnemosyne-daemon binary not found.\n")
		fmt.Fprintf(os.Stderr, "Build it with: go build -o ~/bin/mnemosyne-daemon ./daemon/cmd/mnemosyne-daemon\n")
		os.Exit(1)
	}
}

// cmdAgent handles "mnemosyne agent" — runs the agent process.
func cmdAgent(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to agent config (default: ~/.mnemosyne/agent.yaml)")
	serve := fs.Bool("serve", false, "Server mode: start gRPC chat server and wait")
	message := fs.String("message", "", "Single message (non-interactive)")
	fs.Parse(args)

	if *configPath == "" {
		home, _ := os.UserHomeDir()
		*configPath = filepath.Join(home, ".mnemosyne", "agent.yaml")
	}

	agentBin := findStandaloneBinary("mnemosyne-agent")
	if agentBin != "" {
		agentArgs := []string{"--config", *configPath}
		if *serve {
			agentArgs = append(agentArgs, "--serve")
		}
		if *message != "" {
			agentArgs = append(agentArgs, "--message", *message)
		}
		runBinaryForeground(agentBin, agentArgs...)
	} else {
		fmt.Fprintf(os.Stderr, "Error: mnemosyne-agent binary not found.\n")
		fmt.Fprintf(os.Stderr, "Build it with: go build -o ~/bin/mnemosyne-agent ./agent/cmd/mnemosyne-agent\n")
		os.Exit(1)
	}
}

// cmdChat handles "mnemosyne chat" — starts an interactive chat session.
func cmdChat(args []string) {
	configPath := ""
	message := ""

	for i, arg := range args {
		switch arg {
		case "--config":
			if i+1 < len(args) {
				configPath = args[i+1]
			}
		case "--message", "-m":
			if i+1 < len(args) {
				message = args[i+1]
			}
		case "--help", "-h":
			fmt.Println(`Usage: mnemosyne chat [options]

Options:
  --config PATH   Path to agent config (default: ~/.mnemosyne/agent.yaml)
  --message, -m   Send a single message and exit (non-interactive)
  --help, -h      Show this help

Description:
  Start an interactive chat session with the Mnemosyne agent.
  If --message is provided, sends the message and exits.

  The daemon must be running (use 'mnemosyne start').`)
			return
		}
	}

	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".mnemosyne", "agent.yaml")
	}

	// Verify the daemon is running first
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := memoclient.Connect(ctx, memoclient.Config{Address: "localhost:50051"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot connect to Mnemosyne daemon at localhost:50051.\n")
		fmt.Fprintf(os.Stderr, "Make sure the daemon is running: mnemosyne start\n")
		os.Exit(1)
	}
	c.Close()

	// Delegate to the agent binary for interactive chat
	agentBin := findStandaloneBinary("mnemosyne-agent")
	if agentBin != "" {
		agentArgs := []string{"--config", configPath}
		if message != "" {
			agentArgs = append(agentArgs, "--message", message)
		}
		// Run interactively (pass through stdin/stdout/stderr)
		cmd := exec.Command(agentBin, agentArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	} else {
		fmt.Fprintf(os.Stderr, "Error: mnemosyne-agent binary not found.\n")
		fmt.Fprintf(os.Stderr, "Build it with: go build -o ~/bin/mnemosyne-agent ./agent/cmd/mnemosyne-agent\n")
		os.Exit(1)
	}
}

// findStandaloneBinary searches for a standalone binary in common locations.
func findStandaloneBinary(name string) string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "bin", name),
		filepath.Join(home, "go", "bin", name),
	}

	// Also check PATH
	if p, err := exec.LookPath(name); err == nil {
		return p
	}

	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

// runBinaryForeground runs a binary with stdout/stderr attached.
func runBinaryForeground(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}