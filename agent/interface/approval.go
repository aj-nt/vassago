// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"fmt"
	"strings"
)

// formatApprovalRequest formats a tool approval request for the user.
func formatApprovalRequest(toolName string, args map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(colorize(yellow+bold, "⚠ Approval required"))
	b.WriteString("\n")
	b.WriteString(colorize(bold, fmt.Sprintf("  Tool: %s", toolName)))
	b.WriteString("\n")

	// Show relevant args (limit to prevent wall of text)
	count := 0
	for k, v := range args {
		if count >= 5 {
			b.WriteString(colorize(dim, "  ... (more args)\n"))
			break
		}
		strVal := fmt.Sprintf("%v", v)
		if len(strVal) > 100 {
			strVal = strVal[:100] + "..."
		}
		b.WriteString(fmt.Sprintf("  %s: %s\n", k, colorize(dim, strVal)))
		count++
	}

	b.WriteString(colorize(dim, "\n  Type 'y' to approve, anything else to skip"))
	return b.String()
}

// formatContextUsage shows context token usage.
func formatContextUsage(used, maximum int) string {
	if maximum == 0 {
		return "" // Context management disabled
	}
	usedStr := humanizeTokensCompact(used)
	maxStr := humanizeTokensCompact(maximum)

	// Calculate usage percentage for color coding
	pct := float64(used) / float64(maximum)
	color := green
	if pct > 0.8 {
		color = red
	} else if pct > 0.6 {
		color = yellow
	}

	if !useColor() {
		return fmt.Sprintf("%s/%s", usedStr, maxStr)
	}
	return colorize(color, fmt.Sprintf("%s/%s", usedStr, maxStr))
}

// parseResumeCommand parses "/resume <session-id>" and returns the session ID.
func parseResumeCommand(input string) (string, bool) {
	input = strings.TrimSpace(input)
	// Remove the /resume prefix
	id := strings.TrimPrefix(input, "/resume")
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false
	}
	return id, true
}