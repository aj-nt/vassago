package provider

import (
	"testing"
)

func testModels() map[string]*ModelConfig {
	return map[string]*ModelConfig{
		"premium": {
			Name:          "premium",
			Provider:      "anthropic",
			InputCost:     3.00,
			OutputCost:    15.00,
			ContextWindow: 200000,
			Reasoning:     "high",
			Code:          "high",
			ToolUse:       true,
			Vision:        false,
			Speed:         "medium",
		},
		"standard": {
			Name:          "standard",
			Provider:      "openai",
			InputCost:     0.15,
			OutputCost:    0.60,
			ContextWindow: 128000,
			Reasoning:     "medium",
			Code:          "medium",
			ToolUse:       true,
			Vision:        false,
			Speed:         "fast",
		},
		"local": {
			Name:          "local",
			Provider:      "ollama",
			InputCost:     0.00,
			OutputCost:    0.00,
			ContextWindow: 32000,
			Reasoning:     "low",
			Code:          "medium",
			ToolUse:       true,
			Vision:        false,
			Speed:         "fast",
		},
		"no-tools": {
			Name:          "no-tools",
			Provider:      "openai",
			InputCost:     0.10,
			OutputCost:    0.30,
			ContextWindow: 32000,
			Reasoning:     "low",
			Code:          "low",
			ToolUse:       false,
			Vision:        false,
			Speed:         "fast",
		},
	}
}

func newRouterWithModels(strategy RoutingStrategy) *ModelRouter {
	r := NewModelRouter(strategy)
	for _, m := range testModels() {
		r.Register(*m)
	}
	if err := r.SetPrimary("premium"); err != nil {
		panic(err)
	}
	return r
}

// --- Strategy: Always Primary ---

func TestRouterAlwaysPrimary(t *testing.T) {
	r := newRouterWithModels(StrategyAlwaysPrimary)

	m, err := r.Select(RoutingContext{Iteration: 0, UserMessage: "hello"})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if m.Name != "premium" {
		t.Errorf("expected premium, got %s", m.Name)
	}
}

func TestRouterAlwaysPrimaryDemoted(t *testing.T) {
	r := newRouterWithModels(StrategyAlwaysPrimary)

	// Demote premium with 3 failures
	r.RecordFailure("premium")
	r.RecordFailure("premium")
	r.RecordFailure("premium")

	m, err := r.Select(RoutingContext{})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	// Should fall back to another model, not premium
	if m.Name == "premium" {
		t.Error("premium should be demoted but was selected")
	}
}

// --- Strategy: Cost Optimized ---

func TestRouterCostOptimized(t *testing.T) {
	r := newRouterWithModels(StrategyCostOptimized)

	m, err := r.Select(RoutingContext{HasToolResults: false})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	// "no-tools" is cheapest but can't do tool use. "local" has 0 cost.
	if m.Name != "local" {
		t.Errorf("expected local (cheapest with tool_use), got %s", m.Name)
	}
}

func TestRouterCostOptimizedWithToolResults(t *testing.T) {
	r := newRouterWithModels(StrategyCostOptimized)

	m, err := r.Select(RoutingContext{HasToolResults: true})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	// "no-tools" can't handle tool use, "local" is cheapest that can
	if m.Name != "local" {
		t.Errorf("expected local (cheapest with tool_use), got %s", m.Name)
	}
	if m.Name == "no-tools" {
		t.Error("no-tools model should not be selected when tool results present")
	}
}

// --- Strategy: Quality Optimized ---

func TestRouterQualityOptimized(t *testing.T) {
	r := newRouterWithModels(StrategyQualityOptimized)

	m, err := r.Select(RoutingContext{})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if m.Name != "premium" {
		t.Errorf("expected premium (best reasoning), got %s", m.Name)
	}
}

// --- Strategy: Smart ---

func TestRouterSmartFirstTurn(t *testing.T) {
	r := newRouterWithModels(StrategySmart)

	// Short simple question on first turn → cheapest
	m, err := r.Select(RoutingContext{
		Iteration:      0,
		UserMessage:   "hi",
		PromptCharCount: 2,
	})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if m.Name != "local" {
		t.Errorf("expected local (cheap for simple first turn), got %s", m.Name)
	}
}

func TestRouterSmartComplexReasoning(t *testing.T) {
	r := newRouterWithModels(StrategySmart)

	// Long prompt with reasoning keyword → best model
	m, err := r.Select(RoutingContext{
		Iteration:      0,
		UserMessage:   "I need you to analyze and compare the performance characteristics of different sorting algorithms, specifically why quicksort is preferred over merge sort in practice despite worst-case O(n²) behavior. Should I refactor my current implementation to use introsort instead?",
		PromptCharCount: 250,
	})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if m.Name != "premium" {
		t.Errorf("expected premium (complex reasoning), got %s", m.Name)
	}
}

func TestRouterSmartToolResultFormatting(t *testing.T) {
	r := newRouterWithModels(StrategySmart)

	// Iteration > 0 with tool results → cheap model
	m, err := r.Select(RoutingContext{
		Iteration:      2,
		HasToolResults: true,
		UserMessage:   "search for project files",
		PromptCharCount: 25,
	})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if m.Name != "local" {
		t.Errorf("expected local (cheap for tool result formatting), got %s", m.Name)
	}
}

func TestRouterSmartMediumPrompt(t *testing.T) {
	r := newRouterWithModels(StrategySmart)

	// Medium prompt (100-200 chars, no reasoning keywords) → defaults to primary
	m, err := r.Select(RoutingContext{
		Iteration:      0,
		UserMessage:   "tell me about the project structure",
		PromptCharCount: 150,
	})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if m.Name != "premium" {
		t.Errorf("expected premium (default for medium prompt), got %s", m.Name)
	}
}

// --- Failure Demotion ---

func TestRouterFailureDemotion(t *testing.T) {
	r := newRouterWithModels(StrategyCostOptimized)

	// Local is normally cheapest, demote it
	r.RecordFailure("local")
	r.RecordFailure("local")
	r.RecordFailure("local")

	m, err := r.Select(RoutingContext{})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if m.Name == "local" {
		t.Error("local should be demoted but was selected")
	}
}

func TestRouterSuccessResetsFailures(t *testing.T) {
	r := newRouterWithModels(StrategyCostOptimized)

	r.RecordFailure("local")
	r.RecordFailure("local")
	r.RecordSuccess("local")

	m, err := r.Select(RoutingContext{})
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if m.Name != "local" {
		t.Errorf("expected local (failures reset), got %s", m.Name)
	}
}

// --- Cost Tracking ---

func TestRouterCostTracking(t *testing.T) {
	r := newRouterWithModels(StrategyAlwaysPrimary)

	r.RecordTurnCost("premium", 1000, 500, 1)
	r.RecordTurnCost("premium", 2000, 1000, 2)
	r.RecordTurnCost("local", 500, 200, 3)

	total, input, output := r.SessionCost()
	// Premium: $3/M input, $15/M output
	// Turn 1: 1000*3/1M + 500*15/1M = 0.003 + 0.0075 = 0.0105
	// Turn 2: 2000*3/1M + 1000*15/1M = 0.006 + 0.015 = 0.021
	// Turn 3 (local): free
	// Total: 0.0105 + 0.021 = 0.0315
	expectedCost := 0.0315

	if input != 3500 {
		t.Errorf("expected 3500 input tokens, got %d", input)
	}
	if output != 1700 {
		t.Errorf("expected 1700 output tokens, got %d", output)
	}
	// Cost with floating point tolerance
	if total < expectedCost-0.001 || total > expectedCost+0.001 {
		t.Errorf("expected cost ~%.6f, got %.6f", expectedCost, total)
	}
}

func TestRouterCostByModel(t *testing.T) {
	r := newRouterWithModels(StrategyAlwaysPrimary)

	r.RecordTurnCost("premium", 1000, 500, 1)
	r.RecordTurnCost("local", 500, 200, 2)

	summaries := r.SessionCostByModel()
	if len(summaries) != 2 {
		t.Fatalf("expected 2 model summaries, got %d", len(summaries))
	}

	premiumSummary := summaries["premium"]
	if premiumSummary.TurnCount != 1 {
		t.Errorf("expected 1 premium turn, got %d", premiumSummary.TurnCount)
	}
	if premiumSummary.InputTokens != 1000 {
		t.Errorf("expected 1000 premium input tokens, got %d", premiumSummary.InputTokens)
	}

	localSummary := summaries["local"]
	if localSummary.TurnCount != 1 {
		t.Errorf("expected 1 local turn, got %d", localSummary.TurnCount)
	}
}

func TestRouterResetSession(t *testing.T) {
	r := newRouterWithModels(StrategyAlwaysPrimary)

	r.RecordTurnCost("premium", 1000, 500, 1)
	r.ResetSession()

	total, input, output := r.SessionCost()
	if total != 0 || input != 0 || output != 0 {
		t.Errorf("expected zero costs after reset, got total=%.6f input=%d output=%d", total, input, output)
	}
}

// --- ModelConfig ---

func TestModelConfigCostForTokens(t *testing.T) {
	m := ModelConfig{
		Name:       "test",
		InputCost:  3.00,
		OutputCost: 15.00,
	}

	cost := m.CostForTokens(1_000_000, 1_000_000)
	if cost != 18.00 {
		t.Errorf("expected $18.00, got %.2f", cost)
	}

	cost = m.CostForTokens(0, 0)
	if cost != 0 {
		t.Errorf("expected $0.00, got %.2f", cost)
	}
}

func TestModelConfigCapabilities(t *testing.T) {
	m := ModelConfig{
		ContextWindow: 128000,
		ToolUse:       true,
		Vision:        true,
	}

	caps := m.Capabilities()
	if !caps.SupportsFunctionCalling {
		t.Error("expected tool use support")
	}
	if !caps.SupportsVision {
		t.Error("expected vision support")
	}
	if caps.MaxContextTokens != 128000 {
		t.Errorf("expected 128000 context tokens, got %d", caps.MaxContextTokens)
	}
}

// --- isComplexReasoning ---

func TestIsComplexReasoning(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		{"Why does the algorithm fail on edge cases?", true},
		{"Should I refactor the authentication module?", true},
		{"Analyze the performance bottlenecks", true},
		{"Compare REST vs GraphQL for our API", true},
		{"Debug the memory leak in worker pool", true},
		{"hi", false},
		{"search for files", false},
		{"what time is it", false},
	}

	for _, tt := range tests {
		// Simulate long enough prompt
		charCount := len(tt.msg)
		if tt.expected {
			charCount = 250 // Force length check to pass
		}
		result := isComplexReasoning(tt.msg, charCount)
		if result != tt.expected {
			t.Errorf("isComplexReasoning(%q, %d) = %v, want %v", tt.msg, charCount, result, tt.expected)
		}
	}
}

// --- SetPrimary validation ---

func TestRouterSetPrimaryInvalid(t *testing.T) {
	r := NewModelRouter(StrategyAlwaysPrimary)
	r.Register(ModelConfig{Name: "a", Provider: "openai"})

	err := r.SetPrimary("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent primary model")
	}
}

func TestRouterNoModels(t *testing.T) {
	r := NewModelRouter(StrategySmart)

	_, err := r.Select(RoutingContext{})
	if err == nil {
		t.Error("expected error for empty router")
	}
}