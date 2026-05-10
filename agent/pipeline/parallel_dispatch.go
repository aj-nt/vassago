// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

// ParallelToolDispatchStage routes tool calls to their handlers concurrently.
// Independent tools (those whose arguments don't reference prior tool outputs
// in the same turn) run in parallel. Dependent tools run sequentially.
//
// Phase 30: replaces the old sequential-only ToolDispatchStage.
type ParallelToolDispatchStage struct {
	Registry *tools.ToolRegistry
	EventBus *eventbus.EventBus
	// Approver is called before executing a tool. Return false to skip.
	Approver func(toolName string, args map[string]interface{}) bool
	// MaxParallelTools caps the number of concurrent tool executions.
	MaxParallelTools int
	// callCounts tracks how many times each tool has been called in the current turn.
	callCounts map[string]int
	callMu     sync.Mutex
}

// NewParallelToolDispatchStage creates a new concurrent Stage 4.
func NewParallelToolDispatchStage(registry *tools.ToolRegistry, eventBus *eventbus.EventBus) *ParallelToolDispatchStage {
	return &ParallelToolDispatchStage{
		Registry:         registry,
		EventBus:         eventBus,
		MaxParallelTools: 5, // Phase 30 default: aggressive parallelism
	}
}

// Process executes tool calls concurrently where possible.
func (s *ParallelToolDispatchStage) Process(convCtx *agentctx.ConversationContext, parsed agentctx.ParsedResponse) agentctx.ToolDispatchResult {
	if len(parsed.ToolCalls) == 0 {
		return agentctx.ToolDispatchResult{}
	}

	// Reset call counts at the start of each new turn
	if convCtx != nil && convCtx.Iteration == 0 {
		s.callCounts = make(map[string]int)
	} else if s.callCounts == nil {
		s.callCounts = make(map[string]int)
	}

	// Partition tool calls into dependency groups.
	// A tool is "dependent" if its arguments contain a JSON reference to a
	// prior tool call ID in this turn (e.g., "$call1", "${call2}").
	groups := s.partitionByDependency(parsed.ToolCalls)

	if len(groups) == 0 {
		return agentctx.ToolDispatchResult{}
	}

	// Execute each group sequentially, but tools within a group run concurrently.
	// Most turns have a single group (all independent) — this is the common fast path.
	var allResults []agentctx.ToolResult
	for _, group := range groups {
		results := s.dispatchGroup(group)
		allResults = append(allResults, results...)
	}

	// Restore original call order in results
	ordered := s.orderResults(allResults, parsed.ToolCalls)

	return agentctx.ToolDispatchResult{
		ToolResults: ordered,
	}
}

// partitionByDependency splits tool calls into dependency groups.
// Tools whose JSON arguments reference a prior call ID in this turn are
// placed in a new sequential group after the group containing the
// referenced call.
//
// Detection is simple: scan the raw JSON argument bytes for "$callID"
// patterns. This catches explicit references without full JSON parse.
func (s *ParallelToolDispatchStage) partitionByDependency(calls []agentctx.ToolCall) [][]agentctx.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	var groups [][]agentctx.ToolCall
	currentGroup := make([]agentctx.ToolCall, 0, len(calls))
	seenCallIDs := make(map[string]bool)

	for _, call := range calls {
		// Check if this call references any prior call ID
		dependsOn := s.findReferences(string(call.Arguments), seenCallIDs)

		if dependsOn {
			// Flush current group, start a new one
			if len(currentGroup) > 0 {
				groups = append(groups, currentGroup)
			}
			// Dependent tool runs alone in its own group (sequential with prior)
			groups = append(groups, []agentctx.ToolCall{call})
			currentGroup = make([]agentctx.ToolCall, 0)
		} else {
			currentGroup = append(currentGroup, call)
		}

		seenCallIDs[call.ID] = true
	}

	// Flush final group
	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}

	return groups
}

// findReferences checks if the JSON arguments contain a reference to any
// previously seen call ID. References are patterns like "$callID" or
// "${callID}" anywhere in the raw argument bytes.
func (s *ParallelToolDispatchStage) findReferences(argsJSON string, seenIDs map[string]bool) bool {
	for id := range seenIDs {
		if strings.Contains(argsJSON, "$"+id) || strings.Contains(argsJSON, "${"+id+"}") {
			return true
		}
	}
	return false
}

// dispatchGroup runs a group of independent tools concurrently.
// Results are collected in the order they complete, then reordered
// by the caller to match the original call order.
func (s *ParallelToolDispatchStage) dispatchGroup(group []agentctx.ToolCall) []agentctx.ToolResult {
	if len(group) == 0 {
		return nil
	}

	if len(group) == 1 {
		// Fast path: single tool, no concurrency overhead
		result := s.executeSingle(group[0])
		return []agentctx.ToolResult{result}
	}

	// Cap concurrency
	maxConc := s.MaxParallelTools
	if maxConc <= 0 {
		maxConc = 5
	}

	// Use a semaphore channel to bound concurrency
	sem := make(chan struct{}, maxConc)

	type indexedResult struct {
		idx    int
		result agentctx.ToolResult
	}
	resultCh := make(chan indexedResult, len(group))

	for i, call := range group {
		i, call := i, call // capture for goroutine
		go func() {
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			result := s.executeSingle(call)
			resultCh <- indexedResult{idx: i, result: result}
		}()
	}

	// Collect results
	results := make([]agentctx.ToolResult, len(group))
	for range group {
		ir := <-resultCh
		results[ir.idx] = ir.result
	}

	return results
}

// executeSingle runs a single tool call through the full validation +
// approval + execution pipeline. This is the same logic as the old
// ToolDispatchStage but extracted for reuse in both sequential and
// parallel paths.
func (s *ParallelToolDispatchStage) executeSingle(tc agentctx.ToolCall) agentctx.ToolResult {
	// Cap check: if MaxParallelTools < len(group), excess calls get truncated
	// (the old behavior — now only applies when MaxParallelTools < 1)

	if s.EventBus != nil {
		s.EventBus.Emit(eventbus.Event{Kind: "tool_call", Data: tc.Name})
	}

	handler, err := s.Registry.Get(tc.Name)
	if err != nil {
		return agentctx.ToolResult{
			CallID: tc.ID, Name: tc.Name,
			Result:  fmt.Sprintf("unknown tool: %s", tc.Name),
			IsError: true,
		}
	}

	// Budget check (thread-safe)
	s.callMu.Lock()
	count := s.callCounts[tc.Name]
	s.callMu.Unlock()

	if !tools.CheckBudget(tc.Name, count) {
		return agentctx.ToolResult{
			CallID: tc.ID, Name: tc.Name,
			Result:  tools.BudgetExceededMessage(tc.Name, count),
			IsError: true,
		}
	}

	// Parse arguments
	var args map[string]interface{}
	rawArgs := tc.Arguments
	if len(rawArgs) == 0 || string(rawArgs) == "null" {
		rawArgs = json.RawMessage("{}")
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return agentctx.ToolResult{
			CallID: tc.ID, Name: tc.Name,
			Result:  fmt.Sprintf("invalid arguments: %v", err),
			IsError: true,
		}
	}

	// Validate against JSON Schema
	if schema, ok := s.Registry.Schemas()[tc.Name]; ok {
		tools.ApplyDefaults(args, schema)
		if err := tools.ValidateArgs(tc.Name, args, schema); err != nil {
			verr, ok := err.(*tools.ValidationError)
			if !ok {
				verr = &tools.ValidationError{Tool: tc.Name, Field: "", Issue: err.Error()}
			}
			return agentctx.ToolResult{
				CallID: tc.ID, Name: tc.Name,
				Result:  fmt.Sprintf("Invalid argument: %s. Please check the tool schema and try again.", verr.Error()),
				IsError: true,
			}
		}
	}

	// Approval check
	if s.Approver != nil && !s.Approver(tc.Name, args) {
		if s.EventBus != nil {
			s.EventBus.Emit(eventbus.Event{Kind: "tool_denied", Data: tc.Name})
		}
		return agentctx.ToolResult{
			CallID: tc.ID, Name: tc.Name,
			Result:  "tool execution skipped: user denied approval",
			IsError: true,
		}
	}

	// Execute
	result, isErr := safeCallHandler(handler, args)
	if s.EventBus != nil {
		s.EventBus.Emit(eventbus.Event{Kind: "tool_result", Data: result})
	}

	// Increment call count
	s.callMu.Lock()
	s.callCounts[tc.Name]++
	s.callMu.Unlock()

	return agentctx.ToolResult{
		CallID: tc.ID, Name: tc.Name,
		Result:  result,
		IsError: isErr,
	}
}

// orderResults sorts results to match the original call order.
func (s *ParallelToolDispatchStage) orderResults(results []agentctx.ToolResult, calls []agentctx.ToolCall) []agentctx.ToolResult {
	// Build a map from call ID to result (one pass)
	byID := make(map[string]agentctx.ToolResult, len(results))
	for _, r := range results {
		byID[r.CallID] = r
	}

	// Reconstruct in call order
	ordered := make([]agentctx.ToolResult, 0, len(calls))
	for _, call := range calls {
		if r, ok := byID[call.ID]; ok {
			ordered = append(ordered, r)
		}
	}

	return ordered
}
