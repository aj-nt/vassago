// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import (
	"testing"
)

func TestComputeProgressSignature_SingleTool(t *testing.T) {
	calls := []ToolCall{{Name: "FileRead", Arguments: []byte(`{"path":"/tmp/test"}`)}}
	results := []ToolResult{{CallID: "1", Name: "FileRead", Result: "file contents", IsError: false}}

	sig := ComputeProgressSignature(calls, results, "")
	if sig.ToolNames != "FileRead" {
		t.Errorf("Expected ToolNames 'FileRead', got %q", sig.ToolNames)
	}
	if sig.ResultTag != "success" {
		t.Errorf("Expected ResultTag 'success', got %q", sig.ResultTag)
	}
	if sig.IntentTag != "read" {
		t.Errorf("Expected IntentTag 'read' (derived from tool name), got %q", sig.IntentTag)
	}
}

func TestComputeProgressSignature_IntentFromText(t *testing.T) {
	calls := []ToolCall{{Name: "FileSearch", Arguments: []byte(`{}`)}}
	results := []ToolResult{{CallID: "1", Name: "FileSearch", Result: "found 3 files", IsError: false}}

	// "Let me search for..." → intent "search"
	sig := ComputeProgressSignature(calls, results, "Let me search for the config file.")
	if sig.IntentTag != "search" {
		t.Errorf("Expected IntentTag 'search' from text, got %q", sig.IntentTag)
	}
}

func TestComputeProgressSignature_IntentTextOverridesToolName(t *testing.T) {
	calls := []ToolCall{{Name: "Execute", Arguments: []byte(`{}`)}}
	results := []ToolResult{{CallID: "1", Name: "Execute", Result: "output", IsError: false}}

	// Tool name would give "execute", but text says "Let me find..." → "search"
	sig := ComputeProgressSignature(calls, results, "Now let me find the logs.")
	if sig.IntentTag != "search" {
		t.Errorf("Expected IntentTag 'search' from text overriding tool name, got %q", sig.IntentTag)
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

	sig := ComputeProgressSignature(calls, results, "")
	// ToolNames should be sorted
	if sig.ToolNames != "FileRead,SearchMemories" {
		t.Errorf("Expected sorted ToolNames 'FileRead,SearchMemories', got %q", sig.ToolNames)
	}
	if sig.ResultTag != "success" {
		t.Errorf("Expected ResultTag 'success', got %q", sig.ResultTag)
	}
	// First recognized tool in sorted order is FileRead → intent "read"
	if sig.IntentTag != "read" {
		t.Errorf("Expected IntentTag 'read' from FileRead, got %q", sig.IntentTag)
	}
}

func TestComputeProgressSignature_ErrorResult(t *testing.T) {
	calls := []ToolCall{{Name: "Execute", Arguments: []byte(`{}`)}}
	results := []ToolResult{{CallID: "1", Name: "Execute", Result: "Error: command failed", IsError: true}}

	sig := ComputeProgressSignature(calls, results, "")
	if sig.ResultTag != "error" {
		t.Errorf("Expected ResultTag 'error', got %q", sig.ResultTag)
	}
}

func TestComputeProgressSignature_NotFoundResult(t *testing.T) {
	calls := []ToolCall{{Name: "SearchMemories", Arguments: []byte(`{}`)}}
	results := []ToolResult{{CallID: "1", Name: "SearchMemories", Result: "no results found", IsError: false}}

	sig := ComputeProgressSignature(calls, results, "")
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

	sig := ComputeProgressSignature(calls, results, "")
	if sig.ResultTag != "mixed" {
		t.Errorf("Expected ResultTag 'mixed', got %q", sig.ResultTag)
	}
}

func TestComputeProgressSignature_EmptyResults(t *testing.T) {
	calls := []ToolCall{{Name: "GetHotBlock", Arguments: []byte(`{}`)}}
	results := []ToolResult{}

	sig := ComputeProgressSignature(calls, results, "")
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

// --- Oscillation Detection Tests ---

func TestIsOscillatingProgress_TooFewHistory(t *testing.T) {
	// Need at least 4 history entries for oscillation detection
	history := []ProgressSignature{
		{ToolNames: "SearchMemories", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
	}
	if IsOscillatingProgress(history) {
		t.Error("Short history should not oscillate")
	}
}

func TestIsOscillatingProgress_NoOscillation(t *testing.T) {
	// Each tool appears once — legitimate multi-step work
	history := []ProgressSignature{
		{ToolNames: "FileRead", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "GetSession", ResultTag: "success"},
		{ToolNames: "Execute", ResultTag: "success"},
	}
	if IsOscillatingProgress(history) {
		t.Error("Different tools with no repetition should not oscillate")
	}
}

func TestIsOscillatingProgress_OscillatingPattern(t *testing.T) {
	// SearchMemories appears 4 times in 6 signatures — oscillation!
	history := []ProgressSignature{
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
		{ToolNames: "Execute", ResultTag: "success"},
	}
	if !IsOscillatingProgress(history) {
		t.Error("SearchMemories appears 3+ times — should detect oscillation")
	}
}

func TestIsOscillatingProgress_SameToolThreeTimes(t *testing.T) {
	// Same tool 3 times = oscillation
	history := []ProgressSignature{
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
		{ToolNames: "FileRead", ResultTag: "success"},
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
		{ToolNames: "Execute", ResultTag: "success"},
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
	}
	if !IsOscillatingProgress(history) {
		t.Error("SearchMemories appears 3 times — should detect oscillation")
	}
}

func TestIsOscillatingProgress_JustUnderThreshold(t *testing.T) {
	// Each tool appears only 2 times — not oscillation
	history := []ProgressSignature{
		{ToolNames: "SearchMemories", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "SearchMemories", ResultTag: "success"},
		{ToolNames: "FileSearch", ResultTag: "success"},
	}
	if IsOscillatingProgress(history) {
		t.Error("Each tool appears only 2 times — should not oscillate (need 3+)")
	}
}

func TestIsOscillatingProgress_MultiToolSignature(t *testing.T) {
	// Signatures with multiple tools (e.g., "FileRead,SearchMemories")
	// Each individual tool in the signature counts
	history := []ProgressSignature{
		{ToolNames: "FileRead,SearchMemories", ResultTag: "mixed"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "FileRead,SearchMemories", ResultTag: "mixed"},
		{ToolNames: "FileSearch", ResultTag: "success"},
		{ToolNames: "SearchMemories", ResultTag: "not_found"},
	}
	// SearchMemories appears 3 times (in 2 multi-tool sigs + 1 solo)
	if !IsOscillatingProgress(history) {
		t.Error("SearchMemories appears 3 times across signatures — should oscillate")
	}
}

// --- Intent Detection Tests ---

func TestComputeIntentTag_FromText(t *testing.T) {
	cases := []struct {
		text     string
		expected string
	}{
		{"Let me search for the config file.", "search"},
		{"I'll check the logs now.", "search"},
		{"Now let me read the file contents.", "read"},
		{"Let me examine the output.", "read"},
		{"I'll run the tests.", "execute"},
		{"Time to fix this bug.", "write"},
		{"Let me save this for later.", "write"},
		{"I remember seeing this before.", "memory"},    // "remember" matches before "see" in "seeing"
		{"", "search"}, // fallback to tool name: FileSearch → "search"
	}

	for _, tc := range cases {
		// Use FileSearch as the tool for empty text fallback
		result := ComputeIntentTag("FileSearch", tc.text)
		if result != tc.expected {
			t.Errorf("ComputeIntentTag(%q, %q) = %q, expected %q", "FileSearch", tc.text, result, tc.expected)
		}
	}
}

func TestComputeIntentTag_FallbackToToolName(t *testing.T) {
	cases := []struct {
		toolNames string
		expected  string
	}{
		{"FileSearch", "search"},
		{"SearchMemories", "search"},
		{"FileRead", "read"},
		{"GetHotBlock", "read"},
		{"Execute", "execute"},
		{"FileWrite", "write"},
		{"AddMemory", "memory"},
		{"UnknownTool", "other"},
	}

	for _, tc := range cases {
		result := ComputeIntentTag(tc.toolNames, "")
		if result != tc.expected {
			t.Errorf("ComputeIntentTag(%q, \"\") = %q, expected %q", tc.toolNames, result, tc.expected)
		}
	}
}

func TestComputeIntentTag_TextOverridesTool(t *testing.T) {
	// "Let me find..." → search intent, even though Execute tool would default to "execute"
	result := ComputeIntentTag("Execute", "Let me find the problem first.")
	if result != "search" {
		t.Errorf("Text should override tool name: got %q, expected 'search'", result)
	}
}

func TestIsIntentLoop_RepeatedIntent(t *testing.T) {
	// 3 search intents in 5 iterations → intent loop
	history := []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "FileRead", ResultTag: "success", IntentTag: "read"},
		{ToolNames: "SearchMemories", ResultTag: "not_found", IntentTag: "search"},
		{ToolNames: "Execute", ResultTag: "success", IntentTag: "execute"},
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
	}
	if !IsIntentLoop(history, 3) {
		t.Error("3 search intents should trigger intent loop")
	}
}

func TestIsIntentLoop_NoLoop(t *testing.T) {
	// All different intents — no loop
	history := []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "FileRead", ResultTag: "success", IntentTag: "read"},
		{ToolNames: "Execute", ResultTag: "success", IntentTag: "execute"},
		{ToolNames: "FileWrite", ResultTag: "success", IntentTag: "write"},
	}
	if IsIntentLoop(history, 3) {
		t.Error("All different intents should not be a loop")
	}
}

func TestIsIntentLoop_BelowThreshold(t *testing.T) {
	// Only 2 search intents — below threshold of 3
	history := []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "FileRead", ResultTag: "success", IntentTag: "read"},
		{ToolNames: "SearchMemories", ResultTag: "not_found", IntentTag: "search"},
	}
	if IsIntentLoop(history, 3) {
		t.Error("2 search intents is below threshold of 3")
	}
}

func TestIsIntentLoop_CrossToolLoop(t *testing.T) {
	// The key case: different tools, same intent ("search"), looping
	// This is what the original progress detection MISSED
	history := []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "SearchMemories", ResultTag: "not_found", IntentTag: "search"},
		{ToolNames: "FileRead", ResultTag: "success", IntentTag: "read"},
		{ToolNames: "Execute", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
	}
	// "search" appears 4 times — intent loop!
	if !IsIntentLoop(history, 3) {
		t.Error("4 'search' intents across different tools should be an intent loop")
	}
	// But the signatures themselves are NOT all duplicates
	// (FileSearch/success ≠ SearchMemories/not_found ≠ Execute/success)
	// This is exactly the case the old system missed.
}

func TestIsIntentLoop_Disabled(t *testing.T) {
	history := []ProgressSignature{
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "SearchMemories", ResultTag: "success", IntentTag: "search"},
		{ToolNames: "FileSearch", ResultTag: "success", IntentTag: "search"},
	}
	// threshold 0 disables intent loop detection
	if IsIntentLoop(history, 0) {
		t.Error("threshold 0 should disable intent loop detection")
	}
}
func TestExtractCommandVerb_SimpleCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		expected string
	}{
		{"simple git", `{"command": "git log --oneline"}`, "git"},
		{"simple ls", `{"command": "ls -la /tmp"}`, "ls"},
		{"cd command", `{"command": "cd /tmp && ls"}`, "cd"},
		{"chain with &&", `{"command": "git fetch && git rebase"}`, "git"},
		{"chain with ;", `{"command": "echo hello; echo world"}`, "echo"},
		{"chain with ||", `{"command": "cat file || echo missing"}`, "cat"},
		{"empty args", `{}`, ""},
		{"invalid json", `{bad`, ""},
		{"no command field", `{"path": "/tmp"}`, ""},
		{"just command no args", `{"command": "pwd"}`, "pwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCommandVerb([]byte(tt.args))
			if got != tt.expected {
				t.Errorf("extractCommandVerb(%q) = %q, want %q", tt.args, got, tt.expected)
			}
		})
	}
}

func TestComputeProgressSignature_ExecuteDifferentiation(t *testing.T) {
	// Two Execute calls with different commands should produce different signatures
	gitCall := ToolCall{Name: "Execute", Arguments: []byte(`{"command": "git log --oneline"}`)}
	lsCall := ToolCall{Name: "Execute", Arguments: []byte(`{"command": "ls -la /tmp"}`)}

	gitResult := ToolResult{CallID: "1", Name: "Execute", Result: "output", IsError: false}
	lsResult := ToolResult{CallID: "2", Name: "Execute", Result: "output", IsError: false}

	gitSig := ComputeProgressSignature([]ToolCall{gitCall}, []ToolResult{gitResult}, "")
	lsSig := ComputeProgressSignature([]ToolCall{lsCall}, []ToolResult{lsResult}, "")

	if gitSig.ToolNames == lsSig.ToolNames {
		t.Errorf("Different Execute commands should have different tool names, got same: %q", gitSig.ToolNames)
	}
	if gitSig.ToolNames != "Execute:git" {
		t.Errorf("Expected 'Execute:git', got %q", gitSig.ToolNames)
	}
	if lsSig.ToolNames != "Execute:ls" {
		t.Errorf("Expected 'Execute:ls', got %q", lsSig.ToolNames)
	}
}

func TestComputeProgressSignature_ExecuteFallback(t *testing.T) {
	// Execute call without a command field should fall back to just "Execute"
	call := ToolCall{Name: "Execute", Arguments: []byte(`{}`)}
	result := ToolResult{CallID: "1", Name: "Execute", Result: "output", IsError: false}
	sig := ComputeProgressSignature([]ToolCall{call}, []ToolResult{result}, "")
	if sig.ToolNames != "Execute" {
		t.Errorf("Expected 'Execute' fallback, got %q", sig.ToolNames)
	}
}

func TestComputeProgressSignature_FileReadNoCommand(t *testing.T) {
	// FileRead should NOT have command differentiation — just "FileRead"
	call := ToolCall{Name: "FileRead", Arguments: []byte(`{"path": "/tmp/test"}`)}
	result := ToolResult{CallID: "1", Name: "FileRead", Result: "contents", IsError: false}
	sig := ComputeProgressSignature([]ToolCall{call}, []ToolResult{result}, "")
	if sig.ToolNames != "FileRead" {
		t.Errorf("Expected 'FileRead', got %q", sig.ToolNames)
	}
}
