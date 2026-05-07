// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	memoclient "github.com/aj-nt/mnemosyne/client"
)
func cmdStart(args []string) {
	daemonOnly := false
	agentOnly := false

	for _, arg := range args {
		switch arg {
		case "--daemon-only":
			daemonOnly = true
		case "--agent-only":
			agentOnly = true
		case "--help", "-h":
			fmt.Println(`Usage: mnemosyne start [options]

Options:
  --daemon-only   Start only the daemon
  --agent-only    Start only the agent
  --help, -h      Show this help

Description:
  Starts the Mnemosyne daemon and agent services.
  On macOS, uses launchctl if plists exist; otherwise starts
  the binaries in the background.`)
			return
		}
	}

	// Pre-flight: check configs exist
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".mnemosyne")
	daemonConfig := filepath.Join(dir, "config.yaml")
	agentConfig := filepath.Join(dir, "agent.yaml")

	if !fileExists(daemonConfig) || !fileExists(agentConfig) {
		fmt.Fprintf(os.Stderr, "Error: Configuration not found. Run 'mnemosyne init' first.\n")
		os.Exit(1)
	}

	if isMacOS() {
		launchdDir := filepath.Join(home, "Library", "LaunchAgents")

		if !agentOnly {
			startDaemon(home, launchdDir)
		}

		if !daemonOnly {
			if !agentOnly {
				fmt.Println("Waiting for daemon to be ready...")
				if !waitForDaemonWithConfig(dir, 10*time.Second) {
					fmt.Fprintf(os.Stderr, "  ⚠ Daemon didn't respond within 10s. Check logs:\n")
					fmt.Fprintf(os.Stderr, "    cat %s/daemon.err\n", dir)
					os.Exit(1)
				}
				fmt.Println("  ✓ Daemon is healthy")
			}
			startAgent(home, launchdDir)
		}
	}

	// Verify services are running
	fmt.Println()
	fmt.Println("Verifying services...")
	time.Sleep(500 * time.Millisecond) // brief settle

	allGood := true
	if !agentOnly {
		if isDaemonRunning() {
			fmt.Println("  ✓ Daemon running")
		} else {
			fmt.Println("  ✗ Daemon not running")
			allGood = false
		}
	}
	if !daemonOnly {
		if isAgentRunning() {
			fmt.Println("  ✓ Agent running")
		} else {
			fmt.Println("  ✗ Agent not running")
			allGood = false
		}
	}

	// gRPC health check
	grpcURL := findGRPCURL(dir)
	if checkGRPCHealth(grpcURL) {
		fmt.Println("  ✓ gRPC healthy")
	} else {
		fmt.Println("  ✗ gRPC unreachable")
		allGood = false
	}

	fmt.Println()
	if allGood {
		fmt.Println("All services started. Try: mnemosyne chat")
	} else {
		fmt.Fprintf(os.Stderr, "Some services failed to start. Check logs in %s/\n", dir)
		os.Exit(1)
	}
}

// cmdStop handles "mnemosyne stop" — stops daemon and agent services.
func cmdStop(args []string) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println(`Usage: mnemosyne stop [options]

Options:
  --help, -h   Show this help

Description:
  Stops the Mnemosyne daemon and agent services.
  On macOS, uses launchctl to unload services.`)
			return
		}
	}

	home, _ := os.UserHomeDir()

	// Stop agent
	stopAgent(home)

	// Stop daemon
	stopDaemon(home)

	fmt.Println("\nServices stopped.")
}

// cmdStatus handles "mnemosyne status" — shows service status.
func cmdStatus(args []string) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println(`Usage: mnemosyne status

Description:
  Shows the status of Mnemosyne daemon and agent services.
  Attempts to connect to the daemon's health endpoint.`)
			return
		}
	}

	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".mnemosyne")

	fmt.Println("Mnemosyne Status")
	fmt.Println("═════════════════")
	fmt.Println()

	// Check daemon
	fmt.Print("Daemon:  ")
	if isDaemonRunning() {
		fmt.Println("● running")
	} else {
		fmt.Println("○ stopped")
	}

	// Check agent
	fmt.Print("Agent:   ")
	if isAgentRunning() {
		fmt.Println("● running")
	} else {
		fmt.Println("○ stopped")
	}

	// Check gRPC connectivity
	fmt.Print("gRPC:    ")
	grpcURL := findGRPCURL(dir)
	if checkGRPCHealth(grpcURL) {
		fmt.Println("● healthy")
	} else {
		fmt.Println("○ unreachable")
	}

	fmt.Println()

	// Show database info
	dbPath := filepath.Join(dir, "mnemosyne.db")
	if info, err := os.Stat(dbPath); err == nil {
		fmt.Printf("Database: %s (%.1f MB)\n", dbPath, float64(info.Size())/(1024*1024))
	} else {
		fmt.Printf("Database: %s (not found)\n", dbPath)
	}

	// Show config status
	for _, f := range []string{"config.yaml", "agent.yaml"} {
		path := filepath.Join(dir, f)
		if fileExists(path) {
			fmt.Printf("Config:   %s ✓\n", path)
		} else {
			fmt.Printf("Config:   %s ✗ (missing — run 'mnemosyne init')\n", path)
		}
	}
}

func startDaemon(home string, launchdDir string) {
	daemonPlist := filepath.Join(launchdDir, "com.mnemosyne.daemon.plist")
	if fileExists(daemonPlist) {
		fmt.Println("Starting daemon via launchd...")
		runIgnoreErr("launchctl", "unload", daemonPlist)
		runIgnoreErr("launchctl", "load", "-w", daemonPlist)
		fmt.Println("  ✓ Daemon started")
	} else {
		fmt.Println("Starting daemon in background...")
		startBackgroundProcess(home, "daemon")
		fmt.Println("  ✓ Daemon started")
	}
}

func stopDaemon(home string) {
	launchdDir := filepath.Join(home, "Library", "LaunchAgents")
	daemonPlist := filepath.Join(launchdDir, "com.mnemosyne.daemon.plist")
	if fileExists(daemonPlist) {
		fmt.Println("Stopping daemon via launchd...")
		runIgnoreErr("launchctl", "unload", daemonPlist)
		fmt.Println("  ✓ Daemon stopped")
	} else {
		fmt.Println("Stopping daemon process...")
		runIgnoreErr("pkill", "-f", "mnemosyne.*daemon")
		fmt.Println("  ✓ Daemon stopped")
	}
}

func isDaemonRunning() bool {
	return isProcessRunning("mnemosyne-daemon") || isProcessRunning("mnemosyne", "daemon")
}

func startAgent(home string, launchdDir string) {
	agentPlist := filepath.Join(launchdDir, "com.mnemosyne.agent.plist")
	if fileExists(agentPlist) {
		fmt.Println("Starting agent via launchd...")
		runIgnoreErr("launchctl", "unload", agentPlist)
		runIgnoreErr("launchctl", "load", "-w", agentPlist)
		fmt.Println("  ✓ Agent started")
	} else {
		fmt.Println("Starting agent in background...")
		startBackgroundProcess(home, "agent")
		fmt.Println("  ✓ Agent started")
	}
}

func stopAgent(home string) {
	launchdDir := filepath.Join(home, "Library", "LaunchAgents")
	agentPlist := filepath.Join(launchdDir, "com.mnemosyne.agent.plist")
	if fileExists(agentPlist) {
		fmt.Println("Stopping agent via launchd...")
		runIgnoreErr("launchctl", "unload", agentPlist)
		fmt.Println("  ✓ Agent stopped")
	} else {
		fmt.Println("Stopping agent process...")
		runIgnoreErr("pkill", "-f", "mnemosyne.*agent.*--serve")
		fmt.Println("  ✓ Agent stopped")
	}
}

func isAgentRunning() bool {
	return isProcessRunning("mnemosyne-agent") || isProcessRunning("mnemosyne", "agent")
}

func startBackgroundProcess(home string, component string) {
	var bin string
	var configArg string

	if component == "daemon" {
		bin = findStandaloneBinary("mnemosyne-daemon")
		configArg = filepath.Join(home, ".mnemosyne", "config.yaml")
	} else {
		bin = findStandaloneBinary("mnemosyne-agent")
		configArg = filepath.Join(home, ".mnemosyne", "agent.yaml")
	}

	if bin == "" {
		// Try the unified binary
		bin = findStandaloneBinary("mnemosyne")
		if bin != "" && component == "daemon" {
			cmd := exec.Command(bin, "daemon", "--config", configArg)
			cmd.Start()
			return
		}
		fmt.Fprintf(os.Stderr, "  ✗ Cannot find %s binary\n", component)
		return
	}

	var cmd *exec.Cmd
	if component == "daemon" {
		cmd = exec.Command(bin, "--config", configArg)
	} else {
		cmd = exec.Command(bin, "--serve", "--config", configArg)
	}
	devNull, _ := os.Open(os.DevNull)
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.Start()
}

func runIgnoreErr(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Run()
}

func isProcessRunning(pattern string, matchArg ...string) bool {
	cmd := exec.Command("pgrep", "-fl", pattern)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	if len(matchArg) > 0 {
		return strings.Contains(string(output), matchArg[0])
	}
	return len(output) > 0
}

func checkGRPCHealth(addr string) bool {
	// Try native gRPC connection first (no external dependency)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, err := memoclient.Connect(ctx, memoclient.Config{Address: addr})
	if err != nil {
		return false
	}
	defer c.Close()
	if err := c.HealthCheck(ctx); err != nil {
		return false
	}
	return true
}

func waitForDaemon(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if checkGRPCHealth("localhost:50051") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println("  ⚠ Daemon didn't respond within timeout (may still be starting)")
}

// waitForDaemonWithConfig waits for the daemon to respond, reading the address from config.
func waitForDaemonWithConfig(dir string, timeout time.Duration) bool {
	grpcURL := findGRPCURL(dir)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if checkGRPCHealth(grpcURL) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func findGRPCURL(dir string) string {
	// Try to read daemon config for TCP address
	configPath := filepath.Join(dir, "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		// Simple grep for tcp: line — not a full YAML parse, just quick detection
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "tcp:") {
				addr := strings.TrimSpace(strings.TrimPrefix(line, "tcp:"))
				addr = strings.Trim(addr, `"' `)
				if addr != "" {
					return addr
				}
			}
		}
	}
	return "localhost:50051"
}