// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	agentlog "github.com/aj-nt/mnemosyne/agent/log"
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
	Mnemo memoclient.MnemoClient

	// Hot block cache — updated by Telepathy events, used by GetHotBlock()
	hotBlockCache *memoclient.HotBlockCache
	hotBlockStale int32 // atomic: 1 = stale, needs refetch

	// Pipeline stages — overridable for testing
	Stage1 *pipeline.RequestPrepStage
	Stage2 *pipeline.ProviderCallStage
	Stage3 *pipeline.ResponseProcessingStage
	Stage4 *pipeline.ParallelToolDispatchStage
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
	MaxMutations     int // Max AddMemory calls per session; 0 = default (10), -1 = unlimited
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
func NewAgent(cfg Config, registry *provider.ProviderRegistry, toolReg *tools.ToolRegistry, mnemo memoclient.MnemoClient) *Agent {
	return NewAgentBuilder(cfg, registry, toolReg, mnemo).Build()
}

// NewAgentWithRouter creates a new agent with model routing.
func NewAgentWithRouter(cfg Config, registry *provider.ProviderRegistry, router *provider.ModelRouter, toolReg *tools.ToolRegistry, mnemo memoclient.MnemoClient) *Agent {
	return NewAgentBuilder(cfg, registry, toolReg, mnemo).WithRouter(router).Build()
}

// Close cleans up agent resources.
func (a *Agent) Close() {
	if a.EventBus != nil {
		a.EventBus.Close()
	}
}

// selectProvider picks the provider for this turn using the model router.
// Falls back to the registry's primary if no router is configured.
func (a *Agent) selectProvider(convCtx *agentctx.ConversationContext) (provider.Provider, error) {
	if a.Router == nil {
		p, err := a.Registry.GetPrimary()
		if err == nil {
			convCtx.SetTokenCounter(nil) // Use default (chars/4) when no router
		}
		return p, err
	}

	// Build routing context from conversation state
	routingCtx := provider.RoutingContext{
		Iteration:       convCtx.Iteration,
		UserMessage:     convCtx.FirstUserMessage(),
		HasToolResults:  convCtx.LastMessageIsToolResult(),
		ToolCallCount:   convCtx.ToolResultCount(),
		PromptCharCount: convCtx.CharCount(),
	}

	modelCfg, err := a.Router.Select(routingCtx)
	if err != nil {
		// Fallback to primary provider on routing error
		agentlog.RouterSelect(agentlog.RouterSelectArgs{
			Strategy:       "fallback",
			SelectedModel:  "primary",
			SelectedReason: fmt.Sprintf("router error: %v", err),
			Iteration:      convCtx.Iteration,
		})
		return a.Registry.GetPrimary()
	}

	agentlog.RouterSelect(agentlog.RouterSelectArgs{
		Strategy:       string(a.Router.Strategy()),
		SelectedModel:  modelCfg.Name,
		SelectedReason: fmt.Sprintf("iteration=%d tools=%d chars=%d", routingCtx.Iteration, routingCtx.ToolCallCount, routingCtx.PromptCharCount),
		Iteration:      convCtx.Iteration,
	})

	// Update token counter for the selected model
	convCtx.SetTokenCounter(agentctx.CounterForModel(modelCfg.Name))

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

// Run executes the agent loop for a user message.
// This is the simple API — creates a fresh context each time.
func (a *Agent) Run(ctx context.Context, userMessage string) (*agentctx.PipelineResult, error) {
	// Get hot block from daemon
	hotBlock, err := a.GetHotBlock(ctx)
	if err != nil {
		agentlog.Warn("failed to get hot block", "error", err)
		hotBlock = ""
	}

	skillBlock := a.GetSkillBlock(ctx)

	// Register saved tools as dynamic handlers (trigger-ranked, loaded from daemon)
	a.RegisterSavedToolHandlers(ctx, userMessage)

	// Create session with auto-generated title from first message
	title := autoTitle(userMessage)
	sessionID, _ := a.Mnemo.CreateSession(ctx, a.Config.AgentID, "cli", title)

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
		SkillContent:    skillBlock,
		Temperature:     a.Config.Temperature,
	}

	return a.RunWithContext(ctx, convCtx)
}

// RunWithContext executes the agent loop using an existing conversation context.
// This is the main loop — it runs the 5-stage pipeline until YIELD or TERMINATE.
func (a *Agent) RunWithContext(ctx context.Context, convCtx *agentctx.ConversationContext) (*agentctx.PipelineResult, error) {
	var sessionID string
	if convCtx.SessionID != "" {
		sessionID = convCtx.SessionID
	}

	caller := pipeline.NewBlockingCaller(a.Stage2, a.Stage3)
	cfg := &pipeline.LoopConfig{
		Stages: &pipeline.Stages{
			Stage1: a.Stage1,
			Stage2: a.Stage2,
			Stage3: a.Stage3,
			Stage4: a.Stage4,
			Stage5: a.Stage5,
		},
		MaxContextTokens:     a.Config.MaxContextTokens,
		EventBus:             a.EventBus,
		MaxConsecutiveErrors: 1, // Blocking mode: any provider error is fatal (no retries)
		PersistFunc: func(sid string) {
			a.persistSession(ctx, sid, convCtx)
		},
		FinallyFunc: func(sid string, result *pipeline.LoopResult) {
			if !skipMemoryExtraction(result) {
				a.emitConversationComplete(sid, convCtx)
			}
		},
	}

	result := pipeline.RunPipelineLoop(ctx, convCtx, cfg, caller, sessionID)

	if result.Error != nil {
		return nil, result.Error
	}

	return &agentctx.PipelineResult{
		Response:         &result.Parsed,
		Iterations:       result.Iterations,
		Interrupted:      result.Interrupted,
		MaxIterationsHit: result.MaxIterationsHit,
		SessionID:        sessionID,
	}, nil
}

// --- Memory Extraction ---
//
// Memory extraction is decoupled from the pipeline loop via the EventBus.
// When a conversation completes, FinallyFunc emits a conversation_complete event.
// MemoryExtractionSubscriber (wired in AgentBuilder.Build()) listens for these
// events and runs extraction asynchronously with the cheapest model.
// See memory_extraction_subscriber.go for the subscriber implementation.

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

// GetSkillBlock retrieves high-priority skills from the daemon and formats
// them for the system prompt. Skills are procedural knowledge — reusable
// approaches that any agent instance can follow.
func (a *Agent) GetSkillBlock(ctx context.Context) string {
	skills, err := a.Mnemo.SearchSkills(ctx, "", "", 50)
	if err != nil || len(skills) == 0 {
		return ""
	}

	var buf strings.Builder
	for _, s := range skills {
		fmt.Fprintf(&buf, "## %s (%s)\n", s.Name, s.Category)
		if s.Description != "" {
			fmt.Fprintf(&buf, "%s\n", s.Description)
		}
		if len(s.Triggers) > 0 {
			fmt.Fprintf(&buf, "Triggers: %s\n", strings.Join(s.Triggers, ", "))
		}
		buf.WriteString(s.Content)
		buf.WriteString("\n\n")
	}
	return buf.String()
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
	if sessionID == "" {
		return
	}
	msgs := make([]memoclient.Message, len(convCtx.Messages))
	for i, m := range convCtx.Messages {
		msgs[i] = memoclient.Message{Role: m.Role, Content: m.Content}
	}
	if err := a.Mnemo.AddMessages(ctx, sessionID, msgs); err != nil {
		agentlog.Warn("failed to persist session", "error", err)
	}
}

// autoTitle generates a concise session title from the user's first message.
// It truncates to 60 characters at the last word boundary to avoid cutting words.
func autoTitle(message string) string {
	title := strings.TrimSpace(message)

	// Replace newlines with spaces
	title = strings.ReplaceAll(title, "\n", " ")

	// Collapse multiple spaces
	for strings.Contains(title, "  ") {
		title = strings.ReplaceAll(title, "  ", " ")
	}

	const maxLen = 60
	if len(title) <= maxLen {
		return title
	}

	// Truncate at word boundary
	truncated := title[:maxLen]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}
	return truncated + "…"
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
	Reasoning string
}

// RunStreaming executes the agent loop with streaming text output.
// It returns a channel of StreamEvents for real-time display.
// Tool calls are still executed normally between turns.
func (a *Agent) RunStreaming(ctx context.Context, convCtx *agentctx.ConversationContext, out chan<- StreamEvent) {
	defer close(out)
	var sessionID string
	if convCtx.SessionID != "" {
		sessionID = convCtx.SessionID
	}

	caller := newStreamingCaller(a, out, ctx, convCtx)
	cfg := &pipeline.LoopConfig{
		Stages: &pipeline.Stages{
			Stage1: a.Stage1,
			Stage2: a.Stage2,
			Stage3: a.Stage3,
			Stage4: a.Stage4,
			Stage5: a.Stage5,
		},
		MaxContextTokens:     a.Config.MaxContextTokens,
		EventBus:             a.EventBus,
		MaxConsecutiveErrors: 3,
		PersistFunc: func(sid string) {
			a.persistSession(ctx, sid, convCtx)
		},
		FinallyFunc: func(sid string, result *pipeline.LoopResult) {
			if !skipMemoryExtraction(result) {
				a.emitConversationComplete(sid, convCtx)
			}
		},
	}

	result := pipeline.RunPipelineLoop(ctx, convCtx, cfg, caller, sessionID)

	// Handle loop result — convert LoopResult to stream events
	switch {
	case result.Interrupted:
		out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
	case result.MaxIterationsHit:
		if result.ContextOverflow {
			out <- StreamEvent{Type: agentctx.StreamText, Text: "Context limit reached — conversation too long to continue."}
		}
		out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
	case result.StoppedOnError:
		if result.Error != nil {
			out <- StreamEvent{Error: result.Error}
		}
		out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
	case result.Action == agentctx.StageActionYIELD:
		// Only emit parsed.Text as StreamText if it's fallback text injected by
		// the loop (loop detection with empty LLM response). If the LLM already
		// produced text during the loop, it was streamed in real-time and
		// re-emitting would duplicate it.
		if result.FallbackText && result.Parsed.Text != "" {
			out <- StreamEvent{Type: agentctx.StreamText, Text: result.Parsed.Text}
		}
		out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
	case result.Action == agentctx.StageActionTERMINATE:
		// Context overflow or max iterations — emit informative message
		if result.ContextOverflow {
			out <- StreamEvent{Type: agentctx.StreamText, Text: "Context limit reached — conversation too long to continue."}
		}
		out <- StreamEvent{Type: agentctx.StreamDone, Done: true}
	}
}