// This file is part of Vassago.
// See LICENSE-Apache-2.0 for license information.

package format

import (
	"strings"
	"testing"
)

func TestGetPlatformLimits(t *testing.T) {
	tests := []struct {
		platform       string
		expectMaxLen   int
		expectMarkdown bool
		expectHTML     bool
	}{
		{"discord", 2000, true, false},
		{"telegram", 4096, false, true},
		{"imessage", 4000, false, false},
		{"email", 50000, false, false},
		{"unknown", 2000, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			limits := GetPlatformLimits(tt.platform)
			if limits.MaxMessageLen != tt.expectMaxLen {
				t.Errorf("MaxMessageLen = %d, want %d", limits.MaxMessageLen, tt.expectMaxLen)
			}
			if limits.SupportsMarkdown != tt.expectMarkdown {
				t.Errorf("SupportsMarkdown = %v, want %v", limits.SupportsMarkdown, tt.expectMarkdown)
			}
			if limits.SupportsHTML != tt.expectHTML {
				t.Errorf("SupportsHTML = %v, want %v", limits.SupportsHTML, tt.expectHTML)
			}
		})
	}
}

func TestFormatMessageForPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		message  string
		want     string
	}{
		{
			name:     "short discord message",
			platform: "discord",
			message:  "Hello!",
			want:     "Hello!",
		},
		{
			name:     "short telegram message",
			platform: "telegram",
			message:  "Hello!",
			want:     "Hello!",
		},
		{
			name:     "short imessage message",
			platform: "imessage",
			message:  "Hello!",
			want:     "Hello!",
		},
		{
			name:     "short email message",
			platform: "email",
			message:  "Hello!",
			want:     "Hello!",
		},
		{
			name:     "email with subject line",
			platform: "email",
			message:  "Subject: Build status\nThe build passed.",
			want:     "Subject: Build status\nThe build passed.",
		},
		{
			name:     "unknown platform uses default",
			platform: "unknown",
			message:  "Hello!",
			want:     "Hello!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMessageForPlatform(tt.platform, tt.message)
			if got != tt.want {
				t.Errorf("FormatMessageForPlatform() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatMessageForPlatform_Truncation(t *testing.T) {
	longMsg := strings.Repeat("a", 3000)

	// Discord limit is 2000
	result := FormatMessageForPlatform("discord", longMsg)
	if len(result) > 2020 {
		t.Errorf("discord message too long: %d chars", len(result))
	}
	if !strings.HasSuffix(result, "\n... (truncated)") {
		t.Errorf("discord message should be truncated, got: %q", result[len(result)-30:])
	}

	// Telegram limit is 4096 — should pass through
	result = FormatMessageForPlatform("telegram", longMsg)
	if result != longMsg {
		t.Errorf("telegram should not truncate a 3000-char message")
	}

	// Email limit is 50000 — should pass through
	result = FormatMessageForPlatform("email", longMsg)
	if result != longMsg {
		t.Errorf("email should not truncate a 3000-char message")
	}
}

func TestSplitMessageForPlatform(t *testing.T) {
	shortMsg := "Hello, world!"
	chunks := SplitMessageForPlatform("discord", shortMsg)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for short message, got %d", len(chunks))
	}

	longMsg := strings.Repeat("a", 5000)
	chunks = SplitMessageForPlatform("discord", longMsg)
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for 5000-char discord message, got %d", len(chunks))
	}
	for i, chunk := range chunks {
		// First chunk has no prefix; subsequent chunks have "(continued) " prefix
		expectedMax := 2000 + 15 // max len + continuation prefix len
		if len(chunk) > expectedMax {
			t.Errorf("chunk %d too long: %d chars", i, len(chunk))
		}
	}

	if len(chunks) > 1 {
		if !strings.HasPrefix(chunks[1], "(continued) ") {
			t.Errorf("continuation chunk should have prefix, got: %q", chunks[1][:30])
		}
	}

	// Telegram allows 4096
	chunks = SplitMessageForPlatform("telegram", longMsg)
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for 5000-char telegram message, got %d", len(chunks))
	}
}

func TestValidatePlatform(t *testing.T) {
	validPlatforms := []string{"discord", "telegram", "imessage", "email"}
	for _, p := range validPlatforms {
		if err := ValidatePlatform(p); err != nil {
			t.Errorf("ValidatePlatform(%q) should not error, got: %v", p, err)
		}
	}

	invalidPlatforms := []string{"slack", "whatsapp", "", "Discord", "TELEGRAM"}
	for _, p := range invalidPlatforms {
		if err := ValidatePlatform(p); err == nil {
			t.Errorf("ValidatePlatform(%q) should error", p)
		}
	}
}

func TestValidateEmailTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"valid email", "user@example.com", false},
		{"valid with dots", "first.last@example.com", false},
		{"invalid no @", "notanemail", true},
		{"invalid empty", "", true},
		{"valid with plus", "user+tag@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateEmailTarget(tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmailTarget(%q) error = %v, wantErr %v", tt.target, err, tt.wantErr)
			}
		})
	}
}

func TestComposePlatformContext(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		channelName string
		senderName  string
		contains    []string
	}{
		{
			name:        "discord with sender and channel",
			platform:    "discord",
			channelName: "general",
			senderName:  "Alice",
			contains:    []string{"Alice", "Discord", "general"},
		},
		{
			name:        "telegram with sender and channel",
			platform:    "telegram",
			channelName: "dev-chat",
			senderName:  "Bob",
			contains:    []string{"Bob", "Telegram", "dev-chat"},
		},
		{
			name:        "imessage with sender only",
			platform:    "imessage",
			channelName: "",
			senderName:  "Charlie",
			contains:    []string{"Charlie", "iMessage"},
		},
		{
			name:        "email with channel only",
			platform:    "email",
			channelName: "INBOX",
			senderName:  "",
			contains:    []string{"Email", "INBOX"},
		},
		{
			name:        "unknown platform",
			platform:    "slack",
			channelName: "",
			senderName:  "",
			contains:    []string{"slack"},
		},
		{
			name:        "discord no sender no channel",
			platform:    "discord",
			channelName: "",
			senderName:  "",
			contains:    []string{"Discord"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComposePlatformContext(tt.platform, tt.channelName, tt.senderName)
			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("ComposePlatformContext() = %q, want to contain %q", result, substr)
				}
			}
		})
	}
}

func TestSupportedPlatforms(t *testing.T) {
	platforms := SupportedPlatforms()
	if len(platforms) != 4 {
		t.Errorf("expected 4 supported platforms, got %d", len(platforms))
	}

	expected := map[string]bool{"discord": true, "telegram": true, "imessage": true, "email": true}
	for _, p := range platforms {
		if !expected[p] {
			t.Errorf("unexpected platform: %s", p)
		}
	}
}

// --- Red Team Tests ---

func TestRedTeam_FormatMessageForPlatform_EmptyInput(t *testing.T) {
	result := FormatMessageForPlatform("discord", "")
	if result != "" {
		t.Errorf("empty message should stay empty, got %q", result)
	}

	result = FormatMessageForPlatform("discord", "   ")
	if result != "   " {
		t.Errorf("whitespace message should pass through, got %q", result)
	}
}

func TestRedTeam_FormatMessageForPlatform_Unicode(t *testing.T) {
	unicodeMsg := strings.Repeat("🌍", 1000)
	result := FormatMessageForPlatform("discord", unicodeMsg)
	if !strings.HasSuffix(result, "\n... (truncated)") {
		t.Errorf("long unicode message should be truncated, got len=%d", len(result))
	}
}

func TestRedTeam_FormatMessageForPlatform_EmailSubjectEdgeCases(t *testing.T) {
	// Subject with no body
	result := FormatMessageForPlatform("email", "Subject: Hello")
	if result != "Subject: Hello" {
		t.Errorf("subject-only email should pass through, got %q", result)
	}

	// "Subject: " not at start — should not be treated as subject line
	result = FormatMessageForPlatform("email", "This is Subject: test")
	if result != "This is Subject: test" {
		t.Errorf("subject in middle should not be special, got %q", result)
	}

	// Case-sensitive: lowercase "subject:" should not match
	result = FormatMessageForPlatform("email", "subject: lowercase")
	if strings.HasPrefix(result, "Subject: ") {
		t.Errorf("lowercase 'subject:' should not match subject prefix")
	}
}

func TestRedTeam_SplitMessageForPlatform_BoundaryConditions(t *testing.T) {
	limits := GetPlatformLimits("discord")

	// Exactly at limit — should not split
	msg := strings.Repeat("a", limits.MaxMessageLen)
	chunks := SplitMessageForPlatform("discord", msg)
	if len(chunks) != 1 {
		t.Errorf("message at exact limit should be 1 chunk, got %d", len(chunks))
	}

	// One over limit — should split
	msg = strings.Repeat("a", limits.MaxMessageLen+1)
	chunks = SplitMessageForPlatform("discord", msg)
	if len(chunks) < 2 {
		t.Errorf("message one over limit should split into 2+ chunks, got %d", len(chunks))
	}
}

func TestRedTeam_SplitMessageForPlatform_ContinuationPrefix(t *testing.T) {
	longMsg := strings.Repeat("a", 3000)
	chunks := SplitMessageForPlatform("discord", longMsg)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	if strings.HasPrefix(chunks[0], "(continued) ") {
		t.Errorf("first chunk should not have continuation prefix")
	}

	if !strings.HasPrefix(chunks[1], "(continued) ") {
		t.Errorf("second chunk should have continuation prefix, got: %q", chunks[1][:30])
	}
}

func TestRedTeam_ValidatePlatform_EdgeCases(t *testing.T) {
	edgeCases := []struct {
		name     string
		platform string
		wantErr  bool
	}{
		{"sql injection", "discord'; DROP TABLE users;--", true},
		{"path traversal", "../../../etc/passwd", true},
		{"null bytes", "discord\x00telegram", true},
		{"spaces", "discord telegram", true},
		{"mixed case", "Discord", true},
		{"numeric", "123", true},
	}

	for _, tt := range edgeCases {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlatform(tt.platform)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlatform(%q) error = %v, wantErr %v", tt.platform, err, tt.wantErr)
			}
		})
	}
}

func TestRedTeam_ValidateEmailTarget_EdgeCases(t *testing.T) {
	edgeCases := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"sql injection", "user'; DROP TABLE users;--@example.com", true},
		{"path traversal", "../../../etc/passwd", true},
		{"null bytes", "user\x00@example.com", true},
		{"spaces in local part", "user name@example.com", true},
		{"double @", "user@@example.com", true},
		{"just @", "@", true},
		{"valid subdomain", "user@sub.example.com", false},
	}

	for _, tt := range edgeCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateEmailTarget(tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmailTarget(%q) error = %v, wantErr %v", tt.target, err, tt.wantErr)
			}
		})
	}
}

func TestRedTeam_ComposePlatformContext_InjectionAttempts(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		channelName string
		senderName  string
	}{
		{"platform injection", "discord</script>", "general", "Alice"},
		{"channel name injection", "discord", "general<script>alert(1)</script>", "Alice"},
		{"sender name injection", "telegram", "chat", "Bob\x00Malicious"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComposePlatformContext(tt.platform, tt.channelName, tt.senderName)
			if result == "" {
				t.Error("ComposePlatformContext should not return empty string")
			}
		})
	}
}

func TestRedTeam_FormatMessageForPlatform_VeryLongEmail(t *testing.T) {
	longMsg := strings.Repeat("x", 50001)
	result := FormatMessageForPlatform("email", longMsg)
	if !strings.HasSuffix(result, "\n... (truncated)") {
		t.Errorf("very long email should be truncated, got len=%d", len(result))
	}
}

func TestRedTeam_SplitMessageForPlatform_EmptyMessage(t *testing.T) {
	chunks := SplitMessageForPlatform("discord", "")
	if len(chunks) != 1 {
		t.Errorf("empty message should produce 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "" {
		t.Errorf("empty message chunk should be empty, got %q", chunks[0])
	}
}

func TestRedTeam_GetPlatformLimits_DefaultFallback(t *testing.T) {
	limits := GetPlatformLimits("unknown_platform")
	if limits.MaxMessageLen != 2000 {
		t.Errorf("unknown platform should default to 2000, got %d", limits.MaxMessageLen)
	}
	if limits.SplitPrefix != "(continued) " {
		t.Errorf("unknown platform should have default split prefix, got %q", limits.SplitPrefix)
	}
}
