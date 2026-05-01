package provider

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// ModelConfig describes a single model's metadata for routing decisions.
type ModelConfig struct {
	// Identity
	Name     string `yaml:"name"`     // e.g., "claude-sonnet-4-6"
	Provider string `yaml:"provider"` // e.g., "anthropic", "openai"

	// Cost in USD per 1M tokens
	InputCost  float64 `yaml:"input_cost"`  // e.g., 3.00
	OutputCost float64 `yaml:"output_cost"` // e.g., 15.00

	// Capabilities
	ContextWindow int    `yaml:"context_window"` // e.g., 128000
	Reasoning     string `yaml:"reasoning"`      // "low", "medium", "high"
	Code          string `yaml:"code"`           // "low", "medium", "high"
	ToolUse       bool   `yaml:"tool_use"`       // supports function calling
	Vision        bool   `yaml:"vision"`         // supports image input
	Speed         string `yaml:"speed"`          // "fast", "medium", "slow"

	// Endpoint for health-checking (populated from provider, not from YAML).
	// Empty string = skip health check (assume reachable, e.g., cloud APIs).
	BaseURL string `yaml:"-"`
}

// Capabilities returns the provider capabilities this model supports.
func (m *ModelConfig) Capabilities() agentctx.ProviderCapabilities {
	return agentctx.ProviderCapabilities{
		SupportsStreaming:       true,
		SupportsFunctionCalling: m.ToolUse,
		SupportsPromptCaching:  false,
		SupportsVision:         m.Vision,
		MaxContextTokens:       m.ContextWindow,
	}
}

// CostForTokens calculates the cost for a given number of tokens.
func (m *ModelConfig) CostForTokens(input, output int) float64 {
	return (float64(input)/1_000_000)*m.InputCost + (float64(output)/1_000_000)*m.OutputCost
}

// RoutingStrategy determines how the router selects models.
type RoutingStrategy string

const (
	// StrategyAlwaysPrimary uses the primary model for every turn (default, backward compatible).
	StrategyAlwaysPrimary RoutingStrategy = "always-primary"

	// StrategyCostOptimized picks the cheapest model that supports needed capabilities.
	StrategyCostOptimized RoutingStrategy = "cost-optimized"

	// StrategyQualityOptimized always picks the best (highest reasoning) model.
	StrategyQualityOptimized RoutingStrategy = "quality-optimized"

	// StrategySmart uses heuristic task-based routing per turn.
	StrategySmart RoutingStrategy = "smart"
)

// RoutingContext provides information about the current turn for routing decisions.
type RoutingContext struct {
	Iteration       int    // Current iteration (0 = first turn)
	UserMessage     string // The user's original message
	HasToolResults  bool   // Previous tool calls returned results
	ToolCallCount   int    // Number of tool results in context
	PromptCharCount int    // Character count of the current prompt
}

// ModelRouter selects the best model for each turn based on the routing strategy.
type ModelRouter struct {
	mu       sync.RWMutex
	models   map[string]*ModelConfig // name -> config
	primary  string                  // Primary model name
	strategy RoutingStrategy

	// Failure tracking for demotion
	failures    map[string]int // model name -> consecutive failure count
	lastFailure map[string]time.Time

	// Cost tracking
	costMu  sync.Mutex
	sessionCosts []TurnCost
}

// TurnCost records the cost of a single turn.
type TurnCost struct {
	Model          string
	InputTokens    int
	OutputTokens   int
	CostUSD        float64
	TurnNumber     int
	Timestamp      time.Time
}

// NewModelRouter creates a new model router.
func NewModelRouter(strategy RoutingStrategy) *ModelRouter {
	if strategy == "" {
		strategy = StrategyAlwaysPrimary
	}
	return &ModelRouter{
		models:      make(map[string]*ModelConfig),
		strategy:    strategy,
		failures:    make(map[string]int),
		lastFailure: make(map[string]time.Time),
	}
}

// Register adds a model to the router.
func (r *ModelRouter) Register(cfg ModelConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[cfg.Name] = &cfg
}

// SetPrimary sets the primary model name.
func (r *ModelRouter) SetPrimary(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.models[name]; !ok {
		return fmt.Errorf("model not found: %s", name)
	}
	r.primary = name
	return nil
}

// Primary returns the primary model name.
func (r *ModelRouter) Primary() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.primary
}

// Get returns a model config by name.
func (r *ModelRouter) Get(name string) (*ModelConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.models[name]
	if !ok {
		return nil, fmt.Errorf("model not found: %s", name)
	}
	return m, nil
}

// List returns all registered model names.
func (r *ModelRouter) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.models))
	for name := range r.models {
		names = append(names, name)
	}
	return names
}

// Select chooses the best model for the current turn.
func (r *ModelRouter) Select(ctx RoutingContext) (*ModelConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.models) == 0 {
		return nil, fmt.Errorf("no models registered")
	}

	switch r.strategy {
	case StrategyAlwaysPrimary:
		return r.selectPrimary()

	case StrategyCostOptimized:
		return r.selectCheapest(ctx)

	case StrategyQualityOptimized:
		return r.selectBest(ctx)

	case StrategySmart:
		return r.selectSmart(ctx)

	default:
		return r.selectPrimary()
	}
}

// selectPrimary returns the configured primary model (not demoted).
func (r *ModelRouter) selectPrimary() (*ModelConfig, error) {
	m, ok := r.models[r.primary]
	if !ok {
		return nil, fmt.Errorf("primary model not found: %s", r.primary)
	}
	if r.isDemoted(r.primary) {
		// Find next best model
		return r.findFallback()
	}
	return m, nil
}

// selectCheapest picks the cheapest model that can handle the turn.
func (r *ModelRouter) selectCheapest(ctx RoutingContext) (*ModelConfig, error) {
	var best *ModelConfig
	var bestCost float64 = -1

	for _, m := range r.models {
		if r.isDemoted(m.Name) {
			continue
		}
		// Skip models that can't do tool use if we have tool results
		if ctx.HasToolResults && !m.ToolUse {
			continue
		}
		cost := m.InputCost + m.OutputCost // Simplified: cost per 1M total
		if best == nil || cost < bestCost {
			best = m
			bestCost = cost
		}
	}

	if best == nil {
		return r.findFallback()
	}
	return best, nil
}

// selectBest picks the model with the highest reasoning capability.
func (r *ModelRouter) selectBest(ctx RoutingContext) (*ModelConfig, error) {
	var best *ModelConfig
	bestRank := -1

	reasoningRank := map[string]int{"low": 0, "medium": 1, "high": 2}

	for _, m := range r.models {
		if r.isDemoted(m.Name) {
			continue
		}
		rank := reasoningRank[m.Reasoning]
		if ctx.HasToolResults && !m.ToolUse {
			continue
		}
		if rank > bestRank {
			best = m
			bestRank = rank
		}
	}

	if best == nil {
		return r.findFallback()
	}
	return best, nil
}

// selectSmart uses heuristic rules for per-turn model selection.
//
// Rules (in priority order):
//  1. Complex reasoning (long prompt with reasoning keywords) → best model
//  2. Tool-result turns → primary model (needs good reasoning to interpret results)
//  3. Default → primary model (Claude for quality, GLM-5.1 for cost if routing to cheap)
//
// Cost note: Claude Sonnet 4.6 costs $3/$15 per MTok. At typical usage this
// is ~$0.03-0.10/session. Tool-result turns are the most expensive (full context
// sent) so they should always use the best model.
func (r *ModelRouter) selectSmart(ctx RoutingContext) (*ModelConfig, error) {
	// Rule 1: Complex reasoning detected
	if isComplexReasoning(ctx.UserMessage, ctx.PromptCharCount) {
		if m := r.findBestReasoning(true); m != nil && !r.isDemoted(m.Name) {
			return m, nil
		}
	}

	// Rule 2: Default to primary
	// All turns use primary (Claude) for quality. GLM-5.1 is available as
	// fallback but not routed to automatically — it's a free safety net.
	return r.selectPrimary()
}

// isComplexReasoning detects if the prompt likely needs strong reasoning.
func isComplexReasoning(message string, charCount int) bool {
	if charCount < 200 && len(message) < 200 {
		return false
	}
	keywords := []string{
		"why", "should", "analyze", "compare", "architect",
		"design", "evaluate", "reason", "critique", "explain in detail",
		"refactor", "strategy", "optimize", "debug", "investigate",
	}
	lower := strings.ToLower(message)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// isReachable checks if the model's endpoint is reachable via a quick TCP dial.
// Models with no BaseURL (cloud APIs like Anthropic/OpenAI) are assumed reachable.
// Local endpoints (e.g., Ollama at localhost:11434) get a 2-second TCP health-check.
func (r *ModelRouter) isReachable(m *ModelConfig) bool {
	if m.BaseURL == "" {
		return true // Cloud API — assume reachable
	}
	u, err := url.Parse(m.BaseURL)
	if err != nil {
		return false // Invalid URL — skip this model
	}
	host := u.Host
	if host == "" {
		return true // Can't determine host — assume reachable
	}
	// Add default port if missing
	if !strings.Contains(host, ":") {
		switch u.Scheme {
		case "https":
			host += ":443"
		case "http":
			host += ":80"
		default:
			host += ":80"
		}
	}
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// findCheapestCapable returns the cheapest model with tool_use support.
func (r *ModelRouter) findCheapestCapable(toolUse bool) *ModelConfig {
	var best *ModelConfig
	var bestCost float64 = -1

	for _, m := range r.models {
		if r.isDemoted(m.Name) {
			continue
		}
		if toolUse && !m.ToolUse {
			continue
		}
		cost := m.InputCost + m.OutputCost
		if best == nil || cost < bestCost {
			best = m
			bestCost = cost
		}
	}
	return best
}

// findBestReasoning returns the model with the highest reasoning capability.
func (r *ModelRouter) findBestReasoning(toolUse bool) *ModelConfig {
	reasoningRank := map[string]int{"low": 0, "medium": 1, "high": 2}
	var best *ModelConfig
	bestRank := -1

	for _, m := range r.models {
		if r.isDemoted(m.Name) {
			continue
		}
		if toolUse && !m.ToolUse {
			continue
		}
		rank := reasoningRank[m.Reasoning]
		if rank > bestRank {
			best = m
			bestRank = rank
		}
	}
	return best
}

// findFallback returns any non-demoted model, or the primary if all are demoted.
func (r *ModelRouter) findFallback() (*ModelConfig, error) {
	for _, m := range r.models {
		if !r.isDemoted(m.Name) {
			return m, nil
		}
	}
	// All demoted — use primary anyway (better than nothing)
	if m, ok := r.models[r.primary]; ok {
		return m, nil
	}
	return nil, fmt.Errorf("no models available")
}

// isDemoted checks if a model has been temporarily demoted due to failures.
// A model is demoted after 1 failure (immediate demotion for wrong-format responses,
// unreachable endpoints, etc. — there's no point retrying a model that just failed).
func (r *ModelRouter) isDemoted(name string) bool {
	count := r.failures[name]
	if count < 1 {
		return false
	}
	// Auto-recover after 5 minutes
	last, ok := r.lastFailure[name]
	if ok && time.Since(last) > 5*time.Minute {
		delete(r.failures, name)
		delete(r.lastFailure, name)
		return false
	}
	return true
}

// RecordFailure records a model failure. After 1 failure, the model is immediately demoted.
func (r *ModelRouter) RecordFailure(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures[name]++
	r.lastFailure[name] = time.Now()
}

// RecordSuccess resets the failure counter for a model.
func (r *ModelRouter) RecordSuccess(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.failures, name)
	delete(r.lastFailure, name)
}

// RecordTurnCost records the cost of a completed turn.
func (r *ModelRouter) RecordTurnCost(model string, inputTokens, outputTokens int, turnNumber int) {
	r.costMu.Lock()
	defer r.costMu.Unlock()

	m, err := r.Get(model)
	if err != nil {
		return
	}

	r.sessionCosts = append(r.sessionCosts, TurnCost{
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      m.CostForTokens(inputTokens, outputTokens),
		TurnNumber:   turnNumber,
		Timestamp:    time.Now(),
	})
}

// SessionCost returns the total cost for the current session.
func (r *ModelRouter) SessionCost() (totalUSD float64, totalInput, totalOutput int) {
	r.costMu.Lock()
	defer r.costMu.Unlock()

	for _, tc := range r.sessionCosts {
		totalUSD += tc.CostUSD
		totalInput += tc.InputTokens
		totalOutput += tc.OutputTokens
	}
	return
}

// SessionCostByModel returns cost breakdown per model.
func (r *ModelRouter) SessionCostByModel() map[string]TurnCostSummary {
	r.costMu.Lock()
	defer r.costMu.Unlock()

	summaries := make(map[string]TurnCostSummary)
	for _, tc := range r.sessionCosts {
		s := summaries[tc.Model]
		s.InputTokens += tc.InputTokens
		s.OutputTokens += tc.OutputTokens
		s.CostUSD += tc.CostUSD
		s.TurnCount++
		summaries[tc.Model] = s
	}
	return summaries
}

// TurnCostSummary aggregates costs across turns for a single model.
type TurnCostSummary struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	TurnCount    int
}

// ResetSession clears the session cost tracking.
func (r *ModelRouter) ResetSession() {
	r.costMu.Lock()
	defer r.costMu.Unlock()
	r.sessionCosts = nil
}

// Strategy returns the current routing strategy.
func (r *ModelRouter) Strategy() RoutingStrategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.strategy
}