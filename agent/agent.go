package agent

import (
	"context"
	"fmt"
	"log"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/pipeline"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
	memoclient "github.com/aj-nt/mnemosyne/client"
)

// Agent orchestrates the 5-stage pipeline loop.
type Agent struct {
	Config      Config
	Registry   *provider.ProviderRegistry
	ToolRegistry *tools.ToolRegistry
	EventBus    *eventbus.EventBus
	Mnemo       *memoclient.Client

	// Pipeline stages — overridable for testing
	Stage1 *pipeline.RequestPrepStage
	Stage2 *pipeline.ProviderCallStage
	Stage3 *pipeline.ResponseProcessingStage
	Stage4 *pipeline.ToolDispatchStage
	Stage5 *pipeline.ContextManagementStage
}

// Config holds agent configuration.
type Config struct {
	AgentID          string
	AgentName        string
	AgentRole        string
	Personality     string
	Temperature     float64
	MaxTurns         int
	MaxContextTokens int
	HotBlockConfig   HotBlockConfig
}

// HotBlockConfig controls hot block retrieval.
type HotBlockConfig struct {
	CharLimit          int
	MinPriority        int
	BoostCategories    []string
	SuppressCategories []string
	BoostPriority      int32
}

// NewAgent creates a new agent with the given configuration.
func NewAgent(cfg Config, registry *provider.ProviderRegistry, toolReg *tools.ToolRegistry, mnemo *memoclient.Client) *Agent {
	bus := eventbus.NewEventBus()

	a := &Agent{
		Config:       cfg,
		Registry:     registry,
		ToolRegistry: toolReg,
		EventBus:     bus,
		Mnemo:        mnemo,
	}

	// Wire pipeline stages
	a.Stage1 = pipeline.NewRequestPrepStage(cfg.Personality)
	a.Stage2 = pipeline.NewProviderCallStage(registry)
	a.Stage3 = pipeline.NewResponseProcessingStage(registry)
	a.Stage4 = pipeline.NewToolDispatchStage(toolReg)
	a.Stage5 = pipeline.NewContextManagementStage(cfg.MaxContextTokens)

	return a
}

// Close cleans up agent resources.
func (a *Agent) Close() {
	a.EventBus.Close()
}

// Run executes the agent loop for a user message.
// This is the simple API — creates a fresh context each time.
func (a *Agent) Run(ctx context.Context, userMessage string) (*agentctx.PipelineResult, error) {
	// Get hot block from daemon
	hotBlock, err := a.GetHotBlock(ctx)
	if err != nil {
		log.Printf("Warning: failed to get hot block: %v", err)
		hotBlock = ""
	}

	// Create session
	var sessionID string
	if a.Mnemo != nil {
		sessionID, _ = a.Mnemo.CreateSession(ctx, a.Config.AgentID, "cli", "")
	}

	// Build conversation context
	interrupt := agentctx.NewInterruptSignal()
	convCtx := &agentctx.ConversationContext{
		SessionID:       sessionID,
		AgentID:         a.Config.AgentID,
		Messages:        []agentctx.Message{{Role: "user", Content: userMessage}},
		SystemPrompt:    "",
		Tools:           a.BuildToolDefinitions(),
		MaxIterations:   a.Config.MaxTurns,
		Iteration:       0,
		Interrupt:       interrupt,
		HotBlockContent: hotBlock,
		Temperature:     a.Config.Temperature,
	}

	return a.RunWithContext(ctx, convCtx)
}

// RunWithContext executes the agent loop using an existing conversation context.
// This is the main loop — it runs the 5-stage pipeline until YIELD or TERMINATE.
func (a *Agent) RunWithContext(ctx context.Context, convCtx *agentctx.ConversationContext) (*agentctx.PipelineResult, error) {
	var sessionID string
	if a.Mnemo != nil && convCtx.SessionID != "" {
		sessionID = convCtx.SessionID
	}

	for {
		// Check termination conditions
		if convCtx.Interrupt.IsSet() {
			return &agentctx.PipelineResult{Interrupted: true, SessionID: sessionID}, nil
		}
		if convCtx.Iteration >= convCtx.MaxIterations {
			return &agentctx.PipelineResult{MaxIterationsHit: true, Iterations: convCtx.Iteration, SessionID: sessionID}, nil
		}
		// Check context token limit
		if a.Config.MaxContextTokens > 0 && convCtx.TokenCount() > a.Config.MaxContextTokens {
			return &agentctx.PipelineResult{MaxIterationsHit: true, Iterations: convCtx.Iteration, SessionID: sessionID}, nil
		}

		// Emit iteration event
		a.EventBus.Emit(eventbus.Event{Kind: "iteration_start", Data: convCtx.Iteration})

		// Stage 1: Request Preparation
		prepared := a.Stage1.Process(convCtx)

		// Stage 2: Provider Call
		result := a.Stage2.Process(ctx, convCtx, prepared)
		if result.Error != nil {
			return nil, fmt.Errorf("provider call failed: %w", result.Error)
		}

		// Stage 3: Response Processing
		parsed, err := a.Stage3.Process(ctx, result)
		if err != nil {
			return nil, fmt.Errorf("response processing failed: %w", err)
		}

		// Emit response event
		a.EventBus.Emit(eventbus.Event{Kind: "response", Data: parsed})

		// Stage 4: Tool Dispatch
		dispatch := a.Stage4.Process(convCtx, parsed)

		// Stage 5: Context Management
		action := a.Stage5.Process(convCtx, dispatch.ToolResults, parsed)

		// Emit iteration end event
		a.EventBus.Emit(eventbus.Event{Kind: "iteration_end", Data: action})

		convCtx.Iteration++

		switch action {
		case agentctx.StageActionYIELD:
			a.persistSession(ctx, sessionID, convCtx)
			return &agentctx.PipelineResult{
				Response:   &parsed,
				Iterations: convCtx.Iteration,
				SessionID:  sessionID,
			}, nil
		case agentctx.StageActionTERMINATE:
			a.persistSession(ctx, sessionID, convCtx)
			return &agentctx.PipelineResult{
				Response:         &parsed,
				Iterations:       convCtx.Iteration,
				MaxIterationsHit: convCtx.Iteration >= convCtx.MaxIterations,
				SessionID:        sessionID,
			}, nil
		case agentctx.StageActionCONTINUE:
			continue
		}
	}
}

// GetHotBlock retrieves the hot block from the daemon.
func (a *Agent) GetHotBlock(ctx context.Context) (string, error) {
	if a.Mnemo == nil {
		return "", nil
	}
	return a.Mnemo.GetHotBlock(
		ctx,
		"memory",
		a.Config.AgentID,
		int32(a.Config.HotBlockConfig.CharLimit),
		a.Config.HotBlockConfig.BoostCategories,
		a.Config.HotBlockConfig.SuppressCategories,
		a.Config.HotBlockConfig.BoostPriority,
	)
}

// BuildToolDefinitions creates ToolDefinitions from the registry schemas.
func (a *Agent) BuildToolDefinitions() []agentctx.ToolDefinition {
	schemas := a.ToolRegistry.Schemas()
	defs := make([]agentctx.ToolDefinition, 0, len(schemas))
	for name, schema := range schemas {
		desc := ""
		if d, ok := schema["description"].(string); ok {
			desc = d
		}
		params := map[string]interface{}{}
		if p, ok := schema["parameters"].(map[string]interface{}); ok {
			params = p
		}
		defs = append(defs, agentctx.ToolDefinition{
			Name:        name,
			Description: desc,
			Parameters:  params,
		})
	}
	return defs
}

// persistSession saves the conversation to the daemon.
func (a *Agent) persistSession(ctx context.Context, sessionID string, convCtx *agentctx.ConversationContext) {
	if a.Mnemo == nil || sessionID == "" {
		return
	}
	msgs := make([]memoclient.Message, len(convCtx.Messages))
	for i, m := range convCtx.Messages {
		msgs[i] = memoclient.Message{Role: m.Role, Content: m.Content}
	}
	if err := a.Mnemo.AddMessages(ctx, sessionID, msgs); err != nil {
		log.Printf("Warning: failed to persist session: %v", err)
	}
}