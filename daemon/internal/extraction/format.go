// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package extraction

import (
	"fmt"
	"strings"
)

// formatMessagesForExtraction formats session messages into a text block
// suitable for LLM extraction prompts. Tool outputs are truncated to
// prevent context blowout.
func formatMessagesForExtraction(msgs []SessionMessage) string {
	var b strings.Builder

	for _, m := range msgs {
		switch m.Role {
		case "user":
			fmt.Fprintf(&b, "User: %s\n", m.Content)
		case "assistant":
			if len(m.ToolCalls) > 0 {
				toolNames := make([]string, len(m.ToolCalls))
				for i, tc := range m.ToolCalls {
					toolNames[i] = tc.Name
				}
				if m.Content != "" {
					fmt.Fprintf(&b, "Assistant: %s [tools: %s]\n", m.Content, strings.Join(toolNames, ", "))
				} else {
					fmt.Fprintf(&b, "Assistant: [tools: %s]\n", strings.Join(toolNames, ", "))
				}
			} else if m.Content != "" {
				fmt.Fprintf(&b, "Assistant: %s\n", m.Content)
			}
		case "tool":
			// Truncate tool outputs to prevent context blowout
			output := m.Content
			if len(output) > 200 {
				output = output[:200] + "..."
			}
			fmt.Fprintf(&b, "Tool result (%s): %s\n", m.ToolName, strings.TrimSpace(output))
		}
	}

	return b.String()
}
