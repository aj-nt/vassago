package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync/atomic"

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
	Router     *provider.ModelRouter // Optional: nil = always-primary (backward compatible)
	ToolRegistry *tools.ToolRegistry
	EventBus    *eventbus.EventBus
	Mnemo       *memoclient.Client

	// Hot block cache — updated by Telepathy events, used by GetHotBlock()
	hotBlockCache *memoclient.HotBlockCache
	hotBlockStale int32 // atomic: 1 = stale, needs refetch

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
	a.Stage4 = pipeline.NewToolDispatchStage(toolReg, bus)
	a.Stage5 = pipeline.NewContextManagementStage(cfg.MaxContextTokens)

	return a
}

// NewAgentWithRouter creates a new agent with model routing.
func NewAgentWithRouter(cfg Config, registry *provider.ProviderRegistry, router *provider.ModelRouter, toolReg *tools.ToolRegistry, mnemo *memoclient.Client) *Agent {
	bus := eventbus.NewEventBus()

	a := &Agent{
		Config:       cfg,
		Registry:     registry,
		Router:       router,
		ToolRegistry: toolReg,
		EventBus:     bus,
		Mnemo:        mnemo,
	}

	// Wire pipeline stages
	a.Stage1 = pipeline.NewRequestPrepStage(cfg.Personality)
	a.Stage2 = pipeline.NewProviderCallStageWithRouter(registry, router)
	a.Stage3 = pipeline.NewResponseProcessingStage(registry)
	a.Stage4 = pipeline.NewToolDispatchStage(toolReg, bus)
	a.Stage5 = pipeline.NewContextManagementStage(cfg.MaxContextTokens)

	return a
}

// Close cleans up agent resources.
func (a *Agent) Close() {
	a.EventBus.Close()
}

// selectProvider picks the provider for this turn using the model router.
// Falls back to the registry's primary if no router is configured.
func (a *Agent) selectProvider(convCtx *agentctx.ConversationContext) (provider.Provider, error) {
	if a.Router == nil {
		return a.Registry.GetPrimary()
	}

	// Build routing context from conversation state
	routingCtx := provider.RoutingContext{
		Iteration:       convCtx.Iteration,
		UserMessage:     firstUserMessage(convCtx.Messages),
		HasToolResults:  lastMessageIsToolResult(convCtx.Messages),
		ToolCallCount:   countToolResults(convCtx.Messages),
		PromptCharCount: convCtx.TokenCount() * 4, // Rough: tokens→chars
	}

	modelCfg, err := a.Router.Select(routingCtx)
	if err != nil {
		// Fallback to primary provider on routing error
		return a.Registry.GetPrimary()
	}

	p, err := a.Registry.Get(modelCfg.Provider)
	if err != nil {
		// Model's provider not registered — try primary
		p, err2 := a.Registry.GetPrimary()
		if err2 != nil {
			return nil, fmt.Errorf("model router selected %q (provider %q not found): %w", modelCfg.Name, modelCfg.Provider, err)
		}
		return p, nil
	}
	return p, nil
}

// firstUserMessage returns the first user message content.
func firstUserMessage(msgs []agentctx.Message) string {
	for _, m := range msgs {
		if m.Role == "user" {
			return m.Content
		}
	}
	return ""
}

// lastMessageIsToolResult checks if the last message is a tool result.
func lastMessageIsToolResult(msgs []agentctx.Message) bool {
	if len(msgs) == 0 {
		return false
	}
	return msgs[len(msgs)-1].Role == "tool"
}

// countToolResults counts tool result messages.
func countToolResults(msgs []agentctx.Message) int {
	count := 0
	for _, m := range msgs {
		if m.Role == "tool" {
			count++
		}
	}
	return count
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

// SetHotBlockCache sets the hot block cache for Telepathy-driven updates.
// Must be called before the agent starts processing (before Run).
func (a *Agent) SetHotBlockCache(cache *memoclient.HotBlockCache) {
	a.hotBlockCache = cache
}

// InvalidateHotBlock marks the cached hot block as stale so the next
// GetHotBlock call re-fetches from the daemon. Called by Telepathy events.
func (a *Agent) InvalidateHotBlock() {
	atomic.StoreInt32(&a.hotBlockStale, 1)
}

// GetHotBlock retrieves the hot block from the daemon, using the cached
// version when available and fresh.
func (a *Agent) GetHotBlock(ctx context.Context) (string, error) {
	// If cache exists and not stale, return cached content (even if Mnemo is nil)
	if a.hotBlockCache != nil && atomic.LoadInt32(&a.hotBlockStale) == 0 {
		if content := a.hotBlockCache.Content(); content != "" {
			return content, nil
		}
	}

	if a.Mnemo == nil {
		return "", nil
	}

	// Fetch fresh hot block from daemon
	result, err := a.Mnemo.GetHotBlock(
		ctx,
		"memory",
		a.Config.AgentID,
		int32(a.Config.HotBlockConfig.CharLimit),
		a.Config.HotBlockConfig.BoostCategories,
		a.Config.HotBlockConfig.SuppressCategories,
		a.Config.HotBlockConfig.BoostPriority,
	)
	if err != nil {
		return "", err
	}

	// Update cache and mark as fresh
	if a.hotBlockCache != nil {
		a.hotBlockCache.Update(result, len([]rune(result)), 0) // entryCount unknown from RPC
	}
	atomic.StoreInt32(&a.hotBlockStale, 0)

	return result, nil
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

// StreamEvent is an event emitted during streaming agent execution.
// Text tokens are streamed in real-time; tool calls are emitted as
// discrete events; the final event has Done=true.
type StreamEvent struct {
	Type      agentctx.StreamEventType
	Text      string
	ToolName  string
	ToolCallID string
	ToolInput string
	Done      bool
	Error     error
	Usage     *agentctx.UsageInfo
}

// RunStreaming executes the agent loop with streaming text output.
// It returns a channel of StreamEvents for real-time display.
// Tool calls are still executed normally between turns.
func (a *Agent) RunStreaming(ctx context.Context, convCtx *agentctx.ConversationContext, out chan<- StreamEvent) {
	defer close(out)
	var sessionID string
	if a.Mnemo != nil && convCtx.SessionID != "" {
		sessionID = convCtx.SessionID
	}

	for {
		if convCtx.Interrupt.IsSet() {
			out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
			return
		}
		if convCtx.Iteration >= convCtx.MaxIterations {
			out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
			return
		}
		if a.Config.MaxContextTokens > 0 && convCtx.TokenCount() > a.Config.MaxContextTokens {
			out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
			return
		}

		// Stage 1: Request Preparation
		prepared := a.Stage1.Process(convCtx)

	// Stage 2: Try streaming with the router-selected provider
	p, err := a.selectProvider(convCtx)
	if err != nil {
		out <- StreamEvent{Error: fmt.Errorf("provider error: %w", err)}
		return
	}

	// Track the model name for failure demotion (provider name ≠ model name)
	modelName := ""
	if a.Router != nil {
		modelCfg, selectErr := a.Router.Select(provider.RoutingContext{
			Iteration:      convCtx.Iteration,
			UserMessage:     firstUserMessage(convCtx.Messages),
			HasToolResults:  lastMessageIsToolResult(convCtx.Messages),
			ToolCallCount:   countToolResults(convCtx.Messages),
			PromptCharCount: convCtx.TokenCount() * 4,
		})
		if selectErr == nil {
			modelName = modelCfg.Name
		}
	}

		req, err := p.Prepare(ctx, convCtx, prepared)
		if err != nil {
			out <- StreamEvent{Error: fmt.Errorf("prepare error: %w", err)}
			return
		}

		// Try streaming
		streamCh, streamErr := p.Stream(ctx, req)
		if streamErr != nil {
			// Streaming not supported — fall back to non-streaming
			a.runTurnNonStreaming(ctx, convCtx, out, prepared, p, sessionID, modelName)
			if convCtx.Iteration >= convCtx.MaxIterations {
				out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
				return
			}
			continue
		}

		// Accumulate streaming response
		var fullText string
		var toolCalls []agentctx.ToolCall
		var currentToolID string
		var currentToolName string
		var currentToolInput strings.Builder
		var lastUsage *agentctx.UsageInfo

		for event := range streamCh {
			switch event.Type {
			case agentctx.StreamText:
				out <- StreamEvent{Type: agentctx.StreamText, Text: event.Text}
				fullText += event.Text
			case agentctx.StreamToolStart:
				currentToolID = event.ToolCallID
				currentToolName = event.ToolName
				currentToolInput.Reset()
				out <- StreamEvent{Type: agentctx.StreamToolStart, ToolName: event.ToolName, ToolCallID: event.ToolCallID}
			case agentctx.StreamToolDelta:
				currentToolInput.WriteString(event.ToolInputDelta)
			case agentctx.StreamToolEnd:
				toolCalls = append(toolCalls, agentctx.ToolCall{
					ID:        currentToolID,
					Name:      currentToolName,
					Arguments: json.RawMessage(currentToolInput.String()),
				})
			case agentctx.StreamError:
				if a.Router != nil {
					failName := p.Name()
					if modelName != "" {
						failName = modelName
					}
					a.Router.RecordFailure(failName)
				}
				out <- StreamEvent{Error: event.Error}
				return
			case agentctx.StreamDone:
				// Forward usage info from provider and track cost
				lastUsage = event.Usage
				out <- StreamEvent{Type: agentctx.StreamDone, Done: true, Usage: event.Usage}
			}
		}

		// Build parsed response
		parsed := agentctx.ParsedResponse{
			Text:      fullText,
			ToolCalls: toolCalls,
		}
		if len(toolCalls) == 0 {
			parsed.FinishReason = "stop"
		} else {
			parsed.FinishReason = "tool_use"
		}

		// Track cost for this turn
		if a.Router != nil && lastUsage != nil {
			a.Router.RecordTurnCost(p.Name(), lastUsage.PromptTokens, lastUsage.CompletionTokens, convCtx.Iteration)
		}

		// Record success (reset failure counter)
		if a.Router != nil {
			a.Router.RecordSuccess(p.Name())
		}

		// Emit tool execution events before dispatch
		for _, tc := range toolCalls {
			out <- StreamEvent{Type: agentctx.StreamToolExec, ToolName: tc.Name, ToolCallID: tc.ID}
		}

		// Stage 4: Tool Dispatch (if tool calls present)
		dispatch := a.Stage4.Process(convCtx, parsed)

		// Stage 5: Context Management
		action := a.Stage5.Process(convCtx, dispatch.ToolResults, parsed)

		convCtx.Iteration++

		switch action {
		case agentctx.StageActionYIELD:
			a.persistSession(ctx, sessionID, convCtx)
			out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
			return
		case agentctx.StageActionTERMINATE:
			a.persistSession(ctx, sessionID, convCtx)
			out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
			return
		case agentctx.StageActionCONTINUE:
			// Tool call was made — continue the loop
			continue
		}
	}
}

// runTurnNonStreaming executes a single turn without streaming (fallback).
func (a *Agent) runTurnNonStreaming(ctx context.Context, convCtx *agentctx.ConversationContext, out chan<- StreamEvent, prepared agentctx.PreparedRequest, p provider.Provider, sessionID string, modelName string) {
	result := a.Stage2.Process(ctx, convCtx, prepared)
	if result.Error != nil {
		// Record failure for routing demotion — use model name if available
		if a.Router != nil {
			failName := p.Name()
			if modelName != "" {
				failName = modelName
			}
			a.Router.RecordFailure(failName)
		}
		// If this is not the primary provider, try the primary as immediate fallback
		if p.Name() != a.Registry.Primary() {
			if primary, err := a.Registry.GetPrimary(); err == nil {
				log.Printf("Provider %q failed, falling back to primary %q: %v", p.Name(), primary.Name(), result.Error)
				// Re-prepare and execute with primary
				req, err2 := primary.Prepare(ctx, convCtx, prepared)
				if err2 == nil {
					fallbackResult, execErr := primary.Execute(ctx, req)
					if execErr == nil && fallbackResult.Error == nil {
						result = fallbackResult
					}
				}
				if result.Error != nil {
					out <- StreamEvent{Error: fmt.Errorf("provider call failed: %w", result.Error)}
					return
				}
			} else {
				out <- StreamEvent{Error: fmt.Errorf("provider call failed: %w", result.Error)}
				return
			}
		} else {
			out <- StreamEvent{Error: fmt.Errorf("provider call failed: %w", result.Error)}
			return
		}
	}

	parsed, err := a.Stage3.Process(ctx, result)
	if err != nil {
		out <- StreamEvent{Error: fmt.Errorf("response processing failed: %w", err)}
		return
	}

	// Emit text as a single chunk
	if parsed.Text != "" {
		out <- StreamEvent{Type: agentctx.StreamText, Text: parsed.Text}
	}

	// Stage 4 & 5
	dispatch := a.Stage4.Process(convCtx, parsed)
	a.Stage5.Process(convCtx, dispatch.ToolResults, parsed)
}