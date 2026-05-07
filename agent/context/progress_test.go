// This file is part of Mnemosyne.
// See LICENSE-Apache-2.0 for license information.

package context

import (
	"testing"
)

func TestComputeProgressSignature_SingleTool(t *testing.T) {
	calls := []ToolCall{{Name: "FileRead", Arguments: []byte(`{"path":"/tmp/test"}`)}}
	results := []ToolResult{{CallID: "1", Name: "FileRead", Result: "file contents", IsError: false}}

	sig := ComputeProgressSignature(calls, results)
	if sig.ToolNames != "FileRead" {
		t.Errorf("Expected ToolNames 'FileRead', got %q", sig.ToolNames)
	}
	if sig.ResultTag != "success" {
		t.Errorf("Expected ResultTag 'success', got %q", sig.ResultTag)
	}
}

func TestComputeProgressSignature_MultipleTools(t *testing.T) {
	calls := []ToolCall{
		{Name: "SearchMemories", Arguments: []byte(`{}`)},
		{Name: "FileRead", Arguments: []byte(`{}`)},
	}
	results := []ToolResult{
		{CallID: "1", Name: "SearchMemories", Result: "found 3 entries", IsError: false},
		{CallID: "2", Name: "FileRead", Result: "file contents", IsError: false},
	}

	sig := ComputeProgressSignature(calls, results)
	// ToolNames should be sorted
	if sig.ToolNames != "FileRead,SearchMemories" {
		t.Errorf("Expected sorted ToolNames 'FileRead,SearchMemories', got %q", sig.ToolNames)
	}
	if sig.ResultTag != "success" {
		t.Errorf("Expected ResultTag 'success', got %q", sig.ResultTag)
	}
}

func TestComputeProgressSignature_ErrorResult(t *testing.T) {
	calls := []ToolCall{{Name: "Execute", Arguments: []byte(`{}`)}}
	results := []ToolResult{{CallID: "1", Name: "Execute", Result: "Error: command failed", IsError: true}}

	sig := ComputeProgressSignature(calls, results)
	if sig.ResultTag != "error" {
		t.Errorf("Expected ResultTag 'error', got %q", sig.ResultTag)
	}
}

func TestComputeProgressSignature_NotFoundResult(t *testing.T) {
	calls := []ToolCall{{Name: "SearchMemories", Arguments: []byte(`{}`)}}
	results := []ToolResult{{CallID: "1", Name: "SearchMemories", Result: "no results found", IsError: false}}

	sig := ComputeProgressSignature(calls, results)
	if sig.ResultTag != "not_found" {
		t.Errorf("Expected ResultTag 'not_found', got %q", sig.ResultTag)
	}
}

func TestComputeProgressSignature_MixedResults(t *testing.T) {
	calls := []ToolCall{
		{Name: "FileRead", Arguments: []byte(`{}`)},
		{Name: "FileSearch", Arguments: []byte(`{}`)},
	}
	results := []ToolResult{
		{CallID: "1", Name: "FileRead", Result: "ok", IsError: false},
		{CallID: "2", Name: "FileSearch", Result: "Error: not found", IsError: true},
	}

	sig := ComputeProgressSignature(calls, results)
	if sig.ResultTag != "mixed" {
		t.Errorf("Expected ResultTag 'mixed', got %q", sig.ResultTag)
	}
}

func TestComputeProgressSignature_EmptyResults(t *testing.T) {
	calls := []ToolCall{{Name: "GetHotBlock", Arguments: []byte(`{}`)}}
	results := []ToolResult{}

	sig := ComputeProgressSignature(calls, results)
	if sig.ResultTag != "empty" {
		t.Errorf("Expected ResultTag 'empty', got %q", sig.ResultTag)
	}
}

func TestIsDuplicateProgress_NoHistory(t *testing.T) {
	sig := ProgressSignature{ToolNames: "FileRead", ResultTag: "success"}
	history := []ProgressSignature{}

	if IsDuplicateProgress(sig, history) {
		t.Error("Empty history should not be a duplicate")
	}
}

func TestIsDuplicateProgress_SameToolSameResult(t *testing.T) {
	sig := ProgressSignature{ToolNames: "SearchMemories", ResultTag: "not_found"}
	history := []ProgressSignature{
		{ToolNames: "FileRead", ResultTag: "success"},
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
	}

	if !IsDuplicateProgress(sig, history) {
		t.Error("Same tool+result in recent history should be a duplicate")
	}
}

func TestIsDuplicateProgress_DifferentTool(t *testing.T) {
	sig := ProgressSignature{ToolNames: "FileRead", ResultTag: "success"}
	history := []ProgressSignature{
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
		{ToolNames: "GetHotBlock", ResultTag: "success"},
	}

	if IsDuplicateProgress(sig, history) {
		t.Error("Different tool should not be a duplicate")
	}
}

func TestIsDuplicateProgress_SameToolDifferentResult(t *testing.T) {
	sig := ProgressSignature{ToolNames: "SearchMemories", ResultTag: "success"}
	history := []ProgressSignature{
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
	}

	if IsDuplicateProgress(sig, history) {
		t.Error("Same tool but different result should not be a duplicate")
	}
}

func TestIsDuplicateProgress_OnlyChecksRecentHistory(t *testing.T) {
	sig := ProgressSignature{ToolNames: "SearchMemories", ResultTag: "not_found"}
	// History has 5 entries, but only the last 3 are checked
	history := []ProgressSignature{
		{ToolNames: "FileRead", ResultTag: "success"},       // not in window
		{ToolNames: "GetHotBlock", ResultTag: "success"},    // edge of window
		{ToolNames: "SearchMemories", ResultTag: "success"}, // in window, different result
		{ToolNames: "Execute", ResultTag: "error"},          // in window, different tool
		{ToolNames: "SearchMemories", ResultTag: "not_found"}, // in window, MATCH
	}

	// The last entry matches our sig
	if !IsDuplicateProgress(sig, history) {
		t.Error("Should find duplicate in recent history")
	}
}

func TestProgressDetection_AllowsDifferentToolSequence(t *testing.T) {
	// Different tool sequence = PROGRESS (not a spiral)
	sig := ProgressSignature{ToolNames: "Execute", ResultTag: "success"}
	history := []ProgressSignature{
		{ToolNames: "FileRead", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "GetSession", ResultTag: "success"},
	}

	// Execute doesn't match any recent tool — this is novel progress
	if IsDuplicateProgress(sig, history) {
		t.Error("Different tool sequence should not be a duplicate")
	}
}