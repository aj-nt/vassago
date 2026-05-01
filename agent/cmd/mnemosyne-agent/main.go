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
	"github.com/aj-nt/mnemosyne/agent/scheduler"
	"github.com/aj-nt/mnemosyne/agent/tools"
	iface "github.com/aj-nt/mnemosyne/agent/interface"
	memoclient "github.com/aj-nt/mnemosyne/client"
	pb "github.com/aj-nt/mnemosyne/proto"
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

	// Register agent with daemon for identity tracking and heartbeat
	agentSession := memoclient.NewAgentSession(mnemo, memoclient.AgentConfig{
		ID:   cfg.Agent.ID,
		Name: cfg.Agent.Name,
		Role: cfg.Agent.Role,
	})
	if err := agentSession.Register(ctx); err != nil {
		log.Printf("Warning: agent registration failed (daemon may not support it): %v", err)
	} else {
		defer agentSession.Close()
		// Use the registered agent ID (may be auto-generated if config was empty)
		cfg.Agent.ID = agentSession.AgentID()
		log.Printf("Agent registered with daemon: id=%s", cfg.Agent.ID)
	}

	// Wire providers and tools from config
	registry := setupProviders(&cfg.Provider)
	subRunner := &agent.SubAgentRunnerImpl{
		Mnemo:    mnemo,
		Registry: registry,
		Router:   nil, // will be set below if routing is configured
	}
	toolReg := setupTools(&cfg.Tools, &cfg.Delegation, mnemo, cfg.Agent.ID, subRunner)

	// Create agent — with model router if routing config present
	agentCfg := cfg.ToAgentConfig()
	var a *agent.Agent
	if len(cfg.Routing.Models) > 0 {
		router := setupRouter(&cfg.Routing, &cfg.Provider, registry)
		a = agent.NewAgentWithRouter(agentCfg, registry, router, toolReg, mnemo)
		log.Printf("Model routing enabled: strategy=%s, primary=%s, models=%v",
			cfg.Routing.Strategy, router.Primary(), router.List())
	} else {
		a = agent.NewAgent(agentCfg, registry, toolReg, mnemo)
	}
	defer a.Close()
	subRunner.Router = a.Router // set router for sub-agent delegation

	// Set up hot block cache and Telepathy subscription
	if agentSession.IsRegistered() {
		// Fetch initial hot block to seed the cache
		if hotBlock, err := a.GetHotBlock(ctx); err == nil && hotBlock != "" {
			cache := memoclient.NewHotBlockCache("memory", cfg.Agent.ID, hotBlock, len([]rune(hotBlock)), 0)
			a.SetHotBlockCache(cache)
		}
		// Subscribe to memory changes and invalidate hot block on events
		agentSession.OnEvent = func(event *pb.UpdateEvent) {
			a.InvalidateHotBlock()
			log.Printf("Telepathy: %s event from %s (key=%s)",
				event.EventType.String(), event.AgentId, event.Key)
		}
		subCtx := context.Background()
		if eventCh, err := agentSession.Subscribe(subCtx, []string{"memory"}, nil); err != nil {
			log.Printf("Warning: Telepathy subscription failed: %v", err)
		} else {
			// Drain events in background (they're handled by OnEvent callback)
			go func() {
				for range eventCh {
				}
			}()
		}
	}

	// Start scheduler for periodic tasks (consolidation, cleanup)
	if cfg.Scheduler.Enabled && mnemo != nil {
		sched := scheduler.NewScheduler(mnemo)
		for _, task := range sched.MnemoTasks() {
			sched.AddTask(task)
		}
		sched.Start(ctx)
		defer sched.Stop()
		log.Printf("Scheduler started: %d tasks (interval: %ds)", len(sched.MnemoTasks()), cfg.Scheduler.Interval)
	}

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
func setupTools(cfg *agent.ToolsSection, delegateCfg *agent.DelegationSection, mnemo *memoclient.Client, agentID string, subRunner tools.SubAgentRunner) *tools.ToolRegistry {
	toolReg := tools.NewToolRegistry()
	enabled := cfg.Enabled

	// If no tools.enabled specified, register all tools (backward compatible)
	if len(enabled) == 0 {
		tools.RegisterMemoryTools(toolReg, mnemo, agentID)
		tools.RegisterFileToolsWithConfig(toolReg, cfg.AllowedDirs, cfg.ApprovalRequired)
		tools.RegisterTerminalTools(toolReg)
		if delegateCfg.Enabled {
			tools.RegisterDelegationTool(toolReg, agentID, subRunner, tools.DelegateConfig{
				Enabled:  delegateCfg.Enabled,
				MaxDepth: delegateCfg.MaxDepth,
				MaxTurns: delegateCfg.MaxTurns,
			})
		}
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
	if anyEnabled(enabled, "SearchWeb", "FetchURL") {
		tools.RegisterWebTools(toolReg, nil) // nil = use default HTTP client
	}
	if anyEnabled(enabled, "SendMessage") {
		tools.RegisterMessagingTools(toolReg, mnemo, agentID)
	}
	if anyEnabled(enabled, "Delegate") && delegateCfg.Enabled {
		tools.RegisterDelegationTool(toolReg, agentID, subRunner, tools.DelegateConfig{
			Enabled:  delegateCfg.Enabled,
			MaxDepth: delegateCfg.MaxDepth,
			MaxTurns: delegateCfg.MaxTurns,
		})
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

// setupRouter creates and configures the model router from config.
func setupRouter(cfg *agent.RoutingSection, providerCfg *agent.ProviderSection, registry *provider.ProviderRegistry) *provider.ModelRouter {
	strategy := provider.RoutingStrategy(cfg.Strategy)
	router := provider.NewModelRouter(strategy)

	for i := range cfg.Models {
		m := cfg.Models[i]
		// Populate BaseURL from provider for health-checking.
		// Local endpoints (e.g., Ollama at localhost:11434) get checked;
		// cloud APIs (no local URL) are assumed reachable.
		if p, err := registry.Get(m.Provider); err == nil {
			m.BaseURL = p.EndpointURL()
		}
		router.Register(m)
	}

	// Set primary to the first model, or to the provider's primary model name
	if len(cfg.Models) > 0 {
		if err := router.SetPrimary(cfg.Models[0].Name); err != nil {
			log.Printf("Warning: failed to set primary model to %q: %v", cfg.Models[0].Name, err)
		}
	}

	return router
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