package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cmdStart handles "mnemosyne start" — starts daemon and agent services.
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

	home, _ := os.UserHomeDir()

	if isMacOS() {
		launchdDir := filepath.Join(home, "Library", "LaunchAgents")

		if !agentOnly {
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

		if !daemonOnly {
			if !agentOnly {
				fmt.Println("Waiting for daemon to be ready...")
				waitForDaemon(5 * time.Second)
			}
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
	}

	fmt.Println()
	fmt.Println("Services started. Use 'mnemosyne status' to verify.")
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
	launchdDir := filepath.Join(home, "Library", "LaunchAgents")

	// Stop agent
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

	// Stop daemon
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
	if isProcessRunning("mnemosyne-daemon") || isProcessRunning("mnemosyne", "daemon") {
		fmt.Println("● running")
	} else {
		fmt.Println("○ stopped")
	}

	// Check agent
	fmt.Print("Agent:   ")
	if isProcessRunning("mnemosyne-agent") || isProcessRunning("mnemosyne", "agent") {
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
	cmd := exec.Command("grpcurl", "-plaintext", "-max-time", "2", addr, "list")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
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