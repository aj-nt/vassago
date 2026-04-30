package iface

import (
	"testing"
)

// --- Approval flow tests ---

func TestFormatApprovalRequest(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		want     []string // strings that must appear in output
	}{
		{
			name:     "FileWrite approval",
			toolName: "FileWrite",
			args:     map[string]interface{}{"path": "/tmp/test.txt", "content": "hello"},
			want:     []string{"FileWrite", "/tmp/test.txt", "approve"},
		},
		{
			name:     "Execute approval",
			toolName: "Execute",
			args:     map[string]interface{}{"command": "rm foo.txt"},
			want:     []string{"Execute", "rm foo.txt", "approve"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatApprovalRequest(tt.toolName, tt.args)
			for _, w := range tt.want {
				if !contains(got, w) {
					t.Errorf("formatApprovalRequest(%q, %v) = %q, want to contain %q", tt.toolName, tt.args, got, w)
				}
			}
		})
	}
}

func TestFormatApprovalRequestNilArgs(t *testing.T) {
	got := formatApprovalRequest("Execute", nil)
	if got == "" {
		t.Error("expected non-empty output for nil args")
	}
}

// --- Context usage tests ---

func TestFormatContextUsage(t *testing.T) {
	tests := []struct {
		used       int
		maximum    int
		wantSubstr string
	}{
		{0, 128000, "0/128K"},
		{42000, 128000, "42K/128K"},
		{128000, 128000, "128K/128K"},
		{5000, 128000, "5K/128K"},
	}

	for _, tt := range tests {
		got := formatContextUsage(tt.used, tt.maximum)
		if !contains(got, tt.wantSubstr) {
			t.Errorf("formatContextUsage(%d, %d) = %q, want to contain %q", tt.used, tt.maximum, got, tt.wantSubstr)
		}
	}
}

func TestFormatContextUsageZero(t *testing.T) {
	// When MaxContextTokens is 0 (disabled), show nothing
	got := formatContextUsage(0, 0)
	if got != "" {
		t.Errorf("expected empty string for disabled context, got %q", got)
	}
}

// --- Resume command parsing tests ---

func TestParseResumeArgs(t *testing.T) {
	tests := []struct {
		input string
		id    string
	}{
		{"/resume abc123", "abc123"},
		{"/resume 20260501_deadbeef", "20260501_deadbeef"},
		{"/resume", ""}, // no ID — should fail
	}

	for _, tt := range tests {
		id, ok := parseResumeCommand(tt.input)
		if tt.id == "" {
			if ok {
				t.Errorf("parseResumeCommand(%q) should fail", tt.input)
			}
		} else {
			if !ok {
				t.Errorf("parseResumeCommand(%q) should succeed", tt.input)
			}
			if id != tt.id {
				t.Errorf("parseResumeCommand(%q) = %q, want %q", tt.input, id, tt.id)
			}
		}
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}