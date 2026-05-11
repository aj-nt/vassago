// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agentlog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

// --- Pipeline Iteration Span ---

func TestPipelineIterationSpan(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	PipelineIteration(PipelineIterationArgs{
		Iteration: 3,
		Model:     "gpt-4o",
		Provider:  "openai",
		ToolCount: 2,
		Action:    "CONTINUE",
	})

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}
	if entry["msg"] != "pipeline.iteration" {
		t.Errorf("msg = %q, want pipeline.iteration", entry["msg"])
	}
	if entry["iteration"] != float64(3) {
		t.Errorf("iteration = %v", entry["iteration"])
	}
	if entry["model"] != "gpt-4o" {
		t.Errorf("model = %q", entry["model"])
	}
	if entry["provider"] != "openai" {
		t.Errorf("provider = %q", entry["provider"])
	}
	if entry["tool_count"] != float64(2) {
		t.Errorf("tool_count = %v", entry["tool_count"])
	}
	if entry["action"] != "CONTINUE" {
		t.Errorf("action = %q", entry["action"])
	}
}

func TestPipelineIterationSpanOptionalFields(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	// No tool calls, YIELD action — tool_count should be 0
	PipelineIteration(PipelineIterationArgs{
		Iteration: 1,
		Model:     "claude-sonnet-4",
		Provider:  "anthropic",
		ToolCount: 0,
		Action:    "YIELD",
	})

	var entry map[string]interface{}
	json.Unmarshal(buf.Bytes(), &entry)

	if entry["tool_count"] != float64(0) {
		t.Errorf("tool_count = %v, want 0", entry["tool_count"])
	}
}

// --- Tool Dispatch Span ---

func TestToolDispatchSpan(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	dur := 42 * time.Millisecond
	ToolDispatch(ToolDispatchArgs{
		ToolName: "FileRead",
		Duration: dur,
		IsError:  false,
	})

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}
	if entry["msg"] != "tool.dispatch" {
		t.Errorf("msg = %q, want tool.dispatch", entry["msg"])
	}
	if entry["tool"] != "FileRead" {
		t.Errorf("tool = %q", entry["tool"])
	}
	// Duration should be in milliseconds
	if ms, ok := entry["duration_ms"].(float64); !ok || ms < 40 || ms > 50 {
		t.Errorf("duration_ms = %v, want ~42", entry["duration_ms"])
	}
	if entry["error"] != false {
		t.Errorf("error = %v, want false for successful dispatch", entry["error"])
	}
}

func TestToolDispatchSpanError(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	ToolDispatch(ToolDispatchArgs{
		ToolName: "Execute",
		Duration: 100 * time.Millisecond,
		IsError:  true,
	})

	var entry map[string]interface{}
	json.Unmarshal(buf.Bytes(), &entry)

	if entry["error"] != true {
		t.Errorf("error = %v, want true", entry["error"])
	}
}

// --- Router Select Span ---

func TestRouterSelectSpan(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	RouterSelect(RouterSelectArgs{
		Strategy:       "cost-optimized",
		SelectedModel:  "gpt-4o-mini",
		SelectedReason: "simple query, low complexity",
		Iteration:      5,
	})

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}
	if entry["msg"] != "router.select" {
		t.Errorf("msg = %q, want router.select", entry["msg"])
	}
	if entry["strategy"] != "cost-optimized" {
		t.Errorf("strategy = %q", entry["strategy"])
	}
	if entry["selected_model"] != "gpt-4o-mini" {
		t.Errorf("selected_model = %q", entry["selected_model"])
	}
	if entry["select_reason"] != "simple query, low complexity" {
		t.Errorf("select_reason = %q", entry["select_reason"])
	}
	if entry["iteration"] != float64(5) {
		t.Errorf("iteration = %v", entry["iteration"])
	}
}

func TestRouterSelectSpanPrimary(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	RouterSelect(RouterSelectArgs{
		Strategy:       "primary",
		SelectedModel:  "gpt-4o",
		SelectedReason: "no router configured",
	})

	var entry map[string]interface{}
	json.Unmarshal(buf.Bytes(), &entry)

	if entry["selected_model"] != "gpt-4o" {
		t.Errorf("selected_model = %q", entry["selected_model"])
	}
}

func TestRouterSelectSpanFallback(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	RouterSelect(RouterSelectArgs{
		Strategy:       "fallback",
		SelectedModel:  "claude-sonnet-4",
		SelectedReason: "primary openai failed: timeout",
	})

	var entry map[string]interface{}
	json.Unmarshal(buf.Bytes(), &entry)

	if entry["strategy"] != "fallback" {
		t.Errorf("strategy = %q", entry["strategy"])
	}
}

// verify that existing buffer-based testLogger works for span tests too
func TestSpansGoThroughSameLogger(t *testing.T) {
	buf := new(bytes.Buffer)
	l := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	Init(l)

	PipelineIteration(PipelineIterationArgs{Iteration: 7, Model: "test", Provider: "test", Action: "CONTINUE"})
	ToolDispatch(ToolDispatchArgs{ToolName: "Echo", Duration: time.Millisecond, IsError: false})
	RouterSelect(RouterSelectArgs{Strategy: "primary", SelectedModel: "test", SelectedReason: "test"})

	// Should have 3 JSON lines
	lines := bytes.Count(buf.Bytes(), []byte("\n"))
	if lines < 3 {
		t.Errorf("expected at least 3 log lines, got %d: %s", lines, buf.String())
	}
}
