// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import (
	"encoding/json"
	"testing"
)

func mustJSON(s string) json.RawMessage {
	return json.RawMessage(s)
}

func TestComputeProgressSignatureDifferentiatesFileReadPaths(t *testing.T) {
	calls := []ToolCall{
		{Name: "FileRead", Arguments: mustJSON(`{"path": "/a/foo.go"}`)},
	}
	results := []ToolResult{
		{IsError: false},
	}
	sig := ComputeProgressSignature(calls, results, "")
	if sig.ToolNames != "FileRead:/a/foo.go" {
		t.Errorf("expected FileRead:/a/foo.go, got %s", sig.ToolNames)
	}

	// Different path should produce different signature
	calls2 := []ToolCall{
		{Name: "FileRead", Arguments: mustJSON(`{"path": "/a/bar.go"}`)},
	}
	sig2 := ComputeProgressSignature(calls2, results, "")
	if sig2.ToolNames != "FileRead:/a/bar.go" {
		t.Errorf("expected FileRead:/a/bar.go, got %s", sig2.ToolNames)
	}
	if sig.ToolNames == sig2.ToolNames {
		t.Error("different paths should produce different signatures")
	}
}

func TestComputeProgressSignatureDifferentiatesExecuteCommands(t *testing.T) {
	calls1 := []ToolCall{
		{Name: "Execute", Arguments: mustJSON(`{"command": "git log"}`)},
	}
	calls2 := []ToolCall{
		{Name: "Execute", Arguments: mustJSON(`{"command": "go test"}`)},
	}
	results := []ToolResult{{IsError: false}}

	sig1 := ComputeProgressSignature(calls1, results, "")
	sig2 := ComputeProgressSignature(calls2, results, "")

	if sig1.ToolNames != "Execute:git log" {
		t.Errorf("expected Execute:git log, got %s", sig1.ToolNames)
	}
	if sig2.ToolNames != "Execute:go test" {
		t.Errorf("expected Execute:go test, got %s", sig2.ToolNames)
	}
	if sig1.ToolNames == sig2.ToolNames {
		t.Error("different commands should produce different signatures")
	}
}

func TestComputeProgressSignatureDifferentiatesSearchQueries(t *testing.T) {
	calls1 := []ToolCall{
		{Name: "SearchMemories", Arguments: mustJSON(`{"query": "project"}`)},
	}
	calls2 := []ToolCall{
		{Name: "SearchMemories", Arguments: mustJSON(`{"query": "deployment"}`)},
	}
	results := []ToolResult{{IsError: false}}

	sig1 := ComputeProgressSignature(calls1, results, "")
	sig2 := ComputeProgressSignature(calls2, results, "")

	if sig1.ToolNames == sig2.ToolNames {
		t.Error("different search queries should produce different signatures")
	}
}

func TestComputeProgressSignatureMultiTool(t *testing.T) {
	calls := []ToolCall{
		{Name: "FileRead", Arguments: mustJSON(`{"path": "/a/main.go"}`)},
		{Name: "Execute", Arguments: mustJSON(`{"command": "go test ./..."}`)},
	}
	results := []ToolResult{
		{IsError: false},
		{IsError: true},
	}
	sig := ComputeProgressSignature(calls, results, "")

	if sig.ToolNames != "Execute:go test ./...,FileRead:/a/main.go" {
		t.Errorf("expected compound signature, got %s", sig.ToolNames)
	}
	if sig.ResultTag != "mixed" {
		t.Errorf("expected mixed, got %s", sig.ResultTag)
	}
}

func TestIsDuplicateProgressDifferentPathsNotDuplicate(t *testing.T) {
	sig1 := ProgressSignature{ToolNames: "FileRead:/a/foo.go", ResultTag: "success"}
	sig2 := ProgressSignature{ToolNames: "FileRead:/a/bar.go", ResultTag: "success"}
	sig3 := ProgressSignature{ToolNames: "FileRead:/a/baz.go", ResultTag: "success"}

	history := []ProgressSignature{sig1, sig2}
	// Current is sig3 (different path) — should NOT be duplicate
	if IsDuplicateProgress(sig3, history) {
		t.Error("different paths should not be detected as duplicate progress")
	}
}

func TestIsDuplicateProgressSamePathIsDuplicate(t *testing.T) {
	sig := ProgressSignature{ToolNames: "FileRead:/a/foo.go", ResultTag: "success"}
	history := []ProgressSignature{sig, sig}

	if !IsDuplicateProgress(sig, history) {
		t.Error("same path 3 times should be detected as duplicate progress")
	}
}

func TestIsDuplicateProgressEmptyHistory(t *testing.T) {
	sig := ProgressSignature{ToolNames: "FileRead:/a/foo.go", ResultTag: "success"}
	if IsDuplicateProgress(sig, nil) {
		t.Error("empty history should not be duplicate")
	}
}

func TestExtractKeyArg(t *testing.T) {
	arg := mustJSON(`{"path": "/some/long/path/to/file.go", "other": "ignored"}`)
	got := extractKeyArg(arg, "path")
	if got != "/some/long/path/to/file.go" {
		t.Errorf("extractKeyArg path = %q", got)
	}
	got = extractKeyArg(arg, "missing")
	if got != "" {
		t.Errorf("extractKeyArg missing = %q, want empty", got)
	}
}

func TestTruncateStr(t *testing.T) {
	short := "hello"
	if got := truncateStr(short, 10); got != "hello" {
		t.Errorf("truncateStr short = %q, want %q", got, "hello")
	}
	long := "abcdefghijklmnopqrstuvwxyz"
	if got := truncateStr(long, 5); got != "abcd…" {
		t.Errorf("truncateStr long = %q, want %q", got, "abcd…")
	}
}

func TestIsIntentLoop(t *testing.T) {
	// Test repeated intent
	history := []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "Execute", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "FileRead", ResultTag: "success", IntentTag: "search"},
	}
	if !IsIntentLoop(history, 3) {
		t.Error("repeated intent should be detected as loop")
	}

	// Test no loop
	history = []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "Execute", ResultTag: "success", IntentTag: "execute"},
		{ToolNames: "FileRead", ResultTag: "success", IntentTag: "read"},
	}
	if IsIntentLoop(history, 3) {
		t.Error("no loop should not be detected")
	}

	// Test below threshold
	history = []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "Execute", ResultTag: "success", IntentTag: "search"},
	}
	if IsIntentLoop(history, 3) {
		t.Error("below threshold should not be detected")
	}

	// Test cross-tool loop
	history = []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "FileRead", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "Execute", ResultTag: "success", IntentTag: "search"},
	}
	if !IsIntentLoop(history, 3) {
		t.Error("cross-tool loop should be detected")
	}

	// Test disabled threshold
	history = []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "Execute", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "FileRead", ResultTag: "success", IntentTag: "search"},
	}
	if IsIntentLoop(history, 0) {
		t.Error("disabled threshold should not detect loop")
	}
}

func TestIsOscillatingProgress(t *testing.T) {
	// Test too few history
	history := []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "Execute", ResultTag: "success"},
	}
	if IsOscillatingProgress(history) {
		t.Error("too few history should not be detected")
	}

	// Test no oscillation
	history = []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "Execute", ResultTag: "success"},
		{ToolNames: "FileRead", ResultTag: "success"},
	}
	if IsOscillatingProgress(history) {
		t.Error("no oscillation should not be detected")
	}

	// Test oscillating pattern
	history = []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "FileRead", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "FileRead", ResultTag: "success"},
		{ToolNames: "Execute", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
	}
	if !IsOscillatingProgress(history) {
		t.Error("oscillating pattern should be detected")
	}

	// Test same tool 3x
	history = []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
	}
	if !IsOscillatingProgress(history) {
		t.Error("same tool 3x should be detected")
	}

	// Test just under threshold
	history = []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
	}
	if IsOscillatingProgress(history) {
		t.Error("just under threshold should not be detected")
	}

	// Test multi-tool signature
	history = []ProgressSignature{
		{ToolNames: "FileSearch,FileRead", ResultTag: "success"},
		{ToolNames: "FileSearch,FileRead", ResultTag: "success"},
		{ToolNames: "FileSearch,FileRead", ResultTag: "success"},
		{ToolNames: "FileSearch,FileRead", ResultTag: "success"},
		{ToolNames: "FileSearch,FileRead", ResultTag: "success"},
	}
	if !IsOscillatingProgress(history) {
		t.Error("multi-tool signature should be detected")
	}
}

func TestComputeIntentTag(t *testing.T) {
	// Test from text
	if got := ComputeIntentTag("", "Let me search for the file"); got != "search" {
		t.Errorf("expected search from text, got %s", got)
	}

	// Test from tool name fallback
	if got := ComputeIntentTag("FileSearch", ""); got != "search" {
		t.Errorf("expected search from tool name, got %s", got)
	}

	// Test text overriding tool name
	if got := ComputeIntentTag("FileRead", "I need to execute this command"); got != "execute" {
		t.Errorf("expected execute from text, got %s", got)
	}
}

func TestIsDuplicateProgressEdgeCases(t *testing.T) {
	// Same tool same result IS duplicate
	sig := ProgressSignature{ToolNames: "FileRead:/a/foo.go", ResultTag: "success"}
	history := []ProgressSignature{
		{ToolNames: "FileRead:/a/bar.go", ResultTag: "success"},
		{ToolNames: "FileRead:/a/foo.go", ResultTag: "success"},
	}
	if !IsDuplicateProgress(sig, history) {
		t.Error("same tool same result should be duplicate")
	}

	// Different tool NOT duplicate
	sig = ProgressSignature{ToolNames: "Execute:go", ResultTag: "success"}
	history = []ProgressSignature{
		{ToolNames: "FileRead:/a/foo.go", ResultTag: "success"},
		{ToolNames: "FileRead:/a/bar.go", ResultTag: "success"},
	}
	if IsDuplicateProgress(sig, history) {
		t.Error("different tool should not be duplicate")
	}

	// Same tool different result NOT duplicate
	sig = ProgressSignature{ToolNames: "FileRead:/a/foo.go", ResultTag: "error"}
	history = []ProgressSignature{
		{ToolNames: "FileRead:/a/bar.go", ResultTag: "success"},
		{ToolNames: "FileRead:/a/foo.go", ResultTag: "success"},
	}
	if IsDuplicateProgress(sig, history) {
		t.Error("same tool different result should not be duplicate")
	}

	// Only checks recent history window
	sig = ProgressSignature{ToolNames: "FileRead:/a/foo.go", ResultTag: "success"}
	history = []ProgressSignature{
		{ToolNames: "FileRead:/a/foo.go", ResultTag: "success"},
		{ToolNames: "FileRead:/a/bar.go", ResultTag: "success"},
		{ToolNames: "FileRead:/a/baz.go", ResultTag: "success"},
		{ToolNames: "FileRead:/a/qux.go", ResultTag: "success"},
	}
	if IsDuplicateProgress(sig, history) {
		t.Error("only recent history should be checked")
	}
}