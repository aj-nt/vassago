// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// cmdDaemon handles "vassago daemon" — runs the daemon process.
func cmdDaemon(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to daemon config (default: ~/.vassago/config.yaml)")
	fs.Parse(args)

	if *configPath == "" {
		home, _ := os.UserHomeDir()
		*configPath = filepath.Join(home, ".vassago", "config.yaml")
	}

	daemonBin := findStandaloneBinary("vassago-daemon")
	if daemonBin != "" {
		runBinaryForeground(daemonBin, "--config", *configPath)
	} else {
		fmt.Fprintf(os.Stderr, "Error: vassago-daemon binary not found.\n")
		fmt.Fprintf(os.Stderr, "Build it with: go build -o ~/bin/vassago-daemon ./daemon/cmd/vassago-daemon\n")
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
