package tools

import (
	"fmt"

	"github.com/aj-nt/mnemosyne/client"
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
		if platform != "discord" {
			return fmt.Sprintf("unsupported platform: %s. Only 'discord' is currently supported", platform), true
		}

		// Reject oversized messages
		const maxMessageLen = 2000 // Discord message limit
		if len(message) > maxMessageLen {
			return fmt.Sprintf("Message too long (%d chars, max %d). Break into smaller messages.", len(message), maxMessageLen), true
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
			"description": "Messaging platform (currently only 'discord' is supported)",
			"enum":        []string{"discord"},
		},
		"target": map[string]interface{}{
			"type":        "string",
			"description": "Target channel ID or user ID",
		},
		"message": map[string]interface{}{
			"type":        "string",
			"description": "Message content to send",
		},
	},
	"required": []string{"platform", "target", "message"},
}