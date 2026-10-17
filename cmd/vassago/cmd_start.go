// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	memoclient "github.com/aj-nt/vassago-sdk/client"
)

func cmdStart(args []string) {
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			fmt.Println(`Usage: vassago start [options]

Options:
  --help, -h      Show this help

Description:
  Starts the Vassago daemon service.
  On macOS, uses launchctl if plists exist; otherwise starts
  the daemon binary in the background.`)
			return
		}
	}

	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".vassago")
	daemonConfig := filepath.Join(dir, "config.yaml")

	if !fileExists(daemonConfig) {
		fmt.Fprintf(os.Stderr, "Error: Configuration not found. Run 'vassago init' first.\n")
		os.Exit(1)
	}

	if isMacOS() {
		launchdDir := filepath.Join(home, "Library", "LaunchAgents")
		daemonPlist := filepath.Join(launchdDir, "com.vassago.daemon.plist")
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

	// Wait for daemon to be ready
	fmt.Println("Waiting for daemon to be ready...")
	if !waitForDaemonWithConfig(dir, 10*time.Second) {
		fmt.Fprintf(os.Stderr, "  ✗ Daemon didn't respond within 10s. Check logs:\n")
		fmt.Fprintf(os.Stderr, "    cat %s/daemon.err\n", dir)
		os.Exit(1)
	}
	fmt.Println("  ✓ Daemon is healthy")
	fmt.Println()

	// Verify
	fmt.Println("Verifying services...")
	time.Sleep(500 * time.Millisecond)

	allGood := true
	if isDaemonRunning() {
		fmt.Println("  ✓ Daemon running")
	} else {
		fmt.Println("  ✗ Daemon not running")
		allGood = false
	}

	grpcURL := findGRPCURL(dir)
	if checkGRPCHealth(grpcURL) {
		fmt.Println("  ✓ gRPC healthy")
	} else {
		fmt.Println("  ✗ gRPC unreachable")
		allGood = false
	}

	fmt.Println()
	if allGood {
		fmt.Println("Daemon started. Connect with: vassago status")
	} else {
		fmt.Fprintf(os.Stderr, "Some services failed to start. Check logs in %s/\n", dir)
		os.Exit(1)
	}
}

// cmdStop handles "vassago stop" — stops daemon.
func cmdStop(args []string) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println(`Usage: vassago stop [options]

Options:
  --help, -h   Show this help

Description:
  Stops the Vassago daemon service.
  On macOS, uses launchctl to unload services.`)
			return
		}
	}

	home, _ := os.UserHomeDir()
	launchdDir := filepath.Join(home, "Library", "LaunchAgents")
	daemonPlist := filepath.Join(launchdDir, "com.vassago.daemon.plist")
	if fileExists(daemonPlist) {
		fmt.Println("Stopping daemon via launchd...")
		runIgnoreErr("launchctl", "unload", daemonPlist)
		fmt.Println("  ✓ Daemon stopped")
	} else {
		fmt.Println("Stopping daemon process...")
		runIgnoreErr("pkill", "-f", "vassago.*daemon")
		fmt.Println("  ✓ Daemon stopped")
	}
	fmt.Println("\nDaemon stopped.")
}

// cmdStatus handles "vassago status" — shows daemon status.
func cmdStatus(args []string) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println(`Usage: vassago status

Description:
  Shows the status of the Vassago daemon.
  Attempts to connect to the daemon's health endpoint.`)
			return
		}
	}

	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".vassago")

	fmt.Println("Vassago Daemon Status")
	fmt.Println("══════════════════════")
	fmt.Println()

	fmt.Print("Daemon:  ")
	if isDaemonRunning() {
		fmt.Println("● running")
	} else {
		fmt.Println("○ stopped")
	}

	fmt.Print("gRPC:    ")
	grpcURL := findGRPCURL(dir)
	if checkGRPCHealth(grpcURL) {
		fmt.Println("● healthy")
	} else {
		fmt.Println("○ unreachable")
	}

	fmt.Println()

	dbPath := filepath.Join(dir, "vassago.db")
	if info, err := os.Stat(dbPath); err == nil {
		fmt.Printf("Database: %s (%.1f MB)\n", dbPath, float64(info.Size())/(1024*1024))
	} else {
		fmt.Printf("Database: %s (not found)\n", dbPath)
	}

	for _, f := range []string{"config.yaml"} {
		path := filepath.Join(dir, f)
		if fileExists(path) {
			fmt.Printf("Config:   %s ✓\n", path)
		} else {
			fmt.Printf("Config:   %s ✗ (missing — run 'vassago init')\n", path)
		}
	}
}

func startBackgroundProcess(home string, component string) {
	bin := findStandaloneBinary("vassago-daemon")
	if bin == "" {
		bin = findStandaloneBinary("vassago")
		if bin != "" {
			configArg := filepath.Join(home, ".vassago", "config.yaml")
			cmd := exec.Command(bin, "daemon", "--config", configArg)
			cmd.Start()
			return
		}
		fmt.Fprintf(os.Stderr, "  ✗ Cannot find daemon binary\n")
		return
	}

	configArg := filepath.Join(home, ".vassago", "config.yaml")
	cmd := exec.Command(bin, "--config", configArg)
	devNull, _ := os.Open(os.DevNull)
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.Start()
}

func isDaemonRunning() bool {
	return isProcessRunning("vassago-daemon") || isProcessRunning("vassago", "daemon")
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
	configPath := filepath.Join(dir, "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
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
