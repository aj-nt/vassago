// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aj-nt/mnemosyne/agent"
	"github.com/aj-nt/mnemosyne/agent/setup"
	iface "github.com/aj-nt/mnemosyne/agent/interface"
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

// cmdAgent handles "mnemosyne agent" — runs the agent process (server mode).
func cmdAgent(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to agent config (default: ~/.mnemosyne/agent.yaml)")
	serve := fs.Bool("serve", false, "Server mode: start gRPC chat server and wait")
	fs.Parse(args)

	if *configPath == "" {
		home, _ := os.UserHomeDir()
		*configPath = filepath.Join(home, ".mnemosyne", "agent.yaml")
	}

	ctx := context.Background()

	// Check daemon is running before starting agent
	c, err := memoclient.Connect(ctx, memoclient.Config{Address: "localhost:50051"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot connect to Mnemosyne daemon.\n")
		fmt.Fprintf(os.Stderr, "Make sure the daemon is running: mnemosyne start\n")
		os.Exit(1)
	}
	c.Close()

	result, err := setup.InitAgent(ctx, *configPath)
	if err != nil {
		log.Fatalf("Failed to initialize agent: %v", err)
	}
	defer result.Close()

	if *serve {
		if result.Config.Mnemosyne.ChatListen == "" {
			log.Fatal("--serve requires chat_listen to be configured")
		}
		log.Printf("Agent running in server mode (gRPC chat on %s)", result.Config.Mnemosyne.ChatListen)
		setup.WaitForSignal()
		return
	}

	// Interactive mode via mnemosyne agent (without --serve)
	cli := iface.NewCLI(result.Agent)
	if err := cli.RunInteractive(ctx); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

// cmdChat handles "mnemosyne chat" — starts an interactive chat session.
func cmdChat(args []string) {
	configPath := ""
	message := ""

	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	fs.StringVar(&configPath, "config", "", "Path to agent config (default: ~/.mnemosyne/agent.yaml)")
	fs.StringVar(&message, "message", "", "Send a single message and exit (non-interactive)")
	fs.StringVar(&message, "m", "", "Shorthand for --message")
	fs.Parse(args)

	// Re-parse remaining args for any positional message
	remaining := fs.Args()
	if message == "" && len(remaining) > 0 {
		message = strings.Join(remaining, " ")
	}

	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".mnemosyne", "agent.yaml")
	}

	// Check if help was requested
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println(`Usage: mnemosyne chat [options] [message]

Options:
  --config PATH    Path to agent config (default: ~/.mnemosyne/agent.yaml)
  --message, -m    Send a single message and exit (non-interactive)
  --help, -h       Show this help

Description:
  Start an interactive chat session with the Mnemosyne agent.
  If a message is provided, sends it and exits.

  The daemon must be running (use 'mnemosyne start').

Examples:
  mnemosyne chat                    # Interactive REPL
  mnemosyne chat "What's the time?" # One-shot query
  mnemosyne chat -m "Check memory"  # Same, with flag`)
			return
		}
	}

	// Verify the daemon is running first
	// Read daemon address from agent config if possible, fall back to default
	daemonAddr := "localhost:50051"
	if agentCfg, err := agent.LoadConfig(configPath); err == nil {
		if agentCfg.Mnemosyne.Address != "" {
			daemonAddr = agentCfg.Mnemosyne.Address
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := memoclient.Connect(ctx, memoclient.Config{Address: daemonAddr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot connect to Mnemosyne daemon at %s.\n", daemonAddr)
		fmt.Fprintf(os.Stderr, "Make sure the daemon is running: mnemosyne start\n")
		os.Exit(1)
	}
	c.Close()

	// Initialize the agent natively (no subprocess)
	agentCtx := context.Background()
	result, err := setup.InitAgent(agentCtx, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize agent: %v\n", err)
		os.Exit(1)
	}
	defer result.Close()

	cli := iface.NewCLI(result.Agent)

	if message != "" {
		if err := cli.RunOneShot(agentCtx, message); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Check if stdin has data (piped input)
		stat, _ := os.Stdin.Stat()
		if stat.Mode()&os.ModeCharDevice == 0 {
			var input strings.Builder
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				input.WriteString(scanner.Text())
				input.WriteString("\n")
			}
			msg := strings.TrimSpace(input.String())
			if msg != "" {
				if err := cli.RunOneShot(agentCtx, msg); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
		} else {
			if err := cli.RunInteractive(agentCtx); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	}
}

// findStandaloneBinary searches for a standalone binary in common locations.
func findStandaloneBinary(name string) string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "bin", name),
		filepath.Join(home, "go", "bin", name),
	}

	if p, err := findBinaryInPATHHelper(name); err == nil {
		return p
	}

	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

// findBinaryInPATHHelper wraps exec.LookPath.
func findBinaryInPATHHelper(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return p, nil
}

// execCommand creates an exec.Cmd.
func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// runBinaryForeground runs a binary with stdout/stderr attached.
func runBinaryForeground(name string, args ...string) {
	cmd := execCommand(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}