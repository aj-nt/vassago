// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

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
func promptForPrimaryProvider(_, defaultModel string, ollamaRunning bool, reader *bufio.Reader) (string, string) {
	fmt.Println("Select primary provider:")
	fmt.Println("  1. anthropic  (Claude — recommended)")
	fmt.Println("  2. openai     (GPT-4o, Ollama)")
	fmt.Printf("  Choice [1]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var provider string
	switch input {
	case "2", "openai":
		provider = "openai"
		defaultModel = "gpt-4o-mini"
		if ollamaRunning {
			defaultModel = "glm-5.1:cloud"
		}
	default:
		provider = "anthropic"
		defaultModel = "claude-sonnet-4-6"
	}
	fmt.Println()
	return provider, defaultModel
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
		fmt.Printf("    export ANTHROPIC_API_KEY=%s\n", maskKey(input))
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

// handlePersonalitySelection is removed - personality is now embedded in config.
// This function is kept as a minimal stub for backward compatibility.
func handlePersonalitySelection(nonInteractive bool, dir string, reader *bufio.Reader) (string, string, string) {
	return "", defaultPersonality, ""
}

// promptForPersonalityChoice is removed - personality is now embedded in config.
func promptForPersonalityChoice(reader *bufio.Reader) (string, string) {
	return "", defaultPersonality
}

// handleAPIKeyPersistenceReminder handles reminder about API key persistence
func handleAPIKeyPersistenceReminder(nonInteractive bool, anthropicKey, openaiKey string) {
	if !nonInteractive && (anthropicKey != "" && os.Getenv("ANTHROPIC_API_KEY") == "") {
		fmt.Println()
		fmt.Println("  ⚠  API keys were entered for this session only.")
		fmt.Println("     Add them to your shell profile to persist:")
		if anthropicKey != "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
			fmt.Printf("     export ANTHROPIC_API_KEY=%s\n", maskKey(anthropicKey))
		}
	}
}
