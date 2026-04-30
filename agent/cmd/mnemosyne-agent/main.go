package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/aj-nt/mnemosyne/agent"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
	iface "github.com/aj-nt/mnemosyne/agent/interface"
	memoclient "github.com/aj-nt/mnemosyne/client"
)

func main() {
	configPath := flag.String("config", "configs/agent-dev.yaml", "Path to agent config file")
	oneShot := flag.String("message", "", "Single message (non-interactive)")
	flag.Parse()

	// Load config
	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to Mnemosyne daemon
	ctx := context.Background()
	mnemo, err := memoclient.Connect(ctx, memoclient.Config{
		Address: cfg.Mnemosyne.Address,
	})
	if err != nil {
		log.Fatalf("Failed to connect to Mnemosyne daemon: %v", err)
	}
	defer mnemo.Close()

	// Wire providers and tools from config
	registry := setupProviders(&cfg.Provider)
	toolReg := setupTools(&cfg.Tools, mnemo, cfg.Agent.ID)

	// Create agent
	agentCfg := cfg.ToAgentConfig()
	a := agent.NewAgent(agentCfg, registry, toolReg, mnemo)
	defer a.Close()

	log.Printf("Starting agent %q with primary provider %q (model: %s)",
		cfg.Agent.ID, cfg.Provider.Name, cfg.Provider.Model)

	// Run
	cli := iface.NewCLI(a)
	if *oneShot != "" {
		if err := cli.RunOneShot(ctx, *oneShot); err != nil {
			log.Fatalf("Error: %v", err)
		}
	} else {
		// Check if stdin has data (piped input)
		stat, _ := os.Stdin.Stat()
		if stat.Mode()&os.ModeCharDevice == 0 {
			// Read all of stdin as a single message
			var input strings.Builder
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				input.WriteString(scanner.Text())
				input.WriteString("\n")
			}
			msg := strings.TrimSpace(input.String())
			if msg != "" {
				if err := cli.RunOneShot(ctx, msg); err != nil {
					log.Fatalf("Error: %v", err)
				}
			}
		} else {
			if err := cli.RunInteractive(ctx); err != nil {
				log.Fatalf("Error: %v", err)
			}
		}
	}
}

// setupProviders creates and configures the provider registry from config.
func setupProviders(cfg *agent.ProviderSection) *provider.ProviderRegistry {
	registry := provider.NewProviderRegistry()

	// OpenAI-compatible provider (local Ollama or remote OpenAI)
	openaiBaseURL := firstNonEmpty(cfg.OpenAIBaseURL, cfg.BaseURL, "https://api.openai.com/v1")
	openaiModel := firstNonEmpty(cfg.OpenAIModel, cfg.Model)
	openaiKeyEnv := firstNonEmpty(cfg.OpenAIAPIKeyEnv, cfg.APIKeyEnv, "OPENAI_API_KEY")
	openaiAPIKey := os.Getenv(openaiKeyEnv)

	registry.Register("openai", provider.NewOpenAIProvider(openaiBaseURL, openaiAPIKey, openaiModel))

	// Anthropic provider — only if API key is available
	anthropicKeyEnv := firstNonEmpty(cfg.AnthropicAPIKeyEnv, "ANTHROPIC_API_KEY")
	if apiKey := os.Getenv(anthropicKeyEnv); apiKey != "" {
		anthropicModel := firstNonEmpty(cfg.AnthropicModel, "claude-sonnet-4-6")
		registry.Register("anthropic", provider.NewAnthropicProvider(apiKey, anthropicModel))
		log.Printf("Anthropic provider registered (model: %s)", anthropicModel)
	} else {
		log.Printf("No Anthropic API key found (checked %s), Anthropic provider not available", anthropicKeyEnv)
	}

	// Configure fallback chain and primary provider
	fallbackChain := []string{}
	if cfg.Fallback1 != "" {
		fallbackChain = append(fallbackChain, cfg.Fallback1)
	}
	if cfg.Fallback2 != "" {
		fallbackChain = append(fallbackChain, cfg.Fallback2)
	}
	registry.SetFallbackChain(fallbackChain)
	registry.SetPrimary(cfg.Name)

	// Verify primary provider exists
	if _, err := registry.Get(cfg.Name); err != nil {
		log.Fatalf("Primary provider %q not registered: %v", cfg.Name, err)
	}

	return registry
}

// setupTools creates the tool registry and registers tools per the config's enabled list.
func setupTools(cfg *agent.ToolsSection, mnemo *memoclient.Client, agentID string) *tools.ToolRegistry {
	toolReg := tools.NewToolRegistry()
	enabled := cfg.Enabled

	// If no tools.enabled specified, register all tools (backward compatible)
	if len(enabled) == 0 {
		tools.RegisterMemoryTools(toolReg, mnemo, agentID)
		tools.RegisterFileToolsWithConfig(toolReg, cfg.AllowedDirs, cfg.ApprovalRequired)
		tools.RegisterTerminalTools(toolReg)
		return toolReg
	}

	// Register only enabled tool groups
	if anyEnabled(enabled, "AddMemory", "SearchMemories", "ReplaceMemory", "RemoveMemory", "Consolidate", "GetHotBlock") {
		tools.RegisterMemoryTools(toolReg, mnemo, agentID)
	}
	if anyEnabled(enabled, "FileRead", "FileSearch", "FileWrite") {
		tools.RegisterFileToolsWithConfig(toolReg, cfg.AllowedDirs, cfg.ApprovalRequired)
	}
	if anyEnabled(enabled, "Execute") {
		tools.RegisterTerminalTools(toolReg)
	}

	return toolReg
}

// anyEnabled returns true if any of the given tool names appear in the enabled list.
func anyEnabled(enabled []string, names ...string) bool {
	for _, n := range names {
		for _, e := range enabled {
			if e == n {
				return true
			}
		}
	}
	return false
}

// firstNonEmpty returns the first non-empty string from the arguments.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}