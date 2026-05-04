// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import "testing"

func TestAutoTitle_ShortMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short message",
			input:   "What is 2+2?",
			expected: "What is 2+2?",
		},
		{
			name:     "long message truncated at word boundary",
			input:   "This is a really long message that should be truncated because it exceeds the maximum title length of sixty characters",
			expected: "This is a really long message that should be truncated…",
		},
		{
			name:     "message with newlines",
			input:   "Hello\nthis is a\nmulti-line message",
			expected: "Hello this is a multi-line message",
		},
		{
			name:     "message with extra spaces",
			input:   "Hello   world   with    spaces",
			expected: "Hello world with spaces",
		},
		{
			name:     "exactly 60 chars",
			input:   "What is my favorite color and what projects am I working on?",
			expected: "What is my favorite color and what projects am I working on?",
		},
		{
			name:     "empty message",
			input:   "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:   "   \n\t  ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := autoTitle(tt.input)
			if result != tt.expected {
				t.Errorf("autoTitle(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
			// Verify max length
			if len(result) > 62 { // max 60 + "…"
				t.Errorf("autoTitle result too long: %d chars: %q", len(result), result)
			}
		})
	}
}
