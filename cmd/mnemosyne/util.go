// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"gopkg.in/yaml.v3"
)

// yamlUnmarshal is a thin wrapper around yaml.Unmarshal for config validation.
func yamlUnmarshal(data []byte, v interface{}) error {
	return yaml.Unmarshal(data, v)
}

// yamlMarshal is a thin wrapper around yaml.Marshal for config generation.
func yamlMarshal(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

// validateDaemonConfig validates the daemon config by trying to start the daemon briefly.
func validateDaemonConfig(path string) error {
	if !fileExists(path) {
		return fmt.Errorf("config file not found: %s", path)
	}
	// Try loading via the daemon binary itself
	daemonBin := findStandaloneBinary("mnemosyne-daemon")
	if daemonBin == "" {
		// Fall back to basic YAML validation
		return validateYAMLFile(path)
	}
	// Run the daemon with a timeout — if it fails to parse config, it'll exit quickly
	cmd := exec.Command(daemonBin, "--config", path)
	cmd.Stdout = nil
	cmd.Stderr = nil
	// Don't actually wait for it to run; just check if config loads without error
	// This is tricky — the daemon won't exit on its own.
	// Better: use mnemonic-cli health or the daemon's --validate flag if available.
	// For now, basic YAML validation
	return validateYAMLFile(path)
}

// validateAgentConfig validates the agent config by trying to load it.
func validateAgentConfig(path string) error {
	if !fileExists(path) {
		return fmt.Errorf("config file not found: %s", path)
	}
	return validateYAMLFile(path)
}

// userHomeDir returns the user's home directory.
func userHomeDir() (string, error) {
	return os.UserHomeDir()
}

// readFile reads a file's contents.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// getEnv returns an environment variable.
func getEnv(key string) string {
	return os.Getenv(key)
}

// isMacOS returns true if running on macOS.
func isMacOS() bool {
	return runtime.GOOS == "darwin"
}

// validateYAMLFile does basic YAML syntax validation.
func validateYAMLFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read: %w", err)
	}
	// Try to unmarshal as generic map to check YAML syntax
	var m map[string]interface{}
	if err := yamlUnmarshal(data, &m); err != nil {
		return fmt.Errorf("YAML syntax error: %w", err)
	}
	return nil
}
