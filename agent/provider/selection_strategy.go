// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package provider

// SelectionStrategy selects a model for a given routing context.
// Implementations define different routing policies (always-primary,
// cost-optimized, quality-optimized, smart).
type SelectionStrategy interface {
	// Select chooses the best model for the current turn.
	// Returns the selected ModelConfig or an error if no model is available.
	Select(ctx RoutingContext, router *ModelRouter) (*ModelConfig, error)
}

// AlwaysPrimaryStrategy always uses the configured primary model.
// Falls back to other models only if primary is demoted.
type AlwaysPrimaryStrategy struct{}

// Select returns the primary model, or a fallback if demoted.
func (s *AlwaysPrimaryStrategy) Select(ctx RoutingContext, router *ModelRouter) (*ModelConfig, error) {
	return router.selectPrimary()
}

// CostOptimizedStrategy picks the cheapest model that can handle the turn.
type CostOptimizedStrategy struct{}

// Select returns the cheapest capable model that's not demoted.
func (s *CostOptimizedStrategy) Select(ctx RoutingContext, router *ModelRouter) (*ModelConfig, error) {
	return router.selectCheapest(ctx)
}

// QualityOptimizedStrategy picks the model with the highest reasoning capability.
type QualityOptimizedStrategy struct{}

// Select returns the highest-reasoning model that's not demoted.
func (s *QualityOptimizedStrategy) Select(ctx RoutingContext, router *ModelRouter) (*ModelConfig, error) {
	return router.selectBest(ctx)
}

// SmartStrategy uses heuristic task-based routing per turn.
// Tool-result turns → primary model; simple prompts → cheap model.
type SmartStrategy struct{}

// Select uses heuristic rules for per-turn model selection.
func (s *SmartStrategy) Select(ctx RoutingContext, router *ModelRouter) (*ModelConfig, error) {
	return router.selectSmart(ctx)
}

// strategyFromName creates a SelectionStrategy from a RoutingStrategy name.
// Returns AlwaysPrimaryStrategy for unknown/empty values.
func strategyFromName(name RoutingStrategy) SelectionStrategy {
	switch name {
	case StrategyCostOptimized:
		return &CostOptimizedStrategy{}
	case StrategyQualityOptimized:
		return &QualityOptimizedStrategy{}
	case StrategySmart:
		return &SmartStrategy{}
	case StrategyAlwaysPrimary:
		return &AlwaysPrimaryStrategy{}
	default:
		return &AlwaysPrimaryStrategy{}
	}
}