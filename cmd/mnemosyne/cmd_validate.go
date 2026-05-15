// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// cmdConfigValidateDeep handles "mnemosyne config validate" with deep validation.
func cmdConfigValidateDeep(args []string) {
	home, _ := os.UserHomeDir()
	daemonCfg := filepath.Join(home, ".mnemosyne", "config.yaml")

	// Allow --dir override
	for i, arg := range args {
		if arg == "--dir" && i+1 < len(args) {
			dir := args[i+1]
			daemonCfg = filepath.Join(dir, "config.yaml")
		}
	}

	warnings := 0
	errors := 0

	// === Daemon Config Validation ===
	fmt.Println("═══ Daemon Config ═══")
	fmt.Printf("File: %s\n", daemonCfg)

	if !fileExists(daemonCfg) {
		fmt.Println("  ✗ File not found — run 'mnemosyne init' to create")
		errors++
	} else {
		errs := validateDaemonConfigDeep(daemonCfg)
		for _, e := range errs {
			if e.Severity == "error" {
				fmt.Printf("  ✗ %s\n", e.Message)
				errors++
			} else {
				fmt.Printf("  ⚠ %s\n", e.Message)
				warnings++
			}
		}
		if len(errs) == 0 {
			fmt.Println("  ✓ All checks passed")
		}
	}

	fmt.Println()

	// === Environment Checks ===
	fmt.Println("═══ Environment ═══")

	envChecks := checkEnvironment()
	for _, e := range envChecks {
		if e.Severity == "error" {
			fmt.Printf("  ✗ %s\n", e.Message)
			errors++
		} else {
			fmt.Printf("  ⚠ %s\n", e.Message)
			warnings++
		}
	}
	if len(envChecks) == 0 {
		fmt.Println("  ✓ All checks passed")
	}

	fmt.Println()

	// === Directory Checks ===
	fmt.Println("═══ Data Directories ═══")

	dirChecks := checkDataDirs(home)
	for _, e := range dirChecks {
		if e.Severity == "error" {
			fmt.Printf("  ✗ %s\n", e.Message)
			errors++
		} else {
			fmt.Printf("  ⚠ %s\n", e.Message)
			warnings++
		}
	}
	if len(dirChecks) == 0 {
		fmt.Println("  ✓ All directories exist")
	}

	fmt.Println()

	// Summary
	fmt.Println("───────────────────────────")
	if errors > 0 {
		fmt.Printf("  %d error(s), %d warning(s)\n", errors, warnings)
		os.Exit(1)
	} else if warnings > 0 {
		fmt.Printf("  ✓ No errors, %d warning(s)\n", warnings)
	} else {
		fmt.Println("  ✓ All checks passed — no errors or warnings")
	}
}

// ValidationIssue represents a config validation finding.
type ValidationIssue struct {
	Severity string // "error" or "warning"
	Message  string
}

// validateDaemonConfigDeep validates the daemon config structurally.
func validateDaemonConfigDeep(path string) []ValidationIssue {
	var issues []ValidationIssue

	data, err := os.ReadFile(path)
	if err != nil {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("Cannot read: %v", err)})
		return issues
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("YAML syntax error: %v", err)})
		return issues
	}

	// Check required sections
	daemon, _ := cfg["daemon"].(map[string]interface{})
	database, _ := cfg["database"].(map[string]interface{})
	hotBlock, _ := cfg["hot_block"].(map[string]interface{})

	socket, _ := daemon["socket"].(string)
	tcp, _ := daemon["tcp"].(string)
	if socket == "" && tcp == "" {
		issues = append(issues, ValidationIssue{"error", "daemon.socket or daemon.tcp must be configured"})
	}

	// TLS check
	tls, _ := daemon["tls"].(map[string]interface{})
	cert, _ := tls["cert"].(string)
	key, _ := tls["key"].(string)
	if (cert != "") != (key != "") {
		issues = append(issues, ValidationIssue{"error", "daemon.tls.cert and daemon.tls.key must both be set or both be empty"})
	}

	// Database
	dbPath, _ := database["path"].(string)
	if dbPath == "" {
		issues = append(issues, ValidationIssue{"error", "database.path is required"})
	} else {
		expanded := expandHome(dbPath)
		if !fileExists(filepath.Dir(expanded)) {
			issues = append(issues, ValidationIssue{"warning", fmt.Sprintf("database.path parent directory doesn't exist: %s", filepath.Dir(expanded))})
		}
	}

	busyTimeout := toInt(database["busy_timeout"])
	if busyTimeout != 0 && (busyTimeout < 1000 || busyTimeout > 30000) {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("database.busy_timeout must be between 1000 and 30000 ms, got %d", busyTimeout)})
	}

	// Hot block
	memLimit := toInt(hotBlock["memory_char_limit"])
	if memLimit != 0 && (memLimit < 1000 || memLimit > 20000) {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("hot_block.memory_char_limit must be between 1000 and 20000, got %d", memLimit)})
	}

	userLimit := toInt(hotBlock["user_char_limit"])
	if userLimit != 0 && (userLimit < 500 || userLimit > 10000) {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("hot_block.user_char_limit must be between 500 and 10000, got %d", userLimit)})
	}

	minPri := toInt(hotBlock["min_priority"])
	if minPri != 0 && (minPri < 1 || minPri > 5) {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("hot_block.min_priority must be between 1 and 5, got %d", minPri)})
	}

	obsTTL := toInt(hotBlock["observation_ttl_days"])
	if obsTTL != 0 && (obsTTL < 7 || obsTTL > 365) {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("hot_block.observation_ttl_days must be between 7 and 365, got %d", obsTTL)})
	}

	// Cron
	cron, _ := cfg["cron"].(map[string]interface{})
	cronEnabled, _ := cron["enabled"].(bool)
	cronInterval := toInt(cron["interval"])
	if cronEnabled && cronInterval != 0 && (cronInterval < 10 || cronInterval > 3600) {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("cron.interval must be between 10 and 3600 seconds, got %d", cronInterval)})
	}
	cronMaxJobs := toInt(cron["max_jobs"])
	if cronEnabled && cronMaxJobs != 0 && (cronMaxJobs < 1 || cronMaxJobs > 1000) {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("cron.max_jobs must be between 1 and 1000, got %d", cronMaxJobs)})
	}

	// Logging
	logging, _ := cfg["logging"].(map[string]interface{})
	logLevel, _ := logging["level"].(string)
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if logLevel != "" && !validLevels[logLevel] {
		issues = append(issues, ValidationIssue{"warning", fmt.Sprintf("logging.level %q is not standard (use: debug, info, warn, error)", logLevel)})
	}

	return issues
}

// checkEnvironment validates runtime environment (daemon-focused).
func checkEnvironment() []ValidationIssue {
	var issues []ValidationIssue

	// Check if daemon is reachable
	if !checkGRPCHealth("localhost:50051") {
		issues = append(issues, ValidationIssue{"warning", "Daemon not reachable at localhost:50051 — is it running? (mnemosyne start)"})
	}

	return issues
}

// checkDataDirs validates data directories exist.
func checkDataDirs(home string) []ValidationIssue {
	var issues []ValidationIssue

	base := filepath.Join(home, ".mnemosyne")
	dirs := map[string]string{
		"Data":     base,
		"Database": filepath.Join(base, "mnemosyne.db"),
	}

	for name, path := range dirs {
		if !fileExists(path) {
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("%s directory/file missing: %s (run 'mnemosyne init')", name, path),
			})
		}
	}

	return issues
}

// toInt converts an interface{} to int.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
