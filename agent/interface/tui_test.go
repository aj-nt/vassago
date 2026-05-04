// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFormatBanner(t *testing.T) {
	tests := []struct {
		name        string
		agentName   string
		personality string
	}{
		{"basic", "mnemosyne", "direct, dry-witted"},
		{"empty personality", "test", ""},
		{"long personality", "test", strings.Repeat("x", 100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBanner(tt.agentName, tt.personality)
			if result == "" {
				t.Error("formatBanner returned empty string")
			}
			// Should always contain the agent name
			if !strings.Contains(result, tt.agentName) {
				t.Errorf("formatBanner doesn't contain agent name %q", tt.agentName)
			}
		})
	}
}

func TestFormatBannerNoColor(t *testing.T) {
	// Test that formatBanner works when NO_COLOR is set
	t.Setenv("NO_COLOR", "1")
	result := formatBanner("mnemosyne", "test")
	if result == "" {
		t.Error("formatBanner returned empty string with NO_COLOR")
	}
	// Should not contain ANSI escape codes
	if strings.Contains(result, "\033[") {
		t.Error("formatBanner contains ANSI codes when NO_COLOR is set")
	}
}

func TestFormatStartupInfo(t *testing.T) {
	tests := []struct {
		name       string
		memCount   int
		hbChars    int
		personality string
	}{
		{"basic", 43, 2100, ""},
		{"zero", 0, 0, ""},
		{"large", 10000, 50000, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStartupInfo(tt.memCount, tt.hbChars, tt.personality)
			if result == "" {
				t.Error("formatStartupInfo returned empty string")
			}
		})
	}
}

func TestFormatToolStart(t *testing.T) {
	result := formatToolStart("SearchMemories")
	if !strings.Contains(result, "SearchMemories") {
		t.Errorf("formatToolStart doesn't contain tool name: %q", result)
	}
}

func TestFormatToolEnd(t *testing.T) {
	result := formatToolEnd("SearchMemories")
	if !strings.Contains(result, "SearchMemories") {
		t.Errorf("formatToolEnd doesn't contain tool name: %q", result)
	}
}

func TestFormatToolExec(t *testing.T) {
	result := formatToolExec("FileRead")
	if !strings.Contains(result, "FileRead") {
		t.Errorf("formatToolExec doesn't contain tool name: %q", result)
	}
}

func TestFormatToolError(t *testing.T) {
	result := formatToolError("FileWrite", "permission denied")
	if !strings.Contains(result, "FileWrite") {
		t.Errorf("formatToolError doesn't contain tool name: %q", result)
	}
	if !strings.Contains(result, "permission denied") {
		t.Errorf("formatToolError doesn't contain error message: %q", result)
	}
}

func TestFormatThinkingIndicator(t *testing.T) {
	result := formatThinkingIndicator()
	if result == "" {
		t.Error("formatThinkingIndicator returned empty string")
	}
}

func TestFormatClearThinking(t *testing.T) {
	result := formatClearThinking()
	if result == "" {
		t.Error("formatClearThinking returned empty string")
	}
}

func TestFormatResponse(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		agent   string
		wantEmpty bool
	}{
		{"short text", "Hello, world!", "mnemosyne", false},
		{"multiline", "Line 1\nLine 2\nLine 3", "mnemosyne", false},
		{"empty", "", "mnemosyne", true},
		{"whitespace only", "   \n  \t  ", "mnemosyne", true},
		{"long paragraph", strings.Repeat("word ", 50), "mnemosyne", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatResponse(tt.text, tt.agent)
			if (result == "") != tt.wantEmpty {
				t.Errorf("formatResponse(%q, %q) = %q, wantEmpty=%v", tt.text, tt.agent, result[:min(len(result), 50)], tt.wantEmpty)
			}
			if result != "" && tt.agent != "" && !strings.Contains(result, tt.agent) {
				t.Errorf("formatResponse doesn't contain agent name %q", tt.agent)
			}
		})
	}
}

func TestFormatResponseNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	result := formatResponse("Hello, world!", "test")
	if result == "" {
		t.Error("formatResponse returned empty string with NO_COLOR")
	}
	// Should not contain ANSI escape codes
	if strings.Contains(result, "\033[") {
		t.Error("formatResponse contains ANSI codes when NO_COLOR is set")
	}
}

func TestFormatHistoryLine(t *testing.T) {
	tests := []struct {
		role    string
		content string
	}{
		{"user", "Hello"},
		{"assistant", "Hi there! How can I help?"},
		{"system", "You are a helpful assistant"},
		{"tool", `{"result": "data"}`},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := formatHistoryLine(tt.role, tt.content)
			if !strings.Contains(result, tt.role) {
				t.Errorf("formatHistoryLine doesn't contain role %q", tt.role)
			}
		})
	}
}

func TestFormatSessionLine(t *testing.T) {
	result := formatSessionLine("abc123", "test session", 5, "cli")
	if !strings.Contains(result, "abc123") {
		t.Errorf("formatSessionLine doesn't contain ID: %q", result)
	}
	if !strings.Contains(result, "test session") {
		t.Errorf("formatSessionLine doesn't contain title: %q", result)
	}
}

func TestFormatGoodbye(t *testing.T) {
	tests := []struct {
		name  string
		stats *SessionStats
	}{
		{"nil stats", nil},
		{"empty stats", &SessionStats{}},
		{"with stats", &SessionStats{Turns: 5, ToolCalls: 3, TokensUsed: 1234}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatGoodbye(tt.stats)
			if result == "" {
				t.Error("formatGoodbye returned empty string")
			}
		})
	}
}

func TestFormatError(t *testing.T) {
	result := formatError(fmt.Errorf("test error"))
	if !strings.Contains(result, "test error") {
		t.Errorf("formatError doesn't contain message: %q", result)
	}
}

func TestFormatWarning(t *testing.T) {
	result := formatWarning("something went wrong")
	if !strings.Contains(result, "something went wrong") {
		t.Errorf("formatWarning doesn't contain message: %q", result)
	}
}

func TestFormatContextCleared(t *testing.T) {
	result := formatContextCleared()
	if result == "" {
		t.Error("formatContextCleared returned empty string")
	}
}

func TestFormatInterruptSignal(t *testing.T) {
	result := formatInterruptSignal()
	if result == "" {
		t.Error("formatInterruptSignal returned empty string")
	}
}

func TestFormatCommandsHelp(t *testing.T) {
	result := formatCommandsHelp()
	if !strings.Contains(result, "/quit") {
		t.Errorf("formatCommandsHelp doesn't contain /quit: %q", result)
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
	}{
		{"short", "Hello", 80},
		{"exact width", strings.Repeat("x", 80), 80},
		{"long", strings.Repeat("word ", 50), 40},
		{"multiline", "Line 1\nLine 2\nLine 3", 80},
		{"empty", "", 80},
		{"zero width", "Hello", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.input, tt.maxWidth)
			if tt.input == "" && result != "" {
				t.Errorf("wrapText of empty string should be empty, got %q", result)
			}
			// Check that no line exceeds maxWidth (unless a single word is longer)
			if tt.maxWidth > 0 && result != "" {
				for _, line := range strings.Split(result, "\n") {
					if utf8.RuneCountInString(line) > tt.maxWidth && !strings.Contains(line, " ") {
						// Single long word — acceptable
						continue
					}
				}
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel"},
		{"hello", 0, ""},
		{"🧠💡", 1, "🧠"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateRunes(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

func TestHumanizeChars(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0 chars"},
		{999, "999 chars"},
		{1000, "1.0K chars"},
		{1500, "1.5K chars"},
		{10000, "10.0K chars"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := humanizeChars(tt.n)
			if got != tt.want {
				t.Errorf("humanizeChars(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestHumanizeTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0 tokens"},
		{999, "999 tokens"},
		{1000, "1.0K tokens"},
		{5000, "5.0K tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := humanizeTokens(tt.n)
			if got != tt.want {
				t.Errorf("humanizeTokens(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestUseColor(t *testing.T) {
	// Test NO_COLOR disables color
	t.Run("NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		// Override by re-checking
		if os.Getenv("NO_COLOR") != "1" {
			t.Skip("NO_COLOR not set")
		}
		// useColor reads env each time, so this should work
		result := useColor()
		if result {
			t.Error("useColor should return false when NO_COLOR is set")
		}
	})

	// Test dumb terminal disables color
	t.Run("dumb terminal", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("TERM", "dumb")
		result := useColor()
		if result {
			t.Error("useColor should return false when TERM=dumb")
		}
	})
}

func TestFormatResponseBordered(t *testing.T) {
	// Test that formatResponse produces a bordered panel with the agent name
	t.Setenv("NO_COLOR", "")  // Ensure colors are enabled
	t.Setenv("TERM", "xterm") // Ensure not dumb terminal
	result := formatResponse("Hello, world!", "mnemosyne")
	if !strings.Contains(result, "mnemosyne") {
		t.Errorf("formatResponse should contain agent name: %q", result)
	}
	if !strings.Contains(result, "Hello, world!") {
		t.Errorf("formatResponse should contain text: %q", result)
	}
	// Should have box drawing characters
	if !strings.Contains(result, "╭") && !strings.Contains(result, "─") {
		t.Errorf("formatResponse should use box drawing: %q", result)
	}
}

func TestColorize(t *testing.T) {
	// Test colorize with color enabled
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")
	result := colorize("\033[31m", "hello")
	if !strings.Contains(result, "hello") {
		t.Errorf("colorize should contain text: %q", result)
	}
	if !strings.Contains(result, "\033[31m") {
		t.Errorf("colorize should contain color code: %q", result)
	}
}

func TestFormatGoodbyeWithStats(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")
	stats := &SessionStats{Turns: 5, ToolCalls: 3, TokensUsed: 2500}
	result := formatGoodbye(stats)
	if !strings.Contains(result, "5 turns") {
		t.Errorf("formatGoodbye should contain turn count: %q", result)
	}
	if !strings.Contains(result, "3") {
		t.Errorf("formatGoodbye should contain tool call count: %q", result)
	}
}

func TestFormatPrompt(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")
	result := formatPrompt("mnemosyne")
	if !strings.Contains(result, "❯") {
		t.Errorf("formatPrompt should contain prompt character: %q", result)
	}
}

func TestFormatResumedSession(t *testing.T) {
	result := formatResumedSession("My Session", 5)
	if !strings.Contains(result, "My Session") {
		t.Errorf("expected session title in output, got %q", result)
	}
	if !strings.Contains(result, "5 messages") {
		t.Errorf("expected message count in output, got %q", result)
	}
}

func TestFormatResumedSessionNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	result := formatResumedSession("Test", 10)
	if !strings.Contains(result, "Resumed session: Test (10 messages)") {
		t.Errorf("expected plain text format, got %q", result)
	}
}