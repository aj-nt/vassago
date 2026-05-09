// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/tools"
	"github.com/aj-nt/mnemosyne/client"
)

// --- Lipgloss styles ---

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

// --- Phases ---

type phase int

const (
	phaseReady      phase = iota // waiting for user input
	phaseThinking                 // agent thinking, spinner shown
	phaseStreaming                // agent streaming text response
	phaseToolCall                  // agent executing tool calls
	phaseApproval                  // waiting for user approval of a tool call
)

// --- Messages ---

type streamEventMsg struct{ event agent.StreamEvent }
type thinkingTickMsg struct{}
type approvalResultMsg struct{ approved bool }

// --- Model ---

type teaModel struct {
	agent *agent.Agent
	ctx   context.Context

	// Session state
	sessionID string
	messages  []agentctx.Message
	toolDefs  []agentctx.ToolDefinition
	hotBlock  string
	stats     SessionStats

	// UI components
	input    textarea.Model
	viewport viewport.Model

	// Current streaming response
	turnText strings.Builder
	toolLogs []string

	// Chat history (rendered into viewport)
	chatHistory []string

	// Input history (up/down arrow recall)
	inputHistory []string
	historyIdx   int // -1 = not navigating history

	// UI state
	phase      phase
	thinking   bool
	responding bool
	err        error
	spinner    int
	width      int
	height     int

	// Agent streaming
	eventCh  chan agent.StreamEvent
	convCtx  *agentctx.ConversationContext
	interrupt *agentctx.InterruptSignal
	cancel    context.CancelFunc

	// Tool approval state (for interactive approval — Phase 3b)
	// Currently auto-approved; will be made interactive later.
	pendingApprovalToolName string
	pendingApprovalArgs    map[string]interface{}
}

// --- Phases ---

// RunInteractiveBubble starts an interactive chat session using Bubble Tea.
func RunInteractiveBubble(ctx context.Context, a *agent.Agent) error {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Shift+Enter for newline, /quit to exit)"
	ta.Prompt = "❯ "
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.SetWidth(70)
	ta.SetHeight(1) // single line by default, expands for multi-line

	// Enable cursor blinking
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	sessionID, _ := a.Mnemo.CreateSession(ctx, a.Config.AgentID, "cli", "Interactive session")
	hotBlock, _ := a.GetHotBlock(ctx)

	vp := viewport.New(80, 20)

	m := teaModel{
		agent:     a,
		ctx:       ctx,
		sessionID: sessionID,
		toolDefs:  a.BuildToolDefinitions(),
		hotBlock:  sanitizeUTF8(hotBlock),
		input:     ta,
		viewport:  vp,
		eventCh:   make(chan agent.StreamEvent, 64),
		width:     80,
		height:    24,
		phase:     phaseReady,
		interrupt: agentctx.NewInterruptSignal(),
		historyIdx: -1,
	}

	// Build startup banner
	banner := formatBanner(a.Config.AgentName, a.Config.Personality)
	banner += "\n"
	banner += formatStartupInfo(0, utf8.RuneCountInString(m.hotBlock), "")
	banner += "\n"
	banner += formatCommandsHelp()
	banner += "\n"
	m.chatHistory = []string{banner}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// --- tea.Model interface ---

func (m teaModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(max(msg.Width-4, 20))
		m.viewport.Width = msg.Width
		m.viewport.Height = max(msg.Height-5, 5)
		m.rebuildViewport()
		return m, nil

	case tea.KeyMsg:
		// Handle special keys first
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.phase == phaseApproval {
				// Deny approval
				m.phase = phaseReady
				m.input.SetValue("")
				m.input.Focus()
				m.chatHistory = append(m.chatHistory, styleDim.Render("  ⚠ Tool approval denied"))
				m.chatHistory = append(m.chatHistory, "")
				m.rebuildViewport()
				return m, nil
			}
			if m.responding {
				if m.interrupt != nil {
					m.interrupt.Set()
				}
				if m.cancel != nil {
					m.cancel()
				}
				m.responding = false
				m.thinking = false
				m.phase = phaseReady
				m.endTurn("Interrupted.")
				m.input.Focus()
				m.rebuildViewport()
				return m, nil
			}
			m.persistSession()
			return m, tea.Quit

		case tea.KeyUp:
			// Input history navigation (only in ready phase, not composing)
			if m.phase == phaseReady && !m.input.Focused() {
				// shouldn't happen, but be safe
			} else if m.phase == phaseReady {
				if len(m.inputHistory) > 0 && m.historyIdx < len(m.inputHistory)-1 {
					m.historyIdx++
					m.input.SetValue(m.inputHistory[len(m.inputHistory)-1-m.historyIdx])
					m.input.CursorEnd()
					return m, nil
				}
			}

		case tea.KeyDown:
			// Input history navigation
			if m.phase == phaseReady {
				if m.historyIdx > 0 {
					m.historyIdx--
					m.input.SetValue(m.inputHistory[len(m.inputHistory)-1-m.historyIdx])
					m.input.CursorEnd()
				} else if m.historyIdx == 0 {
					m.historyIdx = -1
					m.input.SetValue("")
				}
				return m, nil
			}

		case tea.KeyEnter:
			// Approval mode: 'y' = approve, anything else = deny
			if m.phase == phaseApproval {
				input := strings.ToLower(strings.TrimSpace(m.input.Value()))
				approved := input == "y" || input == "yes"
				m.phase = phaseReady
				m.input.SetValue("")
				m.input.Focus()
				if approved {
					m.chatHistory = append(m.chatHistory, styleToolOk.Render("  ✓ Approved"))
				} else {
					m.chatHistory = append(m.chatHistory, styleDim.Render("  ⚠ Tool approval denied"))
				}
				m.chatHistory = append(m.chatHistory, "")
				m.rebuildViewport()
				// Note: actual approval wiring is Phase 3b — currently auto-approved
				return m, nil
			}

			if m.responding {
				return m, nil // ignore input while responding
			}

			input := strings.TrimSpace(m.input.Value())
			if input == "" {
				return m, nil
			}

			// Save to history
			m.inputHistory = append(m.inputHistory, input)
			m.historyIdx = -1

			// Handle slash commands
			if strings.HasPrefix(input, "/") {
				handled, quitCmd := m.handleCommand(input)
				if handled {
					m.input.SetValue("")
					m.rebuildViewport()
					if quitCmd != nil {
						return m, quitCmd
					}
					return m, nil
				}
			}

			// Show user message in history
			m.chatHistory = append(m.chatHistory, styleUserLabel.Render("You: ")+input)

			// Start agent response
			m.messages = append(m.messages, agentctx.Message{Role: "user", Content: input})
			m.input.SetValue("")
			m.input.Blur()
			m.responding = true
			m.thinking = true
			m.phase = phaseThinking
			m.turnText.Reset()
			m.toolLogs = nil
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
		m.rebuildViewport()
		return m, cmd

	case thinkingTickMsg:
		if m.thinking {
			m.spinner++
			m.rebuildViewport()
			return m, m.thinkingTick()
		}
		return m, nil
	}

	// Default: update text input/viewport when ready
	if m.phase == phaseReady || m.phase == phaseApproval {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.viewport, _ = m.viewport.Update(msg)
		return m, cmd
	}

	// During responding, still update viewport for scrolling
	m.viewport, _ = m.viewport.Update(msg)
	return m, nil
}

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

// --- Internal methods ---

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
	m.toolLogs = nil
	m.phase = phaseReady
}

// handleStreamEvent processes agent streaming events.
func (m *teaModel) handleStreamEvent(ev agent.StreamEvent) tea.Cmd {
	switch ev.Type {
	case agentctx.StreamText:
		if m.thinking {
			m.thinking = false
			m.phase = phaseStreaming
		}
		m.turnText.WriteString(ev.Text)

	case agentctx.StreamToolStart:
		m.thinking = false
		m.phase = phaseToolCall
		m.stats.ToolCalls++
		m.toolLogs = append(m.toolLogs,
			fmt.Sprintf("  %s %s", styleToolSpin.Render("⟳"), styleDim.Render(ev.ToolName)))

	case agentctx.StreamToolExec:
		m.phase = phaseToolCall
		m.toolLogs = append(m.toolLogs,
			fmt.Sprintf("  %s %s", styleToolExec.Render("⚙"), styleDim.Render(ev.ToolName)))

	case agentctx.StreamToolEnd:
		for i := len(m.toolLogs) - 1; i >= 0; i-- {
			if strings.Contains(m.toolLogs[i], "⟳") {
				toolName := ev.ToolName
				if toolName == "" {
					toolName = "tool"
				}
				m.toolLogs[i] = fmt.Sprintf("  %s %s", styleToolOk.Render("✓"), styleDim.Render(toolName))
				break
			}
		}

	case agentctx.StreamError:
		m.err = ev.Error
		m.responding = false
		m.thinking = false
		m.endTurn("")
		m.input.Focus()

	case agentctx.StreamDone:
		m.responding = false
		m.thinking = false
		m.stats.Turns++
		if ev.Usage != nil {
			m.stats.TokensUsed += ev.Usage.TotalTokens
		}
		if m.agent.Router != nil {
			cost, _, _ := m.agent.Router.SessionCost()
			m.stats.CostUSD = cost
		}
		m.endTurn("")
		m.input.Focus()
	}
	return nil
}

// startAgentTurn launches the agent streaming goroutine.
func (m *teaModel) startAgentTurn(input string) tea.Cmd {
	return func() tea.Msg {
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

		// Wire tool approval — check if approval is needed and pause if so
		m.agent.Stage4.Approver = func(toolName string, args map[string]interface{}) bool {
			if !tools.NeedsApproval(toolName, args) {
				return true
			}
			// Auto-approve in non-interactive mode for now
			// TODO: Phase 3b will make this interactive via tea.Program.Send()
			return true
		}

		m.eventCh = make(chan agent.StreamEvent, 64)
		go m.agent.RunStreaming(turnCtx, convCtx, m.eventCh)
		m.convCtx = convCtx

		ev, ok := <-m.eventCh
		if !ok {
			return streamEventMsg{event: agent.StreamEvent{Type: agentctx.StreamDone}}
		}
		return streamEventMsg{event: ev}
	}
}

// waitForStreamEvent waits for the next streaming event.
func (m *teaModel) waitForStreamEvent() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.eventCh
		if !ok {
			return streamEventMsg{event: agent.StreamEvent{Type: agentctx.StreamDone}}
		}
		return streamEventMsg{event: ev}
	}
}

// thinkingTick sends a spinner animation tick.
func (m *teaModel) thinkingTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return thinkingTickMsg{}
	})
}

// handleCommand processes slash commands.
func (m *teaModel) handleCommand(input string) (bool, tea.Cmd) {
	trimmed := strings.TrimSpace(input)

	switch trimmed {
	case "/quit", "/exit":
		m.persistSession()
		return true, tea.Quit

	case "/clear":
		m.messages = m.messages[:0]
		m.chatHistory = append(m.chatHistory, styleToolOk.Render("✓")+styleDim.Render(" Context cleared"))
		m.chatHistory = append(m.chatHistory, "")
		m.convCtx = nil
		m.err = nil
		return true, nil

	case "/hot":
		m.chatHistory = append(m.chatHistory, m.hotBlock, "")
		return true, nil

	case "/history":
		if len(m.messages) == 0 {
			m.chatHistory = append(m.chatHistory, styleDim.Render("  [No messages in current context]"), "")
		} else {
			for _, msg := range m.messages {
				m.chatHistory = append(m.chatHistory, formatHistoryLine(msg.Role, msg.Content))
			}
			m.chatHistory = append(m.chatHistory, "")
		}
		return true, nil

	case "/sessions":
		if !m.agent.Mnemo.IsConnected() {
			m.chatHistory = append(m.chatHistory, styleDim.Render("  [Not connected to daemon]"), "")
		} else {
			sessions, err := m.agent.Mnemo.ListRecentSessions(m.ctx, 10)
			if err != nil {
				m.chatHistory = append(m.chatHistory, styleError.Render(fmt.Sprintf("Error: %v", err)), "")
			} else if len(sessions) == 0 {
				m.chatHistory = append(m.chatHistory, styleDim.Render("  [No sessions found]"), "")
			} else {
				for _, s := range sessions {
					title := s.Title
					if title == "" {
						title = "(untitled)"
					}
					m.chatHistory = append(m.chatHistory, formatSessionLine(s.ID[:25], title, int(s.MessageCount), s.Source))
				}
				m.chatHistory = append(m.chatHistory, "")
			}
		}
		return true, nil
	}

	if strings.HasPrefix(trimmed, "/resume") {
		id, ok := parseResumeCommand(trimmed)
		if !ok {
			m.chatHistory = append(m.chatHistory, styleError.Render("usage: /resume <session-id>"), "")
			return true, nil
		}
		if !m.agent.Mnemo.IsConnected() {
			m.chatHistory = append(m.chatHistory, styleError.Render("not connected to daemon"), "")
			return true, nil
		}
		detail, err := m.agent.Mnemo.GetSession(m.ctx, id)
		if err != nil {
			m.chatHistory = append(m.chatHistory, styleError.Render(fmt.Sprintf("resume: %v", err)), "")
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
		m.chatHistory = append(m.chatHistory, formatResumedSession(title, len(m.messages)), "")
		return true, nil
	}

	return false, nil
}

// persistSession saves session messages to the daemon.
func (m *teaModel) persistSession() {
	if m.sessionID != "" && len(m.messages) > 0 {
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		msgs := make([]client.Message, 0, len(m.messages))
		for _, msg := range m.messages {
			msgs = append(msgs, client.Message{Role: msg.Role, Content: msg.Content})
		}
		_ = m.agent.Mnemo.AddMessages(persistCtx, m.sessionID, msgs)
	}
}

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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}