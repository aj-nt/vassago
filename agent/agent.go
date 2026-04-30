package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

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
	a.Stage4 = pipeline.NewToolDispatchStage(toolReg, bus)
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

		// Stage 2: Try streaming with the primary provider
		provider, err := a.Registry.GetPrimary()
		if err != nil {
			out <- StreamEvent{Error: fmt.Errorf("provider error: %w", err)}
			return
		}

		req, err := provider.Prepare(ctx, convCtx, prepared)
		if err != nil {
			out <- StreamEvent{Error: fmt.Errorf("prepare error: %w", err)}
			return
		}

		// Try streaming
		streamCh, streamErr := provider.Stream(ctx, req)
		if streamErr != nil {
			// Streaming not supported — fall back to non-streaming
			a.runTurnNonStreaming(ctx, convCtx, out, prepared, provider, sessionID)
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
				out <- StreamEvent{Error: event.Error}
				return
			case agentctx.StreamDone:
				// Forward usage info from provider
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
func (a *Agent) runTurnNonStreaming(ctx context.Context, convCtx *agentctx.ConversationContext, out chan<- StreamEvent, prepared agentctx.PreparedRequest, provider provider.Provider, sessionID string) {
	result := a.Stage2.Process(ctx, convCtx, prepared)
	if result.Error != nil {
		out <- StreamEvent{Error: fmt.Errorf("provider call failed: %w", result.Error)}
		return
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