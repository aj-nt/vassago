// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/client"
)

// --- Lipgloss styles ---

var (
	styleAgentName = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleToolStart = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Italic(true)
	styleToolOkay  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleToolExec = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Italic(true)
	styleContextOK  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleContextWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleContextCrit = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// --- Bubble Tea model ---

type teaModel struct {
	agent *agent.Agent
	ctx   context.Context

	// Session state
	sessionID string
	messages  []agentctx.Message
	toolDefs  []agentctx.ToolDefinition
	hotBlock  string
	stats     SessionStats

	// UI state
	input      textinput.Model
	response   strings.Builder // current streaming response text
	toolLines  []string        // tool indicator lines during streaming
	thinking   bool
	responding bool
	err        error
	phase      phase // where we are in the turn cycle

	// Spinner counter for thinking animation
	spinner int

	// Channel for streaming events from the agent
	eventCh chan agent.StreamEvent

	// Terminal width
	width int

	// Current conversation context (updated after each turn)
	convCtx *agentctx.ConversationContext

	// Interrupt signal for cancellation
	interrupt *agentctx.InterruptSignal
	cancel    context.CancelFunc
}

type phase int

const (
	phaseReady    phase = iota // waiting for user input
	phaseThinking               // agent is thinking, spinner shown
	phaseStreaming              // agent is streaming response
	phaseToolCall               // agent is making tool calls
)

// --- Messages ---

type streamEventMsg struct{ event agent.StreamEvent }
type thinkingTickMsg struct{}

// --- Public entry point ---

// RunInteractiveBubble starts an interactive chat session using Bubble Tea.
func RunInteractiveBubble(ctx context.Context, a *agent.Agent) error {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Prompt = "❯ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	ti.CharLimit = 0
	ti.Width = 70

	sessionID, _ := a.Mnemo.CreateSession(ctx, a.Config.AgentID, "cli", "Interactive session")
	hotBlock, _ := a.GetHotBlock(ctx)

	m := teaModel{
		agent:     a,
		ctx:       ctx,
		sessionID: sessionID,
		toolDefs:  a.BuildToolDefinitions(),
		hotBlock:  sanitizeUTF8(hotBlock),
		input:     ti,
		eventCh:   make(chan agent.StreamEvent, 64),
		width:     80,
		phase:     phaseReady,
		interrupt: agentctx.NewInterruptSignal(),
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// --- tea.Model interface ---

func (m teaModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.input.Width = max(msg.Width-6, 20)
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.responding {
				// Interrupt current response
				if m.interrupt != nil {
					m.interrupt.Set()
				}
				if m.cancel != nil {
					m.cancel()
				}
				m.responding = false
				m.thinking = false
				m.phase = phaseReady
				m.response.Reset()
				m.toolLines = nil
				m.input.Focus()
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyEnter:
			if m.responding {
				return m, nil // ignore input while responding
			}
			input := m.input.Value()
			if input == "" {
				return m, nil
			}

			// Handle slash commands
			if strings.HasPrefix(input, "/") {
				handled, quitCmd := m.handleCommand(input)
				if handled {
					m.input.SetValue("")
					if quitCmd != nil {
						return m, quitCmd
					}
					return m, nil
				}
			}

			// Start agent response
			m.messages = append(m.messages, agentctx.Message{Role: "user", Content: input})
			m.input.SetValue("")
			m.input.Blur()
			m.responding = true
			m.thinking = true
			m.phase = phaseThinking
			m.response.Reset()
			m.toolLines = nil
			m.err = nil
			m.spinner = 0

			return m, tea.Batch(
				m.startAgentTurn(input),
				m.thinkingTick(),
			)
		}

	case streamEventMsg:
		cmd := m.handleStreamEvent(msg.event)
		if m.responding {
			return m, tea.Batch(cmd, m.waitForStreamEvent())
		}
		m.input.Focus()
		return m, cmd

	case thinkingTickMsg:
		if m.thinking {
			m.spinner++
			return m, m.thinkingTick()
		}
		return m, nil
	}

	// Default: update text input
	if m.phase == phaseReady && m.input.Focused() {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m teaModel) View() string {
	var b strings.Builder
	width := m.width

	switch m.phase {
	case phaseThinking:
		// Show spinner
		spinFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		b.WriteString(styleDim.Render(fmt.Sprintf("  ⚕ Thinking %s", spinFrames[m.spinner%len(spinFrames)])))
		b.WriteString("\n\n")

	case phaseStreaming, phaseToolCall:
		// Show accumulated response
		if m.response.Len() > 0 {
			b.WriteString(m.response.String())
			b.WriteString("\n")
		}
		// Show tool indicators
		for _, line := range m.toolLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")

	case phaseReady:
		// Show completed response with context usage
		if m.response.Len() > 0 {
			text := strings.TrimRight(m.response.String(), " \t\n")
			if text != "" {
				b.WriteString(formatResponseBubble(text, m.agent.Config.AgentName, width))
				b.WriteString("\n")
			}

			// Context usage
			if m.convCtx != nil {
				tokenCount := m.convCtx.TokenCount()
				ctxStr := formatContextUsageBubble(tokenCount, m.agent.Config.MaxContextTokens)
				if ctxStr != "" {
					b.WriteString(styleDim.Render("  📊 " + ctxStr))
					b.WriteString("\n")
				}
			}
			b.WriteString("\n")
		}

		// Show error if any
		if m.err != nil {
			b.WriteString(styleError.Render(fmt.Sprintf("Error: %v", m.err)))
			b.WriteString("\n\n")
		}
	}

	// Always show the prompt
	if m.phase == phaseReady {
		b.WriteString(m.input.View())
	} else {
		b.WriteString(styleDim.Render("  ⋯"))
	}

	return b.String()
}

// --- Internal methods ---

// handleStreamEvent processes a streaming event and updates model state.
func (m *teaModel) handleStreamEvent(ev agent.StreamEvent) tea.Cmd {
	switch ev.Type {
	case agentctx.StreamText:
		if m.thinking {
			m.thinking = false
			m.phase = phaseStreaming
		}
		m.response.WriteString(ev.Text)

	case agentctx.StreamToolStart:
		m.thinking = false
		m.phase = phaseToolCall
		m.stats.ToolCalls++
		m.toolLines = append(m.toolLines,
			fmt.Sprintf("  %s %s", styleToolStart.Render("⟳"), styleDim.Render(ev.ToolName)))

	case agentctx.StreamToolExec:
		m.phase = phaseToolCall
		m.toolLines = append(m.toolLines,
			fmt.Sprintf("  %s %s", styleToolExec.Render("⚙"), styleDim.Render(ev.ToolName)))

	case agentctx.StreamToolEnd:
		// Mark the last tool indicator as complete
		for i := len(m.toolLines) - 1; i >= 0; i-- {
			if strings.Contains(m.toolLines[i], "⟳") {
				// Replace spinner with checkmark
				toolName := ev.ToolName
				if toolName == "" {
					toolName = "tool"
				}
				m.toolLines[i] = fmt.Sprintf("  %s %s", styleToolOkay.Render("✓"), styleDim.Render(toolName))
				break
			}
		}

	case agentctx.StreamError:
		m.err = ev.Error
		m.responding = false
		m.thinking = false
		m.phase = phaseReady
		m.input.Focus()

	case agentctx.StreamDone:
		m.responding = false
		m.thinking = false
		m.phase = phaseReady
		m.stats.Turns++
		if ev.Usage != nil {
			m.stats.TokensUsed += ev.Usage.TotalTokens
		}
		if m.agent.Router != nil {
			cost, _, _ := m.agent.Router.SessionCost()
			m.stats.CostUSD = cost
		}
		m.input.Focus()
	}
	return nil
}

// startAgentTurn launches the agent streaming goroutine and returns a command
// that waits for the first event.
func (m *teaModel) startAgentTurn(input string) tea.Cmd {
	return func() tea.Msg {
		// Refresh hot block
		if hb, err := m.agent.GetHotBlock(m.ctx); err == nil && hb != "" {
			m.hotBlock = sanitizeUTF8(hb)
		}

		compacted := compactMessagesForNextTurn(m.messages)
		m.interrupt = agentctx.NewInterruptSignal()
		turnCtx, cancel := context.WithCancel(m.ctx)
		m.cancel = cancel

		convCtx := &agentctx.ConversationContext{
			SessionID:       m.sessionID,
			AgentID:         m.agent.Config.AgentID,
			Messages:        compacted,
			SystemPrompt:    "",
			Tools:           m.toolDefs,
			MaxIterations:   m.agent.Config.MaxTurns,
			Iteration:       0,
			Interrupt:       m.interrupt,
			HotBlockContent: m.hotBlock,
			SkillContent:     m.agent.GetSkillBlock(m.ctx),
			Temperature:     m.agent.Config.Temperature,
		}

		// Wire tool approval
		// NOTE: In Bubble Tea mode, we auto-approve for now.
		// Phase 2 will add interactive approval prompts.
		m.agent.Stage4.Approver = func(toolName string, args map[string]interface{}) bool {
			return true
		}

		go m.agent.RunStreaming(turnCtx, convCtx, m.eventCh)

		// Store context for token count display
		m.convCtx = convCtx

		// Wait for first event
		ev, ok := <-m.eventCh
		if !ok {
			return streamEventMsg{event: agent.StreamEvent{Type: agentctx.StreamDone}}
		}
		return streamEventMsg{event: ev}
	}
}

// waitForStreamEvent returns a command that waits for the next stream event.
func (m *teaModel) waitForStreamEvent() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.eventCh
		if !ok {
			return streamEventMsg{event: agent.StreamEvent{Type: agentctx.StreamDone}}
		}
		return streamEventMsg{event: ev}
	}
}

// thinkingTick returns a command that sends a thinkingTickMsg after 100ms.
func (m *teaModel) thinkingTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return thinkingTickMsg{}
	})
}

// handleCommand processes slash commands. Returns (handled, quitCmd).
// quitCmd is non-nil when the user wants to exit.
func (m *teaModel) handleCommand(input string) (bool, tea.Cmd) {
	switch strings.TrimSpace(input) {
	case "/quit", "/exit":
		// Persist session
		if m.sessionID != "" && len(m.messages) > 0 {
			msgs := make([]client.Message, 0, len(m.messages))
			for _, msg := range m.messages {
				msgs = append(msgs, client.Message{Role: msg.Role, Content: msg.Content})
			}
			_ = m.agent.Mnemo.AddMessages(m.ctx, m.sessionID, msgs)
		}
		return true, tea.Quit

	case "/clear":
		m.messages = m.messages[:0]
		m.response.Reset()
		m.toolLines = nil
		m.err = nil
		m.convCtx = nil
		return true, nil

	case "/hot":
		m.response.Reset()
		m.toolLines = nil
		m.phase = phaseReady
		m.response.WriteString(m.hotBlock)
		return true, nil

	case "/history":
		m.response.Reset()
		m.toolLines = nil
		m.phase = phaseReady
		if len(m.messages) == 0 {
			m.response.WriteString("[No messages in current context]")
		} else {
			for _, msg := range m.messages {
				m.response.WriteString(formatHistoryLine(msg.Role, msg.Content) + "\n")
			}
		}
		return true, nil

	case "/sessions":
		m.response.Reset()
		m.toolLines = nil
		m.phase = phaseReady
		if !m.agent.Mnemo.IsConnected() {
			m.response.WriteString("[Not connected to daemon]")
		} else {
			sessions, err := m.agent.Mnemo.ListRecentSessions(m.ctx, 10)
			if err != nil {
				m.err = err
			} else if len(sessions) == 0 {
				m.response.WriteString("[No sessions found]")
			} else {
				for _, s := range sessions {
					title := s.Title
					if title == "" {
						title = "(untitled)"
					}
					m.response.WriteString(formatSessionLine(s.ID[:25], title, int(s.MessageCount), s.Source) + "\n")
				}
			}
		}
		return true, nil
	}

	if strings.HasPrefix(input, "/resume") {
		id, ok := parseResumeCommand(input)
		if !ok {
			m.err = fmt.Errorf("usage: /resume <session-id>")
			return true, nil
		}
		if !m.agent.Mnemo.IsConnected() {
			m.err = fmt.Errorf("not connected to daemon")
			return true, nil
		}
		detail, err := m.agent.Mnemo.GetSession(m.ctx, id)
		if err != nil {
			m.err = err
			return true, nil
		}
		resumedMsgs := make([]agentctx.Message, 0, len(detail.Messages))
		for _, msg := range detail.Messages {
			resumedMsgs = append(resumedMsgs, agentctx.Message{Role: msg.Role, Content: msg.Content})
		}
		m.messages = resumedMsgs
		m.sessionID = id
		title := detail.Title
		if title == "" {
			title = "(untitled)"
		}
		m.response.Reset()
		m.phase = phaseReady
		m.response.WriteString(formatResumedSession(title, len(m.messages)))
		return true, nil
	}

	return false, nil
}

// formatResponseBubble wraps response text in a bordered panel using lipgloss.
func formatResponseBubble(text, agentName string, width int) string {
	if text == "" {
		return ""
	}

	maxWidth := width - 4 // leave room for borders
	if maxWidth < 40 {
		maxWidth = 40
	}

	wrapped := wrapText(text, maxWidth)
	lines := strings.Split(wrapped, "\n")

	// Calculate content width
	header := " ⚕ " + agentName + " "
	headerLen := utf8.RuneCountInString(header)
	contentWidth := headerLen
	for _, line := range lines {
		l := utf8.RuneCountInString(line)
		if l > contentWidth {
			contentWidth = l
		}
	}

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		PaddingLeft(1).
		PaddingRight(1)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12"))

	contentStyle := lipgloss.NewStyle().
		Width(contentWidth)

	// Build header line
	headerLine := headerStyle.Render(header)

	// Build content lines
	var contentLines []string
	for _, line := range lines {
		contentLines = append(contentLines, contentStyle.Render(line))
	}

	// Combine header + content
	var b strings.Builder
	b.WriteString(headerLine)
	b.WriteString("\n")
	b.WriteString(strings.Join(contentLines, "\n"))

	return borderStyle.Render(b.String())
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

// max returns the larger of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}