// This file is part of Vassago.
// See LICENSE-Apache-2.0 for license information.

// Package format provides platform-aware message formatting constants and
// utilities shared between the agent and daemon. It has no dependencies on
// either package, keeping the import graph clean.
package format

import (
	"fmt"
	"net/mail"
	"strings"
)

// PlatformLimits defines character limits and formatting constraints per platform.
type PlatformLimits struct {
	// MaxMessageLen is the maximum length of a single message.
	MaxMessageLen int
	// SupportsMarkdown is true if the platform renders Markdown.
	SupportsMarkdown bool
	// SupportsHTML is true if the platform renders HTML.
	SupportsHTML bool
	// SplitPrefix is prepended to continuation chunks.
	SplitPrefix string
}

// PlatformLimitsMap defines per-platform message limits.
var PlatformLimitsMap = map[string]PlatformLimits{
	"discord": {
		MaxMessageLen:    2000,
		SupportsMarkdown: true,
		SupportsHTML:     false,
		SplitPrefix:      "(continued) ",
	},
	"telegram": {
		MaxMessageLen:    4096,
		SupportsMarkdown: false, // Telegram uses HTML, not Markdown
		SupportsHTML:     true,
		SplitPrefix:      "(continued) ",
	},
	"imessage": {
		MaxMessageLen:    4000, // AppleScript command-line limit
		SupportsMarkdown: false,
		SupportsHTML:     false,
		SplitPrefix:      "(continued) ",
	},
	"email": {
		MaxMessageLen:    50000, // Practical SMTP limit (no real hard limit)
		SupportsMarkdown: false,
		SupportsHTML:     false, // Plain text for now
		SplitPrefix:      "",
	},
}

// DefaultLimits is used for unknown platforms.
var DefaultLimits = PlatformLimits{
	MaxMessageLen:    2000,
	SupportsMarkdown: false,
	SupportsHTML:     false,
	SplitPrefix:      "(continued) ",
}

// GetPlatformLimits returns the limits for a platform, with a safe default.
func GetPlatformLimits(platform string) PlatformLimits {
	if limits, ok := PlatformLimitsMap[platform]; ok {
		return limits
	}
	return DefaultLimits
}

// SupportedPlatforms returns the list of supported platform names.
func SupportedPlatforms() []string {
	return []string{"discord", "telegram", "imessage", "email"}
}

// ValidatePlatform checks if a platform is supported.
func ValidatePlatform(platform string) error {
	for _, p := range SupportedPlatforms() {
		if p == platform {
			return nil
		}
	}
	return fmt.Errorf("unsupported platform: %s. Supported: discord, telegram, imessage, email", platform)
}

// ComposePlatformContext returns a human-readable prefix for the platform
// to include in the agent's context so it knows where the message came from.
func ComposePlatformContext(platform, channelName, senderName string) string {
	var platformLabel string
	switch platform {
	case "discord":
		platformLabel = "Discord"
	case "telegram":
		platformLabel = "Telegram"
	case "imessage":
		platformLabel = "iMessage"
	case "email":
		platformLabel = "Email"
	default:
		platformLabel = platform
	}

	if senderName != "" && channelName != "" {
		return fmt.Sprintf("[Message from %s on %s in #%s]", senderName, platformLabel, channelName)
	}
	if senderName != "" {
		return fmt.Sprintf("[Message from %s on %s]", senderName, platformLabel)
	}
	if channelName != "" {
		return fmt.Sprintf("[Message on %s in #%s]", platformLabel, channelName)
	}
	return fmt.Sprintf("[Message on %s]", platformLabel)
}

// ValidateEmailTarget checks if a target string looks like a valid email address.
// Returns the normalized address or an error.
func ValidateEmailTarget(target string) (string, error) {
	addr, err := mail.ParseAddress(target)
	if err != nil {
		return "", fmt.Errorf("invalid email address: %s: %w", target, err)
	}
	return addr.Address, nil
}

// FormatMessageForPlatform formats a response message for a specific platform.
// It applies platform-specific transformations:
//   - Truncation to the platform's max message length
//   - Subject prefix extraction for email ("Subject: topic\nbody" format)
func FormatMessageForPlatform(platform, message string) string {
	limits := GetPlatformLimits(platform)

	// For email, extract subject if present
	if platform == "email" {
		return formatEmailMessage(message, limits)
	}

	// Truncate if needed
	if len(message) > limits.MaxMessageLen {
		return message[:limits.MaxMessageLen] + "\n... (truncated)"
	}

	return message
}

// formatEmailMessage handles email-specific formatting.
func formatEmailMessage(message string, limits PlatformLimits) string {
	if strings.HasPrefix(message, "Subject: ") {
		if len(message) > limits.MaxMessageLen {
			return message[:limits.MaxMessageLen] + "\n... (truncated)"
		}
		return message
	}

	if len(message) > limits.MaxMessageLen {
		return message[:limits.MaxMessageLen] + "\n... (truncated)"
	}
	return message
}

// SplitMessageForPlatform splits a message into chunks appropriate for the platform.
func SplitMessageForPlatform(platform, message string) []string {
	limits := GetPlatformLimits(platform)
	if len(message) <= limits.MaxMessageLen {
		return []string{message}
	}

	chunks := splitMessage(message, limits.MaxMessageLen)
	for i := 1; i < len(chunks); i++ {
		if limits.SplitPrefix != "" {
			chunks[i] = limits.SplitPrefix + chunks[i]
		}
	}
	return chunks
}

// splitMessage splits a message into chunks of at most maxLen characters,
// trying to break on newline boundaries when possible.
func splitMessage(message string, maxLen int) []string {
	if len(message) <= maxLen {
		return []string{message}
	}

	var chunks []string
	for len(message) > 0 {
		if len(message) <= maxLen {
			chunks = append(chunks, message)
			break
		}

		chunkEnd := maxLen
		if idx := strings.LastIndex(message[:chunkEnd], "\n"); idx > 0 {
			chunkEnd = idx + 1
		}

		chunks = append(chunks, message[:chunkEnd])
		message = message[chunkEnd:]
	}

	return chunks
}
