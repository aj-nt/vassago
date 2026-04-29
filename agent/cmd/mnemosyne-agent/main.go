package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

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

	// Set up provider registry
	registry := provider.NewProviderRegistry()

	// Determine primary provider from config
	primaryProvider := cfg.Provider.Name

	// Set up OpenAI-compatible provider (local Ollama or remote OpenAI)
	openaiBaseURL := cfg.Provider.OpenAIBaseURL
	if openaiBaseURL == "" {
		openaiBaseURL = cfg.Provider.BaseURL // fallback to old field
	}
	openaiModel := cfg.Provider.OpenAIModel
	if openaiModel == "" {
		openaiModel = cfg.Provider.Model // fallback to shared model field
	}
	openaiKeyEnv := cfg.Provider.OpenAIAPIKeyEnv
	if openaiKeyEnv == "" {
		openaiKeyEnv = cfg.Provider.APIKeyEnv // fallback to shared key env
	}
	openaiAPIKey := os.Getenv(openaiKeyEnv)

	openaiProvider := provider.NewOpenAIProvider(openaiBaseURL, openaiAPIKey, openaiModel)
	registry.Register("openai", openaiProvider)

	// Set up Anthropic provider if API key is available
	anthropicKeyEnv := cfg.Provider.AnthropicAPIKeyEnv
	if anthropicKeyEnv == "" {
		anthropicKeyEnv = "ANTHROPIC_API_KEY"
	}
	if apiKey := os.Getenv(anthropicKeyEnv); apiKey != "" {
		anthropicModel := cfg.Provider.AnthropicModel
		if anthropicModel == "" {
			anthropicModel = "claude-sonnet-4-6"
		}
		anthropicProvider := provider.NewAnthropicProvider(apiKey, anthropicModel)
		registry.Register("anthropic", anthropicProvider)
		log.Printf("Anthropic provider registered (model: %s)", anthropicModel)
	} else {
		log.Printf("No Anthropic API key found (checked %s), Anthropic provider not available", anthropicKeyEnv)
	}

	// Set fallback chain from config
	fallbackChain := []string{}
	if cfg.Provider.Fallback1 != "" {
		fallbackChain = append(fallbackChain, cfg.Provider.Fallback1)
	}
	if cfg.Provider.Fallback2 != "" {
		fallbackChain = append(fallbackChain, cfg.Provider.Fallback2)
	}
	registry.SetFallbackChain(fallbackChain)
	registry.SetPrimary(primaryProvider)

	// Verify primary provider exists
	if _, err := registry.Get(primaryProvider); err != nil {
		log.Fatalf("Primary provider %q not registered: %v", primaryProvider, err)
	}

	// Set up tool registry — honor tools.enabled from config
	toolReg := tools.NewToolRegistry()
	enabledTools := cfg.Tools.Enabled
	
	memoryTools := map[string]bool{
		"AddMemory": true, "SearchMemories": true, "ReplaceMemory": true,
		"RemoveMemory": true, "Consolidate": true, "GetHotBlock": true,
	}
	fileTools := map[string]bool{
		"FileRead": true, "FileSearch": true, "FileWrite": true,
	}
	terminalTools := map[string]bool{
		"Execute": true,
	}

	// If tools.enabled is empty, register all tools (backward compatible)
	registerAll := len(enabledTools) == 0

	shouldRegister := func(name string) bool {
		if registerAll {
			return true
		}
		for _, e := range enabledTools {
			if e == name {
				return true
			}
		}
		return false
	}

	registeredAny := false
	for name := range memoryTools {
		if shouldRegister(name) {
			registeredAny = true
		}
	}
	if registeredAny {
		tools.RegisterMemoryTools(toolReg, mnemo, cfg.Agent.ID)
	}
	
	registeredAny = false
	for name := range fileTools {
		if shouldRegister(name) {
			registeredAny = true
		}
	}
	if registeredAny {
		tools.RegisterFileTools(toolReg)
	}

	registeredAny = false
	for name := range terminalTools {
		if shouldRegister(name) {
			registeredAny = true
		}
	}
	if registeredAny {
		tools.RegisterTerminalTools(toolReg)
	}

	// Create agent
	agentCfg := cfg.ToAgentConfig()
	a := agent.NewAgent(agentCfg, registry, toolReg, mnemo)
	defer a.Close()

	// Set primary provider for the agent
	log.Printf("Starting agent %q with primary provider %q (model: %s)", cfg.Agent.ID, primaryProvider, cfg.Provider.Model)

	// Run
	cli := iface.NewCLI(a)
	if *oneShot != "" {
		if err := cli.RunOneShot(ctx, *oneShot); err != nil {
			log.Fatalf("Error: %v", err)
		}
	} else {
		// Check if stdin has data (piped input)
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Piped input
			var input string
			if _, err := fmt.Scan(&input); err != nil {
				log.Fatalf("Error reading input: %v", err)
			}
			if err := cli.RunOneShot(ctx, input); err != nil {
				log.Fatalf("Error: %v", err)
			}
		} else {
			// Interactive
			if err := cli.RunInteractive(ctx); err != nil {
				log.Fatalf("Error: %v", err)
			}
		}
	}
}