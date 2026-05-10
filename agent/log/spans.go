// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agentlog

import "time"

// --- Pipeline Iteration Span ---

// PipelineIterationArgs holds the fields for pipeline.iteration log span.
type PipelineIterationArgs struct {
	Iteration int
	Model     string
	Provider  string
	ToolCount int
	Action    string // CONTINUE, YIELD, TERMINATE
}

// PipelineIteration logs a structured pipeline.iteration span.
func PipelineIteration(args PipelineIterationArgs) {
	Info("pipeline.iteration",
		"iteration", args.Iteration,
		"model", args.Model,
		"provider", args.Provider,
		"tool_count", args.ToolCount,
		"action", args.Action,
	)
}

// --- Tool Dispatch Span ---

// ToolDispatchArgs holds the fields for tool.dispatch log span.
type ToolDispatchArgs struct {
	ToolName string
	Duration time.Duration
	IsError  bool
}

// ToolDispatch logs a structured tool.dispatch span.
func ToolDispatch(args ToolDispatchArgs) {
	Info("tool.dispatch",
		"tool", args.ToolName,
		"duration_ms", args.Duration.Milliseconds(),
		"error", args.IsError,
	)
}

// --- Router Select Span ---

// RouterSelectArgs holds the fields for router.select log span.
type RouterSelectArgs struct {
	Strategy       string // primary, cost-optimized, fallback, etc.
	SelectedModel  string
	SelectedReason string // why this model was chosen
	Iteration      int    // optional: pipeline iteration at selection time
}

// RouterSelect logs a structured router.select span.
func RouterSelect(args RouterSelectArgs) {
	Info("router.select",
		"strategy", args.Strategy,
		"selected_model", args.SelectedModel,
		"select_reason", args.SelectedReason,
		"iteration", args.Iteration,
	)
}
