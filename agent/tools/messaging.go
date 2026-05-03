package tools

import (
	"fmt"

	"github.com/aj-nt/mnemosyne/client"
	"github.com/aj-nt/mnemosyne/messaging/format"
)

// RegisterMessagingTools adds messaging tool handlers to the registry.
func RegisterMessagingTools(registry *ToolRegistry, mnemo *client.Client, agentID string) {
	registry.Register("SendMessage", makeSendMessage(mnemo, agentID), SendMessageSchema)
}

func makeSendMessage(mnemo *client.Client, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		platform := strArg(args, "platform", "")
		target := strArg(args, "target", "")
		message := strArg(args, "message", "")

		if platform == "" || target == "" || message == "" {
			return "platform, target, and message are required", true
		}

		// Validate platform
		if err := format.ValidatePlatform(platform); err != nil {
			return err.Error(), true
		}

		// Validate email target format
		if platform == "email" {
			validated, err := format.ValidateEmailTarget(target)
			if err != nil {
				return fmt.Sprintf("invalid email address: %s", target), true
			}
			target = validated
		}

		// Check platform-specific message length limits
		limits := format.GetPlatformLimits(platform)
		if len(message) > limits.MaxMessageLen {
			return fmt.Sprintf("Message too long (%d chars, max %d for %s). Break into smaller messages.",
				len(message), limits.MaxMessageLen, platform), true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()

		messageID, err := mnemo.SendMessage(ctx, platform, target, message)
		if err != nil {
			return fmt.Sprintf("SendMessage failed: %v", err), true
		}

		return fmt.Sprintf("Message sent to %s on %s (ID: %s)", target, platform, messageID), false
	}
}

// SendMessageSchema defines the JSON schema for the SendMessage tool.
var SendMessageSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"platform": map[string]interface{}{
			"type":        "string",
			"description": "Messaging platform (discord, telegram, imessage, or email)",
			"enum":        format.SupportedPlatforms(),
		},
		"target": map[string]interface{}{
			"type":        "string",
			"description": "Target channel ID, user ID, or email address",
		},
		"message": map[string]interface{}{
			"type":        "string",
			"description": "Message content to send",
		},
	},
	"required": []string{"platform", "target", "message"},
}