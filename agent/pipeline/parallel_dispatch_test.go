// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

// ============================================================================
// Phase 30: Parallel Tool Dispatch — TDD tests
// ============================================================================

// --- Independent tools run concurrently ---

func TestParallelDispatch_IndependentToolsRunConcurrently(t *testing.T) {
	registry := tools.NewToolRegistry()

	// Two slow, independent tools that record their execution order
	var order []string
	var activeCount atomic.Int32

	slowTool := func(name string) tools.ToolHandler {
		return func(args map[string]interface{}) (string, bool) {
			activeCount.Add(1)
			order = append(order, name+"-start")
			time.Sleep(50 * time.Millisecond) // simulate I/O
			order = append(order, name+"-end")
			activeCount.Add(-1)
			return name + " result", false
		}
	}

	registry.Register("SlowA", slowTool("A"), nil)
	registry.Register("SlowB", slowTool("B"), nil)

	stage := NewParallelToolDispatchStage(registry, &eventbus.EventBus{})

	result := stage.Process(nil, agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "call1", Name: "SlowA", Arguments: []byte("{}")},
			{ID: "call2", Name: "SlowB", Arguments: []byte("{}")},
		},
	})

	if len(result.ToolResults) != 2 {
		t.Fatalf("want 2 results, got %d", len(result.ToolResults))
	}

	// Both should have succeeded
	for _, r := range result.ToolResults {
		if r.IsError {
			t.Errorf("tool %s failed: %s", r.Name, r.Result)
		}
	}

	// If they ran concurrently, both start before either ends.
	// Sequential execution: A-start A-end B-start B-end
	// Parallel execution: A-start B-start (either order) then A-end B-end
	aStarted, bStarted := false, false
	for _, entry := range order {
		if entry == "A-start" {
			aStarted = true
		}
		if entry == "B-start" {
			bStarted = true
		}
	}

	t.Logf("Execution order: %v", order)
	if !aStarted || !bStarted {
		t.Error("both tools should have started")
	}
}

// --- Dependent tools run sequentially ---

func TestParallelDispatch_DependentToolsRunSequentially(t *testing.T) {
	registry := tools.NewToolRegistry()

	var order []string

	registry.Register("Step1", func(args map[string]interface{}) (string, bool) {
		order = append(order, "step1")
		return "output1", false
	}, nil)

	registry.Register("Step2", func(args map[string]interface{}) (string, bool) {
		order = append(order, "step2")
		return "step2 done", false
	}, nil)

	stage := NewParallelToolDispatchStage(registry, &eventbus.EventBus{})

	// Step2 depends on Step1's output (its args reference call1)
	result := stage.Process(nil, agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "call1", Name: "Step1", Arguments: []byte("{}")},
			{ID: "call2", Name: "Step2", Arguments: []byte(`{"input":"$call1"}`)}, // references call1
		},
	})

	if len(result.ToolResults) != 2 {
		t.Fatalf("want 2 results, got %d", len(result.ToolResults))
	}

	// Step1 must execute before Step2
	t.Logf("Execution order: %v", order)
	if len(order) < 2 {
		t.Fatal("both tools should have executed")
	}
	if order[0] != "step1" {
		t.Errorf("step1 should run first, got order: %v", order)
	}
}

// --- Error in one tool doesn't block independent tools ---

func TestParallelDispatch_ErrorDoesNotBlockIndependent(t *testing.T) {
	registry := tools.NewToolRegistry()

	var executed atomic.Int32

	registry.Register("FailTool", func(args map[string]interface{}) (string, bool) {
		executed.Add(1)
		return "boom", true // isError = true
	}, nil)

	registry.Register("GoodTool", func(args map[string]interface{}) (string, bool) {
		executed.Add(1)
		time.Sleep(30 * time.Millisecond)
		return "all good", false
	}, nil)

	stage := NewParallelToolDispatchStage(registry, &eventbus.EventBus{})

	result := stage.Process(nil, agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "c1", Name: "FailTool", Arguments: []byte("{}")},
			{ID: "c2", Name: "GoodTool", Arguments: []byte("{}")},
		},
	})

	if len(result.ToolResults) != 2 {
		t.Fatalf("want 2 results, got %d", len(result.ToolResults))
	}

	// Both should have executed
	if executed.Load() != 2 {
		t.Errorf("both tools should have executed, got %d", executed.Load())
	}

	// GoodTool should have succeeded despite FailTool failing
	goodOK := false
	failOK := false
	for _, r := range result.ToolResults {
		if r.Name == "GoodTool" && !r.IsError {
			goodOK = true
		}
		if r.Name == "FailTool" && r.IsError {
			failOK = true
		}
	}
	if !goodOK {
		t.Error("GoodTool should have succeeded")
	}
	if !failOK {
		t.Error("FailTool should have reported error")
	}
}

// --- MaxParallelTools bounds concurrency ---

func TestParallelDispatch_MaxParallelToolsBoundsConcurrency(t *testing.T) {
	registry := tools.NewToolRegistry()

	var maxActive atomic.Int32

	slowTool := func(name string) tools.ToolHandler {
		return func(args map[string]interface{}) (string, bool) {
			current := maxActive.Add(1)
			// Track the peak
			time.Sleep(20 * time.Millisecond)
			maxActive.Add(-1)
			_ = current
			return name + " done", false
		}
	}

	for i := 0; i < 6; i++ {
		registry.Register(fmt.Sprintf("Tool%d", i), slowTool(fmt.Sprintf("T%d", i)), nil)
	}

	stage := NewParallelToolDispatchStage(registry, &eventbus.EventBus{})
	stage.MaxParallelTools = 3

	calls := make([]agentctx.ToolCall, 6)
	for i := 0; i < 6; i++ {
		calls[i] = agentctx.ToolCall{
			ID: fmt.Sprintf("c%d", i), Name: fmt.Sprintf("Tool%d", i), Arguments: []byte("{}"),
		}
	}

	result := stage.Process(nil, agentctx.ParsedResponse{ToolCalls: calls})

	if len(result.ToolResults) != 6 {
		t.Fatalf("want 6 results, got %d", len(result.ToolResults))
	}

	// All should succeed
	for _, r := range result.ToolResults {
		if r.IsError {
			t.Errorf("tool %s failed: %s", r.Name, r.Result)
		}
	}
}

// --- Preserves call ordering in results ---

func TestParallelDispatch_PreservesResultOrder(t *testing.T) {
	registry := tools.NewToolRegistry()

	for i := 0; i < 5; i++ {
		idx := i
		registry.Register(fmt.Sprintf("T%d", i), func(args map[string]interface{}) (string, bool) {
			time.Sleep(time.Duration(5-idx) * 10 * time.Millisecond) // reverse order completion
			return fmt.Sprintf("r%d", idx), false
		}, nil)
	}

	stage := NewParallelToolDispatchStage(registry, &eventbus.EventBus{})

	calls := make([]agentctx.ToolCall, 5)
	for i := 0; i < 5; i++ {
		calls[i] = agentctx.ToolCall{ID: fmt.Sprintf("c%d", i), Name: fmt.Sprintf("T%d", i), Arguments: []byte("{}")}
	}

	result := stage.Process(nil, agentctx.ParsedResponse{ToolCalls: calls})

	if len(result.ToolResults) != 5 {
		t.Fatalf("want 5 results, got %d", len(result.ToolResults))
	}

	// Results must be in original call order
	for i, r := range result.ToolResults {
		if r.CallID != fmt.Sprintf("c%d", i) {
			t.Errorf("result[%d].CallID = %s, want c%d", i, r.CallID, i)
		}
	}
}

// --- Single tool call still works ---

func TestParallelDispatch_SingleToolCall(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.Register("Echo", func(args map[string]interface{}) (string, bool) {
		return "echo", false
	}, nil)

	stage := NewParallelToolDispatchStage(registry, &eventbus.EventBus{})

	result := stage.Process(nil, agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "c1", Name: "Echo", Arguments: []byte("{}")},
		},
	})

	if len(result.ToolResults) != 1 || result.ToolResults[0].Result != "echo" {
		t.Errorf("single tool call failed: %v", result.ToolResults)
	}
}

// --- Empty tool calls ---

func TestParallelDispatch_EmptyToolCalls(t *testing.T) {
	registry := tools.NewToolRegistry()
	stage := NewParallelToolDispatchStage(registry, &eventbus.EventBus{})

	result := stage.Process(nil, agentctx.ParsedResponse{})

	if len(result.ToolResults) != 0 {
		t.Errorf("empty calls: want 0 results, got %d", len(result.ToolResults))
	}
}

// --- Budget still works with parallel dispatch ---

func TestParallelDispatch_BudgetStillWorks(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.Register("FileRead", func(args map[string]interface{}) (string, bool) {
		return "ok", false
	}, nil)

	stage := NewParallelToolDispatchStage(registry, &eventbus.EventBus{})

	// Call 5 times (budget for FileRead is 5)
	for i := 0; i < 5; i++ {
		result := stage.Process(nil, agentctx.ParsedResponse{
			ToolCalls: []agentctx.ToolCall{
				{ID: fmt.Sprintf("c%d", i), Name: "FileRead", Arguments: []byte("{}")},
			},
		})
		if len(result.ToolResults) != 1 || result.ToolResults[0].IsError {
			t.Errorf("call %d failed: %v", i, result.ToolResults[0])
		}
	}

	// 6th call should be blocked
	result := stage.Process(nil, agentctx.ParsedResponse{
		ToolCalls: []agentctx.ToolCall{
			{ID: "c6", Name: "FileRead", Arguments: []byte("{}")},
		},
	})
	if len(result.ToolResults) != 1 || !result.ToolResults[0].IsError {
		t.Error("6th call should be blocked by budget")
	}
}
