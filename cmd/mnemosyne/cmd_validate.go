// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aj-nt/mnemosyne/agent"
)

// cmdConfigValidate handles "mnemosyne config validate" with deep validation.
func cmdConfigValidateDeep(args []string) {
	home, _ := os.UserHomeDir()
	daemonCfg := filepath.Join(home, ".mnemosyne", "config.yaml")
	agentCfg := filepath.Join(home, ".mnemosyne", "agent.yaml")

	// Allow --dir override
	for i, arg := range args {
		if arg == "--dir" && i+1 < len(args) {
			dir := args[i+1]
			daemonCfg = filepath.Join(dir, "config.yaml")
			agentCfg = filepath.Join(dir, "agent.yaml")
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
		// Deep validation: load through the daemon's actual config parser
		// Since we can't import daemon/internal, we do our own structural validation
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

	// === Agent Config Validation ===
	fmt.Println("═══ Agent Config ═══")
	fmt.Printf("File: %s\n", agentCfg)

	if !fileExists(agentCfg) {
		fmt.Println("  ✗ File not found — run 'mnemosyne init' to create")
		errors++
	} else {
		// Deep validation: load through agent.LoadConfig which calls Validate()
		errs := validateAgentConfigDeep(agentCfg)
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

// validateAgentConfigDeep loads the agent config through agent.LoadConfig
// which runs the full Validate() method. Also adds extra checks for things
// that Validate() doesn't cover (missing files, API keys, etc).
func validateAgentConfigDeep(path string) []ValidationIssue {
	var issues []ValidationIssue

	// Load through the actual agent parser — this runs Validate() internally
	cfg, err := agent.LoadConfig(path)
	if err != nil {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("Config validation failed: %v", err)})
		return issues
	}

	// --- Extra checks beyond Validate() ---
	// Personality file validation removed - personality is now embedded in config
	issues = append(issues, validateProviderAPIKeys(cfg)...)
	issues = append(issues, validateToolNames(cfg)...)
	issues = append(issues, validateAllowedDirs(cfg)...)
	issues = append(issues, validateMultiAgentRegistry(cfg)...)
	issues = append(issues, validateBrowserCacheDir(cfg)...)
	issues = append(issues, validateImageGenCacheDir(cfg)...)
	issues = append(issues, validateImageGenAPIKey(cfg)...)
	issues = append(issues, validateRoutingModels(cfg)...)

	return issues
}



// validateProviderAPIKeys checks if provider API keys are available
func validateProviderAPIKeys(cfg *agent.AgentConfig) []ValidationIssue {
	var issues []ValidationIssue
	
	if cfg.Provider.Name == "anthropic" || cfg.Provider.AnthropicAPIKeyEnv != "" {
		keyEnv := cfg.Provider.AnthropicAPIKeyEnv
		if keyEnv == "" {
			keyEnv = "ANTHROPIC_API_KEY"
		}
		if os.Getenv(keyEnv) == "" {
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("Primary provider is 'anthropic' but %s is not set", keyEnv),
			})
		}
	}

	if cfg.Provider.Name == "openai" || cfg.Provider.OpenAIAPIKeyEnv != "" {
		keyEnv := cfg.Provider.OpenAIAPIKeyEnv
		if keyEnv == "" {
			keyEnv = "OPENAI_API_KEY"
		}
		if os.Getenv(keyEnv) == "" && cfg.Provider.OpenAIBaseURL == "" {
			// Only warn for OpenAI proper (not Ollama which doesn't need keys)
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("OpenAI provider configured but %s is not set (Ollama URLs don't need this)", keyEnv),
			})
		}
	}
	
	return issues
}

// validateToolNames checks if tool names are recognized
func validateToolNames(cfg *agent.AgentConfig) []ValidationIssue {
	var issues []ValidationIssue
	knownTools := map[string]bool{
		"AddMemory": true, "SearchMemories": true, "ReplaceMemory": true,
		"RemoveMemory": true, "Consolidate": true, "GetHotBlock": true,
		"FileRead": true, "FileWrite": true, "FileSearch": true,
		"Execute": true,
		"SearchWeb": true, "FetchURL": true,
		"SearchSessions": true, "ListSessions": true, "GetSession": true,
		"AddTodo": true, "ListTodos": true, "CompleteTodo": true, "RemoveTodo": true,
		"SendMessage": true,
		"Delegate": true,
		"ScheduleCron": true,
		"Browse": true,
		"AnalyzeImage": true,
		"GenerateImage": true,
		"ListAgents": true, "RegisterAgent": true, "FindAgent": true,
	}
	for _, name := range cfg.Tools.Enabled {
		if !knownTools[name] {
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("Unknown tool name in tools.enabled: %q (will be ignored)", name),
			})
		}
	}

	for _, name := range cfg.Tools.ApprovalRequired {
		if !knownTools[name] {
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("Unknown tool name in tools.approval_required: %q", name),
			})
		}
	}
	
	return issues
}

// validateAllowedDirs checks if allowed_dirs exist
func validateAllowedDirs(cfg *agent.AgentConfig) []ValidationIssue {
	var issues []ValidationIssue
	for _, dir := range cfg.Tools.AllowedDirs {
		expanded := expandHome(dir)
		if !fileExists(expanded) {
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("tools.allowed_dirs entry doesn't exist: %s (resolved: %s)", dir, expanded),
			})
		}
	}
	return issues
}

// validateMultiAgentRegistry checks multi-agent registry path
func validateMultiAgentRegistry(cfg *agent.AgentConfig) []ValidationIssue {
	var issues []ValidationIssue
	if cfg.MultiAgent.Enabled && cfg.MultiAgent.RegistryPath != "" {
		// Registry file doesn't need to exist yet (created on first register)
		dir := filepath.Dir(cfg.MultiAgent.RegistryPath)
		if !fileExists(dir) {
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("multi_agent.registry_path parent directory doesn't exist: %s", dir),
			})
		}
	}
	return issues
}

// validateBrowserCacheDir checks browser cache directory
func validateBrowserCacheDir(cfg *agent.AgentConfig) []ValidationIssue {
	var issues []ValidationIssue
	if cfg.Browser.Enabled && cfg.Browser.CacheDir != "" {
		if !fileExists(cfg.Browser.CacheDir) {
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("browser.cache_dir doesn't exist: %s (will be created on first use)", cfg.Browser.CacheDir),
			})
		}
	}
	return issues
}

// validateImageGenCacheDir checks image generation cache directory
func validateImageGenCacheDir(cfg *agent.AgentConfig) []ValidationIssue {
	var issues []ValidationIssue
	if cfg.ImageGen.Enabled && cfg.ImageGen.CacheDir != "" {
		if !fileExists(cfg.ImageGen.CacheDir) {
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("image_gen.cache_dir doesn't exist: %s (will be created on first use)", cfg.ImageGen.CacheDir),
			})
		}
	}
	return issues
}

// validateImageGenAPIKey checks image generation API key
func validateImageGenAPIKey(cfg *agent.AgentConfig) []ValidationIssue {
	var issues []ValidationIssue
	if cfg.ImageGen.Enabled {
		keyEnv := cfg.ImageGen.APIKeyEnv
		if keyEnv == "" {
			keyEnv = "OPENAI_API_KEY"
		}
		if os.Getenv(keyEnv) == "" {
			issues = append(issues, ValidationIssue{
				"warning",
				fmt.Sprintf("image_gen is enabled but %s is not set", keyEnv),
			})
		}
	}
	return issues
}

// validateRoutingModels checks routing models configuration
func validateRoutingModels(cfg *agent.AgentConfig) []ValidationIssue {
	var issues []ValidationIssue
	if cfg.Routing.Strategy != "" {
		validStrategies := map[string]bool{
			"always-primary": true, "cost-optimized": true, "quality-optimized": true, "smart": true,
		}
		if !validStrategies[cfg.Routing.Strategy] {
			// This is caught by Validate(), but double-check
			issues = append(issues, ValidationIssue{
				"error",
				fmt.Sprintf("routing.strategy %q is not valid (use: always-primary, cost-optimized, quality-optimized, smart)", cfg.Routing.Strategy),
			})
		}

		// Check routing models reference valid providers
		for _, m := range cfg.Routing.Models {
			if m.Provider != "anthropic" && m.Provider != "openai" {
				issues = append(issues, ValidationIssue{
					"warning",
					fmt.Sprintf("routing.models entry %q references unknown provider %q (must be 'anthropic' or 'openai')", m.Name, m.Provider),
				})
			}
		}
	}
	return issues
}

// validateDaemonConfigDeep validates the daemon config structurally.
// We can't import daemon/internal/daemon, so we parse the YAML into a
// generic map and apply the same validation rules ourselves.
func validateDaemonConfigDeep(path string) []ValidationIssue {
	var issues []ValidationIssue

	// Try to parse YAML
	data, err := os.ReadFile(path)
	if err != nil {
		issues = append(issues, ValidationIssue{"error", fmt.Sprintf("Cannot read: %v", err)})
		return issues
	}

	var cfg map[string]interface{}
	if err := yamlUnmarshal(data, &cfg); err != nil {
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

// checkEnvironment validates runtime environment.
func checkEnvironment() []ValidationIssue {
	var issues []ValidationIssue

	// Check Anthropic API key
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		issues = append(issues, ValidationIssue{"warning", "ANTHROPIC_API_KEY not set — Anthropic provider unavailable"})
	}

	// Check OpenAI API key (only needed for non-local OpenAI)
	openaiKey := os.Getenv("OPENAI_API_KEY")
	ollamaKey := os.Getenv("OLLAMA_API_KEY")
	if openaiKey == "" && ollamaKey == "" {
		issues = append(issues, ValidationIssue{"warning", "Neither OPENAI_API_KEY nor OLLAMA_API_KEY set — OpenAI provider may need API key unless using Ollama"})
	}

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
		"Data":         base,
		"Database":    filepath.Join(base, "mnemosyne.db"),
		"Screenshots": filepath.Join(base, "screenshots"),
		"Agents":      filepath.Join(base, "agents"),
		"Pastes":      filepath.Join(base, "pastes"),
		"Personalities": filepath.Join(base, "personalities"),
		"Generated Images": filepath.Join(base, "generated_images"),
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

// expandHome is defined in cmd_init.go
func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// toInt converts an interface{} to int (YAML unmarshals numbers as int or float64).
func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}