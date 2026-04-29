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
	apiKey := os.Getenv(cfg.Provider.APIKeyEnv)
	openaiProvider := provider.NewOpenAIProvider(cfg.Provider.BaseURL, apiKey, cfg.Provider.Model)
	registry.Register("openai", openaiProvider)

	// Set up tool registry
	toolReg := tools.NewToolRegistry()
	tools.RegisterMemoryTools(toolReg, mnemo, cfg.Agent.ID)
	tools.RegisterFileTools(toolReg)
	tools.RegisterTerminalTools(toolReg)

	// Set up Anthropic provider if API key is available
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		anthropicProvider := provider.NewAnthropicProvider(apiKey, cfg.Provider.Model)
		registry.Register("anthropic", anthropicProvider)
	}

	// Create agent
	agentCfg := cfg.ToAgentConfig()
	a := agent.NewAgent(agentCfg, registry, toolReg, mnemo)
	defer a.Close()

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
