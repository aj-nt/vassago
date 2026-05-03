// Package setup provides shared agent initialization logic used by both
// the standalone mnemosyne-agent binary and the unified mnemosyne CLI.
package setup

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
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
	"google.golang.org/grpc/reflection"
)

// AgentResult holds the initialized agent and its resources.
// The caller is responsible for calling Close() on cleanup.
type AgentResult struct {
	Agent         *agent.Agent
	Config        *agent.AgentConfig
	Mnemo         *memoclient.Client
	AgentSession  *memoclient.AgentSession
	GRPCServer    *grpc.Server
	Bridge        *iface.MessageBridge
	Scheduler     *scheduler.Scheduler
}

// InitAgent sets up the full agent stack: config, daemon connection,
// providers, tools, router, Telepathy, scheduler, gRPC chat server,
// and message bridge. Returns an AgentResult that must be cleaned up.
func InitAgent(ctx context.Context, configPath string) (*AgentResult, error) {
	// Load config
	cfg, err := agent.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Connect to Mnemosyne daemon
	mnemo, err := memoclient.Connect(ctx, memoclient.Config{
		Address: cfg.Mnemosyne.Address,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Mnemosyne daemon: %w", err)
	}

	// Register agent with daemon
	agentSession := memoclient.NewAgentSession(mnemo, memoclient.AgentConfig{
		ID:   cfg.Agent.ID,
		Name: cfg.Agent.Name,
		Role: cfg.Agent.Role,
	})
	if err := agentSession.Register(ctx); err != nil {
		log.Printf("Warning: agent registration failed (daemon may not support it): %v", err)
	} else {
		cfg.Agent.ID = agentSession.AgentID()
		log.Printf("Agent registered with daemon: id=%s", cfg.Agent.ID)
	}

	// Wire providers and tools
	registry := Providers(&cfg.Provider)
	subRunner := &agent.SubAgentRunnerImpl{
		Mnemo:    mnemo,
		Registry: registry,
		Router:   nil,
	}
	toolReg := Tools(&cfg.Tools, &cfg.Delegation, &cfg.Browser, &cfg.Vision, &cfg.ImageGen, &cfg.MultiAgent, mnemo, cfg.Agent.ID, cfg.Agent.Name, cfg.Agent.Role, subRunner, registry)

	// Create agent (with router if configured)
	agentCfg := cfg.ToAgentConfig()
	var a *agent.Agent
	if len(cfg.Routing.Models) > 0 {
		router := Router(&cfg.Routing, &cfg.Provider, registry)
		a = agent.NewAgentWithRouter(agentCfg, registry, router, toolReg, mnemo)
		log.Printf("Model routing enabled: strategy=%s, primary=%s, models=%v",
			cfg.Routing.Strategy, router.Primary(), router.List())
	} else {
		a = agent.NewAgent(agentCfg, registry, toolReg, mnemo)
	}
	subRunner.Router = a.Router

	// Set up hot block cache and Telepathy subscription
	if agentSession.IsRegistered() {
		if hotBlock, err := a.GetHotBlock(ctx); err == nil && hotBlock != "" {
			cache := memoclient.NewHotBlockCache("memory", cfg.Agent.ID, hotBlock, len([]rune(hotBlock)), 0)
			a.SetHotBlockCache(cache)
		}
		agentSession.OnEvent = func(event *pb.UpdateEvent) {
			switch event.EventType {
			case pb.UpdateEvent_CRON_FIRE:
				if event.CronFire != nil && event.CronFire.AgentId == cfg.Agent.ID {
					log.Printf("Telepathy: CRON_FIRE event from %s (job=%s, name=%s)",
						event.CronFire.AgentId, event.CronFire.JobId, event.CronFire.JobName)
					go HandleCronFire(a, cfg.Agent.ID, cfg.Agent.MaxTurns, cfg.Provider.Temperature, event.CronFire)
				}
			default:
				a.InvalidateHotBlock()
				log.Printf("Telepathy: %s event from %s (key=%s)",
					event.EventType.String(), event.AgentId, event.Key)
			}
		}
		subCtx := context.Background()
		if eventCh, err := agentSession.Subscribe(subCtx, []string{"memory"}, nil); err != nil {
			log.Printf("Warning: Telepathy subscription failed: %v", err)
		} else {
			go func() {
				for range eventCh {
				}
			}()
		}
	}

	// Start scheduler
	var sched *scheduler.Scheduler
	if cfg.Scheduler.Enabled && mnemo != nil {
		sched = scheduler.NewScheduler(mnemo)
		for _, task := range sched.MnemoTasks() {
			sched.AddTask(task)
		}
		sched.Start(ctx)
		log.Printf("Scheduler started: %d tasks (interval: %ds)", len(sched.MnemoTasks()), cfg.Scheduler.Interval)
	}

	// Start gRPC chat server if configured
	var grpcServer *grpc.Server
	if cfg.Mnemosyne.ChatListen != "" {
		lis, err := net.Listen("tcp", cfg.Mnemosyne.ChatListen)
		if err != nil {
			return nil, fmt.Errorf("failed to listen for gRPC chat server on %s: %w", cfg.Mnemosyne.ChatListen, err)
		}
		grpcServer = grpc.NewServer()
		chatServer := iface.NewChatServer(a, mnemo)
		pb.RegisterAgentChatServer(grpcServer, chatServer)
		reflection.Register(grpcServer)
		go func() {
			log.Printf("gRPC ChatStream server listening on %s", cfg.Mnemosyne.ChatListen)
			if err := grpcServer.Serve(lis); err != nil {
				log.Fatalf("gRPC chat server failed: %v", err)
			}
		}()
	}

	// Start MessageBridge
	var bridge *iface.MessageBridge
	if cfg.Messaging.Discord.Enabled {
		bridgeConfig := iface.DefaultBridgeConfig()
		bridgeConfig.AgentID = cfg.Agent.ID
		bridge = iface.NewMessageBridge(a, mnemo, bridgeConfig)
		go func() {
			log.Printf("MessageBridge: starting incoming message handler...")
			if err := bridge.Start(ctx); err != nil {
				log.Printf("MessageBridge: stopped with error: %v", err)
			}
		}()
	}

	log.Printf("Starting agent %q with primary provider %q (model: %s)",
		cfg.Agent.ID, cfg.Provider.Name, cfg.Provider.Model)

	return &AgentResult{
		Agent:        a,
		Config:       cfg,
		Mnemo:        mnemo,
		AgentSession: agentSession,
		GRPCServer:   grpcServer,
		Bridge:       bridge,
		Scheduler:    sched,
	}, nil
}

// Close cleans up all agent resources.
func (r *AgentResult) Close() {
	if r.Bridge != nil {
		r.Bridge.Stop()
	}
	if r.GRPCServer != nil {
		r.GRPCServer.GracefulStop()
	}
	if r.Scheduler != nil {
		r.Scheduler.Stop()
	}
	tools.CleanupBrowser()
	r.Agent.Close()
	if r.AgentSession.IsRegistered() {
		r.AgentSession.Close()
	}
	if r.Mnemo != nil {
		r.Mnemo.Close()
	}
}

// WaitForSignal blocks until SIGINT or SIGTERM is received.
func WaitForSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Printf("Agent shutting down")
}

// Providers creates and configures the provider registry from config.
func Providers(cfg *agent.ProviderSection) *provider.ProviderRegistry {
	registry := provider.NewProviderRegistry()

	openaiBaseURL := FirstNonEmpty(cfg.OpenAIBaseURL, cfg.BaseURL, "https://api.openai.com/v1")
	openaiModel := FirstNonEmpty(cfg.OpenAIModel, cfg.Model)
	openaiKeyEnv := FirstNonEmpty(cfg.OpenAIAPIKeyEnv, cfg.APIKeyEnv, "OPENAI_API_KEY")
	openaiAPIKey := os.Getenv(openaiKeyEnv)

	registry.Register("openai", provider.NewOpenAIProvider(openaiBaseURL, openaiAPIKey, openaiModel))

	anthropicKeyEnv := FirstNonEmpty(cfg.AnthropicAPIKeyEnv, "ANTHROPIC_API_KEY")
	if apiKey := os.Getenv(anthropicKeyEnv); apiKey != "" {
		anthropicModel := FirstNonEmpty(cfg.AnthropicModel, "claude-sonnet-4-6")
		registry.Register("anthropic", provider.NewAnthropicProvider(apiKey, anthropicModel))
		log.Printf("Anthropic provider registered (model: %s)", anthropicModel)
	} else {
		log.Printf("No Anthropic API key found (checked %s), Anthropic provider not available", anthropicKeyEnv)
	}

	fallbackChain := []string{}
	if cfg.Fallback1 != "" {
		fallbackChain = append(fallbackChain, cfg.Fallback1)
	}
	if cfg.Fallback2 != "" {
		fallbackChain = append(fallbackChain, cfg.Fallback2)
	}
	registry.SetFallbackChain(fallbackChain)
	registry.SetPrimary(cfg.Name)

	if _, err := registry.Get(cfg.Name); err != nil {
		log.Fatalf("Primary provider %q not registered: %v", cfg.Name, err)
	}

	return registry
}

// Tools creates the tool registry and registers tools per the config's enabled list.
func Tools(cfg *agent.ToolsSection, delegateCfg *agent.DelegationSection, browserCfg *agent.BrowserSection, visionCfg *agent.VisionSection, imageGenCfg *agent.ImageGenSection, multiAgentCfg *agent.MultiAgentSection, mnemo *memoclient.Client, agentID string, agentName string, agentRole string, subRunner tools.SubAgentRunner, providerRegistry *provider.ProviderRegistry) *tools.ToolRegistry {
	toolReg := tools.NewToolRegistry()
	enabled := cfg.Enabled

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
				Provider:        imageGenCfg.Provider,
				Model:           imageGenCfg.Model,
				APIKeyEnv:       imageGenCfg.APIKeyEnv,
				MaxPromptLength: imageGenCfg.MaxPromptLength,
				CacheDir:        imageGenCfg.CacheDir,
			})
		}
		if multiAgentCfg.Enabled {
			tools.RegisterMultiAgentTools(toolReg, tools.MultiAgentConfig{
				Enabled:      multiAgentCfg.Enabled,
				RegistryPath: multiAgentCfg.RegistryPath,
			}, agentID, agentName, agentRole)
		}
		return toolReg
	}

	if AnyEnabled(enabled, "AddMemory", "SearchMemories", "ReplaceMemory", "RemoveMemory", "Consolidate", "GetHotBlock") {
		tools.RegisterMemoryTools(toolReg, mnemo, agentID)
	}
	if AnyEnabled(enabled, "FileRead", "FileSearch", "FileWrite") {
		tools.RegisterFileToolsWithConfig(toolReg, cfg.AllowedDirs, cfg.ApprovalRequired)
	}
	if AnyEnabled(enabled, "Execute") {
		tools.RegisterTerminalTools(toolReg)
	}
	if AnyEnabled(enabled, "SearchSessions", "ListSessions", "GetSession") {
		tools.RegisterSessionTools(toolReg, mnemo)
	}
	if AnyEnabled(enabled, "AddTodo", "ListTodos", "CompleteTodo", "RemoveTodo") {
		tools.RegisterTodoTools(toolReg, mnemo, agentID)
	}
	if AnyEnabled(enabled, "ScheduleCron") {
		tools.RegisterCronTools(toolReg, mnemo, agentID)
	}
	if AnyEnabled(enabled, "SearchWeb", "FetchURL") {
		tools.RegisterWebTools(toolReg, nil)
	}
	if AnyEnabled(enabled, "SendMessage") {
		tools.RegisterMessagingTools(toolReg, mnemo, agentID)
	}
	if AnyEnabled(enabled, "Browse") && browserCfg.Enabled {
		tools.RegisterBrowserTools(toolReg, tools.BrowserConfig{
			Enabled:  browserCfg.Enabled,
			Headless: browserCfg.Headless,
			CacheDir: browserCfg.CacheDir,
		})
	}
	if AnyEnabled(enabled, "AnalyzeImage") && visionCfg.Enabled {
		tools.RegisterVisionTools(toolReg, tools.VisionConfig{
			Enabled:      visionCfg.Enabled,
			MaxImageSize: visionCfg.MaxImageSize,
		}, providerRegistry)
	}
	if AnyEnabled(enabled, "GenerateImage") && imageGenCfg.Enabled {
		tools.RegisterImageGenTools(toolReg, tools.ImageGenConfig{
			Enabled:         imageGenCfg.Enabled,
			Provider:        imageGenCfg.Provider,
			Model:           imageGenCfg.Model,
			APIKeyEnv:       imageGenCfg.APIKeyEnv,
			MaxPromptLength: imageGenCfg.MaxPromptLength,
			CacheDir:        imageGenCfg.CacheDir,
		})
	}
	if AnyEnabled(enabled, "ListAgents", "RegisterAgent", "FindAgent") && multiAgentCfg.Enabled {
		tools.RegisterMultiAgentTools(toolReg, tools.MultiAgentConfig{
			Enabled:      multiAgentCfg.Enabled,
			RegistryPath: multiAgentCfg.RegistryPath,
		}, agentID, agentName, agentRole)
	}
	if AnyEnabled(enabled, "Delegate") && delegateCfg.Enabled {
		tools.RegisterDelegationTool(toolReg, agentID, subRunner, tools.DelegateConfig{
			Enabled:  delegateCfg.Enabled,
			MaxDepth: delegateCfg.MaxDepth,
			MaxTurns: delegateCfg.MaxTurns,
		})
	}

	return toolReg
}

// Router creates and configures the model router from config.
func Router(cfg *agent.RoutingSection, providerCfg *agent.ProviderSection, registry *provider.ProviderRegistry) *provider.ModelRouter {
	strategy := provider.RoutingStrategy(cfg.Strategy)
	router := provider.NewModelRouter(strategy)

	for i := range cfg.Models {
		m := cfg.Models[i]
		if p, err := registry.Get(m.Provider); err == nil {
			m.BaseURL = p.EndpointURL()
		}
		router.Register(m)
	}

	if len(cfg.Models) > 0 {
		if err := router.SetPrimary(cfg.Models[0].Name); err != nil {
			log.Printf("Warning: failed to set primary model to %q: %v", cfg.Models[0].Name, err)
		}
	}

	return router
}

// HandleCronFire processes a cron fire event by invoking the agent.
func HandleCronFire(a *agent.Agent, agentID string, maxTurns int, temperature float64, event *pb.CronFireEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	prompt := fmt.Sprintf("[Scheduled Task: %s (ID: %s, Schedule: %s)]\n\n%s",
		event.JobName, event.JobId, event.Schedule, event.Message)

	log.Printf("CronFire: invoking agent for job %q (%s)", event.JobName, event.JobId)

	hotBlock, _ := a.GetHotBlock(ctx)
	toolDefs := a.BuildToolDefinitions()
	convCtx := &agentctx.ConversationContext{
		AgentID: agentID,
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

// AnyEnabled returns true if any of the given tool names appear in the enabled list.
func AnyEnabled(enabled []string, names ...string) bool {
	for _, n := range names {
		for _, e := range enabled {
			if e == n {
				return true
			}
		}
	}
	return false
}

// FirstNonEmpty returns the first non-empty string from the arguments.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}