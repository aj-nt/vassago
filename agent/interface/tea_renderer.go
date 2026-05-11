// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// --- Lipgloss styles (shared across the TUI) ---

var (
	styleDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleToolSpin  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Italic(true)
	styleToolOk    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleToolExec  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Italic(true)
	styleContextOK = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleContextWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleContextCrit = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	stylePanelBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	stylePanelHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleUserLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	styleApprovalBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("11")).
				Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// Spin frames for thinking indicator
var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// --- tea.Model: View ---

func (m teaModel) View() string {
	// Layout: viewport (chat history), status line, input area
	vpView := m.viewport.View()

	// Status line
	var statusLine string
	if m.phase == phaseThinking {
		statusLine = styleDim.Render(fmt.Sprintf("  ⚕ Thinking %s", spinFrames[m.spinner%len(spinFrames)]))
	} else if m.phase == phaseApproval {
		statusLine = styleDim.Render("  ⚠ Awaiting approval (y/n)")
	} else if m.stats.Turns > 0 {
		parts := []string{fmt.Sprintf("turn %d", m.stats.Turns)}
		if m.stats.ToolCalls > 0 {
			parts = append(parts, fmt.Sprintf("%d tools", m.stats.ToolCalls))
		}
		statusLine = styleStatusBar.Render("  " + strings.Join(parts, " · "))
	}

	// Input area
	var inputView string
	if m.phase == phaseReady {
		inputView = m.input.View()
	} else if m.phase == phaseApproval {
		inputView = styleDim.Render("  y/n> ")
	} else {
		inputView = styleDim.Render("  ⋯")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		vpView,
		statusLine,
		inputView,
	)
}

// --- Viewport management ---

// rebuildViewport rebuilds the viewport content from chatHistory and current turn state.
func (m *teaModel) rebuildViewport() {
	var b strings.Builder

	// Render completed chat history
	for _, line := range m.chatHistory {
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Render current turn state
	switch m.phase {
	case phaseThinking:
		b.WriteString(styleDim.Render(fmt.Sprintf("  ⚕ Thinking %s", spinFrames[m.spinner%len(spinFrames)])))
		b.WriteString("\n")

	case phaseStreaming, phaseToolCall:
		if m.turnText.Len() > 0 {
			b.WriteString(m.turnText.String())
			b.WriteString("\n")
		}
		for _, line := range m.toolLogs {
			b.WriteString(line)
			b.WriteString("\n")
		}

	case phaseApproval:
		b.WriteString("\n")
		b.WriteString(styleApprovalBox.Render(
			fmt.Sprintf("⚠ Approval required\n  Tool: %s\n  Type 'y' to approve, anything else to skip",
				m.pendingApprovalToolName)))
		b.WriteString("\n")
	}

	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}

// endTurn finalizes a completed turn and resets state.
func (m *teaModel) endTurn(fallbackText string) {
	text := strings.TrimRight(m.turnText.String(), " \t\n")
	if text == "" && fallbackText != "" {
		text = fallbackText
	}
	if text != "" {
		panel := formatResponseBubble(text, m.agent.Config.AgentName, m.width)
		m.chatHistory = append(m.chatHistory, panel)

		if m.convCtx != nil {
			tokenCount := m.convCtx.TokenCount()
			ctxStr := formatContextUsageBubble(tokenCount, m.agent.Config.MaxContextTokens)
			if ctxStr != "" {
				m.chatHistory = append(m.chatHistory, styleDim.Render("  📊 "+ctxStr))
			}
		}
		m.chatHistory = append(m.chatHistory, "") // blank separator
	}

	m.turnText.Reset()
	m.reasoningText.Reset()
	m.toolLogs = nil
	m.phase = phaseReady
}

// --- Bubble/lipgloss formatters ---

// formatResponseBubble wraps response text in a lipgloss bordered panel.
func formatResponseBubble(text, agentName string, width int) string {
	if text == "" {
		return ""
	}

	maxContent := width - 6
	if maxContent < 30 {
		maxContent = 30
	}

	wrapped := wrapText(text, maxContent)
	lines := strings.Split(wrapped, "\n")

	contentWidth := utf8.RuneCountInString(" ⚕ "+agentName+" ")
	for _, line := range lines {
		if l := utf8.RuneCountInString(line); l > contentWidth {
			contentWidth = l
		}
	}

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		PaddingLeft(1).
		PaddingRight(1).
		Width(contentWidth + 2)

	var b strings.Builder
	b.WriteString(stylePanelHeader.Render("⚕ " + agentName))
	b.WriteString("\n")
	b.WriteString(strings.Join(lines, "\n"))

	return panelStyle.Render(b.String())
}

// formatContextUsageBubble shows context token usage with color coding.
func formatContextUsageBubble(used, maximum int) string {
	if maximum == 0 {
		return ""
	}
	pct := float64(used) / float64(maximum)
	usedStr := humanizeTokensCompact(used)
	maxStr := humanizeTokensCompact(maximum)

	switch {
	case pct > 0.8:
		return styleContextCrit.Render(fmt.Sprintf("%s/%s tokens", usedStr, maxStr))
	case pct > 0.6:
		return styleContextWarn.Render(fmt.Sprintf("%s/%s tokens", usedStr, maxStr))
	default:
		return styleContextOK.Render(fmt.Sprintf("%s/%s tokens", usedStr, maxStr))
	}
}

// truncateForUI truncates a string to maxLen for display.
func truncateForUI(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
