package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// cmdInit handles "mnemosyne init" — first-run interactive setup.
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
	daemonConfigPath, agentConfigPath, personalityPath := getConfigPaths(dir)
	daemonConfigExists, agentConfigExists, personalityExists := checkExistingConfigs(daemonConfigPath, agentConfigPath, personalityPath)

	// Step 2: Provider Detection
	primaryProvider, primaryModel, anthropicKey, openaiKey := handleProviderDetection(flags.nonInteractive, reader)

	// Step 3: API Key Entry (if missing)
	anthropicKey, openaiKey = handleAPIKeyEntry(flags.nonInteractive, primaryProvider, anthropicKey, openaiKey, reader)

	// Model selection
	primaryModel = handleModelSelection(flags.nonInteractive, primaryModel, reader)

	// Step 4: Tool Selection
	enabledTools, toolNames := handleToolSelection(flags.nonInteractive, reader)

	// Step 5: Personality
	_, personalityContent, personalityPath := handlePersonalitySelection(flags.nonInteractive, dir, reader)

	// Step 6: Write Daemon Config
	writeDaemonConfig(daemonConfigPath, dir, daemonConfigExists, flags.force)

	// Step 7: Write Agent Config
	writeAgentConfig(agentConfigPath, dir, primaryProvider, primaryModel, toolNames, agentConfigExists, flags.force)

	// Step 8: Write Personality
	writePersonalityFile(personalityPath, personalityContent, personalityExists, flags.force)

	// Step 9: launchd plists
	handleLaunchdPlists(dir)

	// Summary
	printSetupSummary(dir, daemonConfigPath, agentConfigPath, personalityPath, primaryProvider, primaryModel, enabledTools, anthropicKey, openaiKey)

	// API key persistence reminder
	handleAPIKeyPersistenceReminder(flags.nonInteractive, anthropicKey, openaiKey)
}

// parseInitFlags parses command line flags for init command
func parseInitFlags(args []string) struct {
	dir           string
	force         bool
	nonInteractive bool
	help          bool
} {
	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".mnemosyne")

	result := struct {
		dir           string
		force         bool
		nonInteractive bool
		help          bool
	}{
		dir:           defaultDir,
		force:         false,
		nonInteractive: false,
		help:          false,
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
	fmt.Println(`Usage: mnemosyne init [options]

Options:
  --dir PATH           Data directory (default: ~/.mnemosyne)
  --force, -f          Overwrite existing configs (default: skip)
  --non-interactive, -y  Accept all defaults, no prompts
  --help, -h           Show this help

Description:
  Sets up Mnemosyne for first use. Walks through:
  1. Data directory selection
  2. Provider & API key configuration
  3. Model selection
  4. Tool enable/disable
  5. Personality selection

  Use --non-interactive to accept all defaults without prompts.
  Use --force to regenerate configs that already exist.`)
}

// printSetupHeader prints the setup header
func printSetupHeader() {
	fmt.Println("╭─────────────────────────────────────────────╮")
	fmt.Println("│  Mnemosyne First-Run Setup                  │")
	fmt.Println("╰─────────────────────────────────────────────╯")
	fmt.Println()
}

// handleDataDirectorySelection handles data directory selection
func handleDataDirectorySelection(defaultDir string, force bool, nonInteractive bool, reader *bufio.Reader) string {
	dir := defaultDir
	if !nonInteractive && !force {
		fmt.Printf("Data directory [%s]: ", dir)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			dir = input
		}
		// Expand ~
		dir = expandHome(dir)
		fmt.Println()
	}
	return dir
}

// createDirectories creates necessary directories
func createDirectories(dir string) {
	subdirs := []string{
		"screenshots",
		"agents",
		"pastes",
		"personalities",
		"generated_images",
	}

	fmt.Println("Creating directories...")
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", dir, err)
		os.Exit(1)
	}
	fmt.Printf("  ✓ %s\n", dir)
	for _, sub := range subdirs {
		path := filepath.Join(dir, sub)
		if err := os.MkdirAll(path, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ %s/\n", sub)
	}
	fmt.Println()
}

// getConfigPaths returns paths for configuration files
func getConfigPaths(dir string) (string, string, string) {
	daemonConfigPath := filepath.Join(dir, "config.yaml")
	agentConfigPath := filepath.Join(dir, "agent.yaml")
	personalityPath := filepath.Join(dir, "personalities", "bartimaeus.md")
	return daemonConfigPath, agentConfigPath, personalityPath
}

// checkExistingConfigs checks if configuration files already exist
func checkExistingConfigs(daemonConfigPath, agentConfigPath, personalityPath string) (bool, bool, bool) {
	daemonConfigExists := fileExists(daemonConfigPath)
	agentConfigExists := fileExists(agentConfigPath)
	personalityExists := fileExists(personalityPath)
	return daemonConfigExists, agentConfigExists, personalityExists
}

// handleProviderDetection handles provider detection and selection
func handleProviderDetection(nonInteractive bool, reader *bufio.Reader) (string, string, string, string) {
	fmt.Println("══ Step 1: Provider Setup ══")
	fmt.Println()

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	openaiKey := os.Getenv("OPENAI_API_KEY")
	ollamaRunning := checkOllama()

	fmt.Println("Detecting providers...")
	if anthropicKey != "" {
		fmt.Println("  ✓ Anthropic API key found in ANTHROPIC_API_KEY")
	} else {
		fmt.Println("  ⊘ No Anthropic API key found")
	}
	if openaiKey != "" {
		fmt.Println("  ✓ OpenAI API key found in OPENAI_API_KEY")
	}
	if ollamaRunning {
		fmt.Println("  ✓ Ollama detected on localhost:11434")
	}
	fmt.Println()

	// Determine primary provider
	primaryProvider := "anthropic"
	primaryModel := "claude-sonnet-4-6"
	if anthropicKey == "" && openaiKey != "" {
		primaryProvider = "openai"
		primaryModel = "gpt-4o-mini"
	} else if anthropicKey == "" && ollamaRunning {
		primaryProvider = "openai"
		primaryModel = "glm-5.1:cloud"
	}

	if !nonInteractive {
		primaryProvider, primaryModel = promptForPrimaryProvider(primaryProvider, primaryModel, ollamaRunning, reader)
	}

	return primaryProvider, primaryModel, anthropicKey, openaiKey
}

// promptForPrimaryProvider prompts user to select primary provider
func promptForPrimaryProvider(defaultProvider, defaultModel string, ollamaRunning bool, reader *bufio.Reader) (string, string) {
	fmt.Println("Select primary provider:")
	fmt.Println("  1. anthropic  (Claude — recommended)")
	fmt.Println("  2. openai     (GPT-4o, Ollama)")
	fmt.Printf("  Choice [1]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	switch input {
	case "2", "openai":
		defaultProvider = "openai"
		defaultModel = "gpt-4o-mini"
		if ollamaRunning {
			defaultModel = "glm-5.1:cloud"
		}
	default:
		defaultProvider = "anthropic"
		defaultModel = "claude-sonnet-4-6"
	}
	fmt.Println()
	return defaultProvider, defaultModel
}

// handleAPIKeyEntry handles API key entry for missing keys
func handleAPIKeyEntry(nonInteractive bool, primaryProvider, anthropicKey, openaiKey string, reader *bufio.Reader) (string, string) {
	if !nonInteractive && anthropicKey == "" && primaryProvider == "anthropic" {
		anthropicKey = promptForAnthropicAPIKey(reader)
	}

	if !nonInteractive && openaiKey == "" && primaryProvider == "openai" && !checkOllama() {
		openaiKey = promptForOpenAIAPIKey(reader)
	}

	return anthropicKey, openaiKey
}

// promptForAnthropicAPIKey prompts user to enter Anthropic API key
func promptForAnthropicAPIKey(reader *bufio.Reader) string {
	fmt.Println("No Anthropic API key detected.")
	fmt.Println("Get one at: https://console.anthropic.com/settings/keys")
	fmt.Print("Enter Anthropic API key (or press Enter to skip): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		fmt.Println("  ✓ API key saved for this session")
		fmt.Println("  ⚠ To persist, add to your shell profile:")
			fmt.Printf("    export ANTHROPIC_API_KEY=\"%s\"\n", maskKey(input))
	} else {
		fmt.Println("  ⊘ Skipped — agent will not be able to use Anthropic")
	}
	fmt.Println()
	return input
}

// promptForOpenAIAPIKey prompts user to enter OpenAI API key
func promptForOpenAIAPIKey(reader *bufio.Reader) string {
	fmt.Println("No OpenAI API key detected.")
	fmt.Print("Enter OpenAI API key (or press Enter to skip): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		fmt.Println("  ✓ API key saved for this session")
	} else {
		fmt.Println("  ⊘ Skipped — set OPENAI_API_KEY to use OpenAI")
	}
	fmt.Println()
	return input
}

// handleModelSelection handles model selection
func handleModelSelection(nonInteractive bool, primaryModel string, reader *bufio.Reader) string {
	if !nonInteractive {
		fmt.Printf("Primary model [%s]: ", primaryModel)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			primaryModel = input
		}
		fmt.Println()
	}
	return primaryModel
}

// handleToolSelection handles tool selection process
func handleToolSelection(nonInteractive bool, reader *bufio.Reader) ([]string, []string) {
	fmt.Println("══ Step 2: Tool Selection ══")
	fmt.Println()

	type toolGroup struct {
		name        string
		description string
		defaultOn   bool
	}

	groups := []toolGroup{
		{"memory", "Memory tools (add, search, replace, remove, consolidate, hot block)", true},
		{"files", "File tools (read, search, write)", true},
		{"terminal", "Terminal execution", true},
		{"web", "Web tools (search, fetch URL)", true},
		{"sessions", "Session management (search, list, get)", true},
		{"todos", "Todo management (add, list, complete, remove)", true},
		{"cron", "Cron scheduling", true},
		{"messaging", "Messaging (send to Discord, Telegram, etc.)", true},
		{"delegation", "Sub-agent delegation", true},
		{"browse", "Browser automation (requires ChromeDP)", false},
		{"vision", "Image analysis (requires vision-capable model)", false},
		{"image_gen", "Image generation (requires DALL-E API key)", false},
		{"multi_agent", "Multi-agent registry and discovery", true},
	}

	enabledTools := []string{}
	if nonInteractive {
		// Accept defaults
		for _, g := range groups {
			if g.defaultOn {
				enabledTools = append(enabledTools, g.name)
			}
		}
		fmt.Println("Using default tool selection:")
	} else {
		fmt.Println("Select tools to enable (y/n, Enter = default):")
		fmt.Println()
	}

	for _, g := range groups {
		if nonInteractive {
			status := "enabled"
			if !g.defaultOn {
				status = "disabled"
			}
			fmt.Printf("  %-12s %s (%s)\n", g.name, g.description, status)
		} else {
			prompt := "n"
			if g.defaultOn {
				prompt = "y"
			}
			fmt.Printf("  %-12s %s [%s]: ", g.name, g.description, prompt)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))

			enable := g.defaultOn
			switch input {
			case "y", "yes":
				enable = true
			case "n", "no":
				enable = false
			}
			if enable {
				enabledTools = append(enabledTools, g.name)
			}
		}
	}
	fmt.Println()

	// Map tool groups to individual tool names
	toolNames := groupsToToolNames(enabledTools)
	return enabledTools, toolNames
}

// handlePersonalitySelection handles personality selection
func handlePersonalitySelection(nonInteractive bool, dir string, reader *bufio.Reader) (string, string, string) {
	fmt.Println("══ Step 3: Personality ══")
	fmt.Println()

	personalityChoice := "bartimaeus"
	personalityContent := defaultPersonality

	if !nonInteractive {
		personalityChoice, personalityContent = promptForPersonalityChoice(reader)
	}

	personalityPath := getPersonalityPath(personalityChoice, dir)
	return personalityChoice, personalityContent, personalityPath
}

// promptForPersonalityChoice prompts user to select personality
func promptForPersonalityChoice(reader *bufio.Reader) (string, string) {
	personalityChoice := "bartimaeus"
	personalityContent := defaultPersonality

	fmt.Println("Select personality:")
	fmt.Println("  1. bartimaeus  (direct, dry-witted — recommended)")
	fmt.Println("  2. custom     (write your own)")
	fmt.Printf("  Choice [1]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "2" || input == "custom" {
		personalityChoice = "custom"
		fmt.Println()
		fmt.Println("Enter personality description (multi-line, end with empty line):")
		var lines []string
		for {
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" && len(lines) > 0 {
				break
			}
			lines = append(lines, line)
		}
		if len(lines) > 0 {
			personalityContent = strings.Join(lines, "\n")
		}
	}
	fmt.Println()
	return personalityChoice, personalityContent
}

// getPersonalityPath returns the path for personality file
func getPersonalityPath(personalityChoice, dir string) string {
	if personalityChoice == "bartimaeus" {
		return filepath.Join(dir, "personalities", "bartimaeus.md")
	} else {
		return filepath.Join(dir, "personalities", "custom.md")
	}
}

// writeDaemonConfig writes daemon configuration file
func writeDaemonConfig(daemonConfigPath, dir string, daemonConfigExists, force bool) {
	if daemonConfigExists && !force {
		fmt.Printf("⊙ Daemon config already exists at %s (use --force to overwrite)\n", daemonConfigPath)
	} else {
		fmt.Printf("Writing daemon config to %s...\n", daemonConfigPath)
		daemonCfg := generateDaemonConfig(dir)
		data, err := yaml.Marshal(daemonCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling daemon config: %v\n", err)
			os.Exit(1)
		}
		header := "# Mnemosyne Daemon Configuration\n# Generated by mnemosyne init on " + time.Now().Format("2006-01-02") + "\n\n"
		if err := os.WriteFile(daemonConfigPath, []byte(header+string(data)), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing daemon config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("  ✓ Daemon config written")
	}
}

// writeAgentConfig writes agent configuration file
func writeAgentConfig(agentConfigPath, dir, primaryProvider, primaryModel string, toolNames []string, agentConfigExists, force bool) {
	if agentConfigExists && !force {
		fmt.Printf("⊙ Agent config already exists at %s (use --force to overwrite)\n", agentConfigPath)
	} else {
		fmt.Printf("Writing agent config to %s...\n", agentConfigPath)
		agentCfg := generateAgentConfig(dir, primaryProvider, primaryModel)
		// Override tools with user's selection
		agentCfg.Tools.Enabled = toolNames
		// Set approval_required based on enabled tools
		approvalRequired := []string{}
		for _, t := range toolNames {
			if t == "FileWrite" || t == "Execute" {
				approvalRequired = append(approvalRequired, t)
			}
		}
		agentCfg.Tools.ApprovalRequired = approvalRequired

		data, err := yaml.Marshal(agentCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling agent config: %v\n", err)
			os.Exit(1)
		}
		header := "# Mnemosyne Agent Configuration\n# Generated by mnemosyne init on " + time.Now().Format("2006-01-02") + "\n\n"
		if err := os.WriteFile(agentConfigPath, []byte(header+string(data)), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing agent config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("  ✓ Agent config written")
	}
}

// writePersonalityFile writes personality file
func writePersonalityFile(personalityPath, personalityContent string, personalityExists, force bool) {
	if personalityExists && !force {
		fmt.Printf("⊙ Personality file already exists at %s (use --force to overwrite)\n", personalityPath)
	} else {
		fmt.Printf("Writing personality file to %s...\n", personalityPath)
		if err := os.WriteFile(personalityPath, []byte(personalityContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing personality: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("  ✓ Personality written")
	}
}

// handleLaunchdPlists handles launchd plists creation on macOS
func handleLaunchdPlists(dir string) {
	if isMacOS() {
		plists("mnemosyne", dir)
	}
}

// printSetupSummary prints setup completion summary
func printSetupSummary(dir, daemonConfigPath, agentConfigPath, personalityPath, primaryProvider, primaryModel string, enabledTools []string, anthropicKey, openaiKey string) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  Mnemosyne Setup Complete")
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Data directory:  %s\n", dir)
	fmt.Printf("  Daemon config:   %s\n", daemonConfigPath)
	fmt.Printf("  Agent config:    %s\n", agentConfigPath)
	fmt.Printf("  Personality:     %s\n", personalityPath)
	fmt.Printf("  Provider:        %s (%s)\n", primaryProvider, primaryModel)
	fmt.Printf("  Tools:           %s\n", strings.Join(enabledTools, ", "))
	if anthropicKey != "" || openaiKey != "" {
		fmt.Println("  API keys:        detected in environment")
	} else {
		fmt.Println("  API keys:       none detected — set env vars before starting")
	}
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    1. Validate configs:        mnemosyne config validate")
	fmt.Println("    2. Start services:        mnemosyne start")
	fmt.Println("    3. Verify health:         mnemosyne health")
	fmt.Println("    4. Start chatting:        mnemosyne chat")
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
}

// handleAPIKeyPersistenceReminder handles reminder about API key persistence
func handleAPIKeyPersistenceReminder(nonInteractive bool, anthropicKey, openaiKey string) {
	if !nonInteractive && (anthropicKey != "" && os.Getenv("ANTHROPIC_API_KEY") == "") {
		fmt.Println()
		fmt.Println("  ⚠  API keys were entered for this session only.")
		fmt.Println("     Add them to your shell profile to persist:")
		if anthropicKey != "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
			fmt.Printf("     export ANTHROPIC_API_KEY=\"%s\"\n", maskKey(anthropicKey))
		}
	}
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