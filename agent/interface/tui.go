package iface

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// ANSI color/style codes for terminal output.
const (
	reset      = "\033[0m"
	bold       = "\033[1m"
	dim        = "\033[2m"
	italic     = "\033[3m"
	underline  = "\033[4m"

	// Foreground colors
	black   = "\033[30m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"

	// Bright foreground colors
	brightRed    = "\033[91m"
	brightGreen  = "\033[92m"
	brightYellow = "\033[93m"
	brightBlue   = "\033[94m"
	brightMagenta = "\033[95m"
	brightCyan   = "\033[96m"
	brightWhite  = "\033[97m"

	// Box-drawing characters
	boxTL = "╭"
	boxTR = "╮"
	boxBL = "╰"
	boxBR = "╯"
	boxH  = "─"
	boxV  = "│"
)

// isTerminal checks if stdout is a TTY. If not, we skip colors.
// (We import mattn/go-isatty but for simplicity just use env heuristics.)

// colorize wraps text in an ANSI color code.
func colorize(color, text string) string {
	if !useColor() {
		return text
	}
	return color + text + reset
}

// useColor returns true if we should emit ANSI color codes.
// Checks TERM and NO_COLOR environment variables.
func useColor() bool {
	// Respect NO_COLOR convention (https://no-color.org/)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return os.Getenv("TERM") != "dumb"
}

// --- Output formatters ---

// formatBanner prints the startup banner.
func formatBanner(agentName, personality string) string {
	if !useColor() {
		return fmt.Sprintf("Mnemosyne Agent — %s", agentName)
	}

	lines := []string{
		bold + brightCyan + "  § " + agentName + reset,
	}
	if personality != "" {
		// Truncate personality for display
		p := personality
		if utf8.RuneCountInString(p) > 60 {
			p = truncateRunes(p, 57) + "..."
		}
		lines = append(lines, dim+"  "+p+reset)
	}
	return strings.Join(lines, "\n")
}

// formatStartupInfo prints a compact status line on startup.
func formatStartupInfo(memCount int, hotBlockChars int, personality string) string {
	if !useColor() {
		return fmt.Sprintf("memories: %d | hot block: %d chars", memCount, hotBlockChars)
	}
	parts := []string{
		colorize(brightYellow, "🧠") + colorize(dim, fmt.Sprintf(" %d memories", memCount)),
		colorize(brightBlue, "📝") + colorize(dim, fmt.Sprintf(" hot block: %s", humanizeChars(hotBlockChars))),
	}
	return dim + "  " + strings.Join(parts, dim+" • "+reset+dim) + reset
}

// formatPrompt returns the styled input prompt.
func formatPrompt(agentName string) string {
	if !useColor() {
		return "> "
	}
	return colorize(brightCyan, "❯ ") + reset
}

// formatCommandsHelp returns the help line for available commands.
func formatCommandsHelp() string {
	if !useColor() {
		return "type /quit to exit, /clear to reset, /hot for hot block, /history for context, /sessions for recent, /resume to restore"
	}
	return dim + "type " +
		colorize(yellow, "/quit") + dim + " exit · " +
		colorize(yellow, "/clear") + dim + " reset · " +
		colorize(yellow, "/hot") + dim + " hot block · " +
		colorize(yellow, "/history") + dim + " context · " +
		colorize(yellow, "/sessions") + dim + " recent · " +
		colorize(yellow, "/resume") + dim + " restore" + reset
}

// formatResumedSession shows confirmation of a resumed session.
func formatResumedSession(title string, msgCount int) string {
	if !useColor() {
		return fmt.Sprintf("Resumed session: %s (%d messages)", title, msgCount)
	}
	return colorize(brightCyan, "Resumed") + ": " + colorize(bold, title) +
		colorize(dim, fmt.Sprintf(" (%d messages)", msgCount))
}

// formatToolStart shows a tool dispatch indicator when a tool call begins.
func formatToolStart(toolName string) string {
	if !useColor() {
		return fmt.Sprintf("  → %s", toolName)
	}
	return fmt.Sprintf("  %s %s", colorize(cyan, "⟳"), colorize(dim+italic, toolName))
}

// formatToolEnd shows a tool completion indicator when a tool call finishes.
func formatToolEnd(toolName string) string {
	if !useColor() {
		return fmt.Sprintf("  ✓ %s", toolName)
	}
	return fmt.Sprintf("  %s %s", colorize(brightGreen, "✓"), colorize(dim, toolName))
}

// formatToolExec shows when the agent is dispatching/running a tool.
func formatToolExec(toolName string) string {
	if !useColor() {
		return fmt.Sprintf("  ⚙ %s", toolName)
	}
	return fmt.Sprintf("  %s %s", colorize(brightYellow, "⚙"), colorize(dim+italic, toolName))
}

// formatToolError shows a tool error indicator.
func formatToolError(toolName, errMsg string) string {
	if !useColor() {
		return fmt.Sprintf("  ✗ %s: %s", toolName, errMsg)
	}
	return fmt.Sprintf("  %s %s: %s", colorize(brightRed, "✗"), colorize(dim, toolName), colorize(brightRed, errMsg))
}

// formatThinkingIndicator shows while waiting for the first streaming token.
func formatThinkingIndicator() string {
	if !useColor() {
		return "Thinking..."
	}
	return colorize(dim+italic, "  ⚕ Thinking...")
}

// formatClearThinking erases the thinking indicator line.
func formatClearThinking() string {
	if !useColor() {
		return "\r             \r"
	}
	// Clear the current line and move cursor to beginning
	return "\r\033[2K"
}

// formatResponse wraps the agent's text response in a bordered panel.
// If the response is empty or very short, skip the panel.
func formatResponse(text string, agentName string) string {
	text = strings.TrimRight(text, " \t\n")
	if text == "" {
		return ""
	}

	if !useColor() {
		return text + "\n"
	}

	// Word-wrap the text to a reasonable width
	maxWidth := 74 // leaves room for "│ " and " │" borders
	wrapped := wrapText(text, maxWidth)
	lines := strings.Split(wrapped, "\n")

	// Calculate content width (minimum: header length)
	header := " ⚕ " + agentName + " "
	headerLen := utf8.RuneCountInString(header)
	contentWidth := headerLen
	for _, line := range lines {
		l := utf8.RuneCountInString(line)
		if l > contentWidth {
			contentWidth = l
		}
	}

	// Total visual width of each line (borders included)
	// Content: │ text... │  = 2 + contentWidth + 2 = contentWidth + 4
	totalWidth := contentWidth + 4

	var b strings.Builder

	// Top border: ╭─ ⚕ name ─╮
	// Visual width must equal totalWidth (contentWidth + 4)
	// Layout: ╭(1) + ─(1) + header + fill(─) + ─(1) + ╮(1) = totalWidth
	// So header + fill + 4 = totalWidth → fill = contentWidth - headerLen
	b.WriteString(colorize(dim, boxTL))
	b.WriteString(colorize(dim, boxH))
	b.WriteString(colorize(brightCyan+bold, header))
	rightFill := contentWidth - headerLen
	if rightFill > 0 {
		b.WriteString(colorize(dim, strings.Repeat(boxH, rightFill)))
	}
	b.WriteString(colorize(dim, boxH))
	b.WriteString(colorize(dim, boxTR))
	b.WriteString("\n")

	// Content lines: │ text │
	for _, line := range lines {
		lineLen := utf8.RuneCountInString(line)
		padding := contentWidth - lineLen
		b.WriteString(colorize(dim, boxV+" "))
		b.WriteString(line)
		if padding > 0 {
			b.WriteString(strings.Repeat(" ", padding))
		}
		b.WriteString(colorize(dim, " "+boxV))
		b.WriteString("\n")
	}

	// Bottom border: ╰──────────────╯
	b.WriteString(colorize(dim, boxBL))
	b.WriteString(colorize(dim, strings.Repeat(boxH, totalWidth-2))) // -2 for ╰ and ╯
	b.WriteString(colorize(dim, boxBR))

	return b.String()
}

// formatInterruptSignal shows when the user Ctrl+C's during generation.
func formatInterruptSignal() string {
	if !useColor() {
		return "\nInterrupting..."
	}
	return "\n" + colorize(brightYellow, "⚠ Interrupting...")
}

// formatGoodbye shows the exit message.
func formatGoodbye(stats *SessionStats) string {
	if !useColor() {
		if stats != nil {
			costStr := ""
			if stats.CostUSD > 0 {
				costStr = fmt.Sprintf(", $%.4f", stats.CostUSD)
			}
			return fmt.Sprintf("Goodbye! (%d turns, %d tool calls, %d tokens%s)", stats.Turns, stats.ToolCalls, stats.TokensUsed, costStr)
		}
		return "Goodbye!"
	}
	base := colorize(brightCyan+bold, "Goodbye!")
	if stats != nil && stats.Turns > 0 {
		costStr := ""
		if stats.CostUSD > 0 {
			costStr = fmt.Sprintf(" · $%.4f", stats.CostUSD)
		}
		base += " " + colorize(dim, fmt.Sprintf("(%d turns · %d tool calls · %s%s)",
			stats.Turns, stats.ToolCalls, humanizeTokens(stats.TokensUsed), costStr))
	}
	return base
}

// formatError prints a styled error message.
func formatError(err error) string {
	if !useColor() {
		return fmt.Sprintf("Error: %v", err)
	}
	return colorize(brightRed, fmt.Sprintf("Error: %v", err))
}

// formatWarning prints a styled warning message.
func formatWarning(msg string) string {
	if !useColor() {
		return fmt.Sprintf("Warning: %s", msg)
	}
	return colorize(brightYellow, "⚠ ") + colorize(dim, msg)
}

// formatContextCleared shows when /clear is invoked.
func formatContextCleared() string {
	if !useColor() {
		return "[Context cleared]"
	}
	return colorize(brightGreen, "✓") + colorize(dim, " Context cleared")
}

// formatSessionLine formats a single session in /sessions output.
func formatSessionLine(id, title string, msgCount int, source string) string {
	if !useColor() {
		return fmt.Sprintf("  %-25s  %-20s  %3d msg  [%s]", id, title, msgCount, source)
	}
	return "  " +
		colorize(dim, fmt.Sprintf("%-25s", id)) +
		colorize(brightWhite, fmt.Sprintf("%-20s", title)) +
		colorize(brightYellow, fmt.Sprintf("%3d", msgCount)) + colorize(dim, " msg") +
		colorize(dim, fmt.Sprintf("  [%s]", source))
}

// formatHistoryLine formats a single message in /history output.
func formatHistoryLine(role, content string) string {
	preview := content
	if utf8.RuneCountInString(preview) > 100 {
		preview = truncateRunes(preview, 97) + "..."
	}
	if !useColor() {
		return fmt.Sprintf("  [%s] %s", role, preview)
	}
	roleColor := dim
	switch role {
	case "user":
		roleColor = brightCyan
	case "assistant":
		roleColor = brightGreen
	case "system":
		roleColor = yellow
	case "tool":
		roleColor = magenta
	}
	return "  " + colorize(roleColor, fmt.Sprintf("[%s]", role)) + " " + colorize(dim, preview)
}

// --- Utility functions ---

// SessionStats tracks aggregate stats for the session.
type SessionStats struct {
	Turns      int
	ToolCalls  int
	TokensUsed int
	CostUSD    float64
}

// wrapText wraps text to maxWidth characters, preserving existing newlines.
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	var b strings.Builder
	for _, paragraph := range strings.Split(text, "\n") {
		if utf8.RuneCountInString(paragraph) <= maxWidth {
			b.WriteString(paragraph)
			b.WriteByte('\n')
			continue
		}
		// Simple word-wrap
		col := 0
		for _, word := range strings.Split(paragraph, " ") {
			wordLen := utf8.RuneCountInString(word)
			if col > 0 && col+1+wordLen > maxWidth {
				b.WriteByte('\n')
				col = 0
			}
			if col > 0 {
				b.WriteByte(' ')
				col++
			}
			b.WriteString(word)
			col += wordLen
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// truncateRunes truncates a string to at most n runes.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n])
}

// humanizeChars converts a character count to a human-readable string.
func humanizeChars(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d chars", n)
	}
	return fmt.Sprintf("%.1fK chars", float64(n)/1000)
}

// humanizeTokens converts a token count to a human-readable string.
func humanizeTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d tokens", n)
	}
	return fmt.Sprintf("%.1fK tokens", float64(n)/1000)
}

// humanizeTokensCompact converts a token count to a compact string (no "tokens" suffix).
func humanizeTokensCompact(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.0fK", float64(n)/1000)
}
