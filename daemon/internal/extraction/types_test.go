// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package extraction

import (
	"strings"
	"testing"
)

func TestExtractionConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DebounceInterval != 30 {
		t.Errorf("DebounceInterval = %d, want 30", cfg.DebounceInterval)
	}
	if cfg.HardCapInterval != 900 {
		t.Errorf("HardCapInterval = %d, want 900 (15 min)", cfg.HardCapInterval)
	}
	if cfg.MaxMessagesPerExtraction != 50 {
		t.Errorf("MaxMessagesPerExtraction = %d, want 50", cfg.MaxMessagesPerExtraction)
	}
	if cfg.MinMessagesForExtraction != 4 {
		t.Errorf("MinMessagesForExtraction = %d, want 4", cfg.MinMessagesForExtraction)
	}
	if cfg.Priority != 2 {
		t.Errorf("Priority = %d, want 2", cfg.Priority)
	}
}

func TestExtractionConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid with model", Config{Model: "test-model"}, false},
		{"defaults require model", DefaultConfig(), true},
		{"model required", Config{Model: ""}, true},
		{"debounce too low", Config{Model: "test", DebounceInterval: 4}, true},
		{"debounce too high", Config{Model: "test", DebounceInterval: 601}, true},
		{"zero debounce disables", Config{Model: "test", DebounceInterval: 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestCandidate_Valid(t *testing.T) {
	tests := []struct {
		name    string
		c       Candidate
		wantErr bool
	}{
		{"valid", Candidate{Key: "test_key", Content: "some content", Category: "quirk", Target: "memory", Priority: 2}, false},
		{"missing key", Candidate{Content: "x", Category: "quirk", Target: "memory", Priority: 2}, true},
		{"missing content", Candidate{Key: "k", Category: "quirk", Target: "memory", Priority: 2}, true},
		{"missing category", Candidate{Key: "k", Content: "x", Target: "memory", Priority: 2}, true},
		{"invalid target", Candidate{Key: "k", Content: "x", Category: "quirk", Target: "bogus", Priority: 2}, true},
		{"invalid priority low", Candidate{Key: "k", Content: "x", Category: "quirk", Target: "memory", Priority: 0}, true},
		{"invalid priority high", Candidate{Key: "k", Content: "x", Category: "quirk", Target: "memory", Priority: 6}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestContext_MessageFormat verifies messages are formatted correctly for the LLM prompt.
// This is a pure function test — no LLM calls involved.
func TestFormatMessagesForExtraction(t *testing.T) {
	msgs := []SessionMessage{
		{Role: "user", Content: "I prefer short commit messages"},
		{Role: "assistant", Content: "Got it. Concise commits.", ToolCalls: []ToolCall{{Name: "terminal", Args: "git commit -m 'fix bug'"}}},
		{Role: "tool", Content: "[commit abc123]"},
		{Role: "assistant", Content: "Done."},
	}

	result := formatMessagesForExtraction(msgs)

	if len(result) == 0 {
		t.Fatal("expected non-empty formatted result")
	}
	// Must include the user message content
	if !contains(result, "short commit messages") {
		t.Error("formatted result should contain user message content")
	}
	// Should include tool call info
	if !contains(result, "terminal") {
		t.Error("formatted result should contain tool call information")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && strings.Contains(s, substr)
}
