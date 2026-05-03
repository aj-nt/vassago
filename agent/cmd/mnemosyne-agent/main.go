package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/scheduler"
	"github.com/aj-nt/mnemosyne/agent/tools"
	iface "github.com/aj-nt/mnemosyne/agent/interface"
	memoclient "github.com/aj-nt/mnemosyne/client"
	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc"
)

func main() {
	configPath := flag.String("config", "configs/agent-dev.yaml", "Path to agent config file")
	oneShot := flag.String("message", "", "Single message (non-interactive)")
	serve := flag.Bool("serve", false, "Server mode: start gRPC chat server and wait (no interactive CLI)")
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
	toolReg := setupTools(&cfg.Tools, &cfg.Delegation, &cfg.Browser, &cfg.Vision, &cfg.ImageGen, mnemo, cfg.Agent.ID, subRunner, registry)

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
	defer tools.CleanupBrowser() // Clean up browser driver on exit
	subRunner.Router = a.Router // set router for sub-agent delegation

	// Set up hot block cache and Telepathy subscription
	if agentSession.IsRegistered() {
		// Fetch initial hot block to seed the cache
		if hotBlock, err := a.GetHotBlock(ctx); err == nil && hotBlock != "" {
			cache := memoclient.NewHotBlockCache("memory", cfg.Agent.ID, hotBlock, len([]rune(hotBlock)), 0)
			a.SetHotBlockCache(cache)
		}
			// Subscribe to memory changes and handle cron fire events
		agentSession.OnEvent = func(event *pb.UpdateEvent) {
			switch event.EventType {
			case pb.UpdateEvent_CRON_FIRE:
				// Handle cron fire: invoke the agent with the cron job's message
				if event.CronFire != nil {
					log.Printf("Telepathy: CRON_FIRE event from %s (job=%s, name=%s)",
						event.CronFire.AgentId, event.CronFire.JobId, event.CronFire.JobName)
					// Only handle if this job belongs to our agent
					if event.CronFire.AgentId == cfg.Agent.ID {
						go handleCronFire(a, cfg.Agent.ID, cfg.Agent.MaxTurns, cfg.Provider.Temperature, event.CronFire)
					}
				}
			default:
				// Memory/other events — invalidate hot block
				a.InvalidateHotBlock()
				log.Printf("Telepathy: %s event from %s (key=%s)",
					event.EventType.String(), event.AgentId, event.Key)
			}
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

	// Start gRPC chat server if configured
	// This allows interactive conversations over gRPC (e.g., from Hermes)
	// while the CLI runs normally for local interactive use.
	var grpcServer *grpc.Server
	if cfg.Mnemosyne.ChatListen != "" {
		lis, err := net.Listen("tcp", cfg.Mnemosyne.ChatListen)
		if err != nil {
			log.Fatalf("Failed to listen for gRPC chat server on %s: %v", cfg.Mnemosyne.ChatListen, err)
		}
		grpcServer = grpc.NewServer()
		chatServer := iface.NewChatServer(a, mnemo)
		pb.RegisterAgentChatServer(grpcServer, chatServer)
		go func() {
			log.Printf("gRPC ChatStream server listening on %s", cfg.Mnemosyne.ChatListen)
			if err := grpcServer.Serve(lis); err != nil {
				log.Fatalf("gRPC chat server failed: %v", err)
			}
		}()
		defer grpcServer.GracefulStop()
	}

	// Start MessageBridge for incoming messages (Discord, Telegram, etc.)
	// This subscribes to the daemon's messaging stream and routes incoming
	// messages through the agent, sending responses back to the platform.
	var bridge *iface.MessageBridge
	if cfg.Messaging.Discord.Enabled && *serve {
		bridgeConfig := iface.DefaultBridgeConfig()
		bridgeConfig.AgentID = cfg.Agent.ID
		bridge = iface.NewMessageBridge(a, mnemo, bridgeConfig)
		go func() {
			log.Printf("MessageBridge: starting incoming message handler...")
			if err := bridge.Start(ctx); err != nil {
				log.Printf("MessageBridge: stopped with error: %v", err)
			}
		}()
		defer bridge.Stop()
	}

	// Run
	if *serve {
		// Server mode: just wait for gRPC connections, no interactive CLI
		if cfg.Mnemosyne.ChatListen == "" {
			log.Fatal("--serve requires chat_listen to be configured")
		}
		log.Printf("Agent running in server mode (gRPC chat on %s)", cfg.Mnemosyne.ChatListen)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("Agent shutting down")
	}

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
func setupTools(cfg *agent.ToolsSection, delegateCfg *agent.DelegationSection, browserCfg *agent.BrowserSection, visionCfg *agent.VisionSection, imageGenCfg *agent.ImageGenSection, mnemo *memoclient.Client, agentID string, subRunner tools.SubAgentRunner, providerRegistry *provider.ProviderRegistry) *tools.ToolRegistry {
	toolReg := tools.NewToolRegistry()
	enabled := cfg.Enabled

	// If no tools.enabled specified, register all tools (backward compatible)
	if len(enabled) == 0 {
		tools.RegisterMemoryTools(toolReg, mnemo, agentID)
		tools.RegisterFileToolsWithConfig(toolReg, cfg.AllowedDirs, cfg.ApprovalRequired)
		tools.RegisterTerminalTools(toolReg)
		tools.RegisterSessionTools(toolReg, mnemo)
		tools.RegisterTodoTools(toolReg, mnemo, agentID)
		tools.RegisterCronTools(toolReg, mnemo, agentID)
		if delegateCfg.Enabled {
			tools.RegisterDelegationTool(toolReg, agentID, subRunner, tools.DelegateConfig{
				Enabled:  delegateCfg.Enabled,
				MaxDepth: delegateCfg.MaxDepth,
				MaxTurns: delegateCfg.MaxTurns,
			})
		}
		if browserCfg.Enabled {
			tools.RegisterBrowserTools(toolReg, tools.BrowserConfig{
				Enabled:  browserCfg.Enabled,
				Headless: browserCfg.Headless,
				CacheDir: browserCfg.CacheDir,
			})
		}
		if visionCfg.Enabled {
			tools.RegisterVisionTools(toolReg, tools.VisionConfig{
				Enabled:      visionCfg.Enabled,
				MaxImageSize: visionCfg.MaxImageSize,
			}, providerRegistry)
		}
		if imageGenCfg.Enabled {
			tools.RegisterImageGenTools(toolReg, tools.ImageGenConfig{
				Enabled:         imageGenCfg.Enabled,
				Provider:       imageGenCfg.Provider,
				Model:          imageGenCfg.Model,
				APIKeyEnv:      imageGenCfg.APIKeyEnv,
				MaxPromptLength: imageGenCfg.MaxPromptLength,
				CacheDir:       imageGenCfg.CacheDir,
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
	if anyEnabled(enabled, "SearchSessions", "ListSessions", "GetSession") {
		tools.RegisterSessionTools(toolReg, mnemo)
	}
	if anyEnabled(enabled, "AddTodo", "ListTodos", "CompleteTodo", "RemoveTodo") {
		tools.RegisterTodoTools(toolReg, mnemo, agentID)
	}
	if anyEnabled(enabled, "ScheduleCron") {
		tools.RegisterCronTools(toolReg, mnemo, agentID)
	}
	if anyEnabled(enabled, "SearchWeb", "FetchURL") {
		tools.RegisterWebTools(toolReg, nil) // nil = use default HTTP client
	}
	if anyEnabled(enabled, "SendMessage") {
		tools.RegisterMessagingTools(toolReg, mnemo, agentID)
	}
	if anyEnabled(enabled, "Browse") && browserCfg.Enabled {
		tools.RegisterBrowserTools(toolReg, tools.BrowserConfig{
			Enabled:  browserCfg.Enabled,
			Headless: browserCfg.Headless,
			CacheDir: browserCfg.CacheDir,
		})
	}
	if anyEnabled(enabled, "AnalyzeImage") && visionCfg.Enabled {
		tools.RegisterVisionTools(toolReg, tools.VisionConfig{
			Enabled:      visionCfg.Enabled,
			MaxImageSize: visionCfg.MaxImageSize,
		}, providerRegistry)
	}
	if anyEnabled(enabled, "GenerateImage") && imageGenCfg.Enabled {
		tools.RegisterImageGenTools(toolReg, tools.ImageGenConfig{
			Enabled:         imageGenCfg.Enabled,
			Provider:       imageGenCfg.Provider,
			Model:          imageGenCfg.Model,
			APIKeyEnv:      imageGenCfg.APIKeyEnv,
			MaxPromptLength: imageGenCfg.MaxPromptLength,
			CacheDir:       imageGenCfg.CacheDir,
		})
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

// handleCronFire processes a cron fire event by invoking the agent with the
// cron job's message. This is the core of the proactive agent: the daemon
// fires a cron job, publishes the event, and the agent picks it up and acts on it.
func handleCronFire(a *agent.Agent, agentID string, maxTurns int, temperature float64, event *pb.CronFireEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Build the prompt prefix with cron context
	prompt := fmt.Sprintf("[Scheduled Task: %s (ID: %s, Schedule: %s)]\n\n%s",
		event.JobName, event.JobId, event.Schedule, event.Message)

	log.Printf("CronFire: invoking agent for job %q (%s)", event.JobName, event.JobId)

	// Build a conversation context for this cron fire
	hotBlock, _ := a.GetHotBlock(ctx)

	toolDefs := a.BuildToolDefinitions()
	convCtx := &agentctx.ConversationContext{
		AgentID:         agentID,
		Messages: []agentctx.Message{
			{Role: "user", Content: prompt},
		},
		Tools:           toolDefs,
		MaxIterations:   maxTurns,
		Iteration:       0,
		Interrupt:       agentctx.NewInterruptSignal(),
		HotBlockContent: hotBlock,
		Temperature:     temperature,
	}

	// Run the agent and collect the response
	out := make(chan agent.StreamEvent, 100)
	go a.RunStreaming(ctx, convCtx, out)

	var responseText string
	for evt := range out {
		if evt.Type == agentctx.StreamText {
			responseText += evt.Text
		} else if evt.Type == agentctx.StreamError {
			log.Printf("CronFire: stream error for job %q: %v", event.JobName, evt.Error)
			return
		} else if evt.Type == agentctx.StreamDone {
			break
		}
	}

	log.Printf("CronFire: agent completed job %q (response: %d chars)", event.JobName, len(responseText))
}