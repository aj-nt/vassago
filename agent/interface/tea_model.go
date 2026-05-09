// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// styles — lipgloss replaces raw ANSI codes
var (
	styleAgentName = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")) // bright cyan

	styleDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // gray

	stylePrompt = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")). // bright cyan
			SetString("❯ ")

	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")). // bright red
			Bold(true)

	styleWarning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")) // bright yellow

	styleToolStart = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")). // cyan
			Italic(true)

	styleToolEnd = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")) // bright green

	styleToolExec = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")). // bright yellow
			Italic(true)

	styleToolError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")). // bright red
			Bold(true)

	styleContextOK  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleContextWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleContextCrit = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	styleThinking = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")). // gray
			Italic(true)

	styleHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")). // bright cyan
			Bold(true)

	styleBorder = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // gray for borders

	styleResponseLabel = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("12")) // bright cyan
)

// teaModel is the Bubble Tea model for the interactive chat TUI.
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
	output     strings.Builder // accumulated streaming response
	spinner    int
	thinking   bool
	responding bool // true while agent is streaming
	err        error

	// Channels
	eventCh chan agent.StreamEvent

	// Width for layout
	width int
}

// streamTickMsg is sent when a streaming event arrives.
type streamTickMsg struct{ event agent.StreamEvent }

// thinkingTickMsg is sent on a timer to animate the thinking indicator.
type thinkingTickMsg struct{}

// agentResponseDoneMsg is sent when the agent finishes responding.
type agentResponseDoneMsg struct {
	text      strings.Builder
	toolCalls int
	ctx       *agentctx.ConversationContext
}

// RunInteractiveBubble starts the interactive chat using Bubble Tea.
func RunInteractiveBubble(ctx context.Context, a *agent.Agent) error {
	// Initialize text input
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Prompt = "❯ "
	ti.PromptStyle = stylePrompt
	ti.CharLimit = 0 // unlimited
	ti.Width = 70

	// Create session
	sessionID, err := a.Mnemo.CreateSession(ctx, a.Config.AgentID, "cli", "Interactive session")
	if err != nil {
		// Non-fatal — continue without session persistence
		sessionID = ""
	}

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
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m teaModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink, // start cursor blinking
		func() tea.Msg { return nil }, // no-op initial command
	)
}

func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.input.Width = max(msg.Width-4, 20)
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.responding {
				// Interrupt the current response
				// TODO: wire interrupt signal
				m.responding = false
				m.thinking = false
				return m, nil
			}
			// Exit
			return m, tea.Quit

		case tea.KeyEnter:
			if m.responding {
				return m, nil // ignore input while responding
			}
			input := m.input.Value()
			if input == "" {
				return m, nil
			}

			// Handle commands
			handled, cmd := m.handleCommand(input)
			if handled {
				m.input.SetValue("")
				return m, cmd
			}

			// Send message to agent
			m.messages = append(m.messages, agentctx.Message{Role: "user", Content: input})
			m.input.SetValue("")
			m.responding = true
			m.thinking = true
			m.output.Reset()
			m.err = nil

			return m, tea.Batch(
				m.startAgentResponse(input),
				m.thinkingTick(),
			)
		}

	case streamTickMsg:
		ev := msg.event
		switch ev.Type {
		case agentctx.StreamText:
			if m.thinking {
				m.thinking = false
			}
			m.output.WriteString(ev.Text)
			return m, nil

		case agentctx.StreamToolStart:
			m.thinking = false
			m.stats.ToolCalls++
			m.output.WriteString(fmt.Sprintf("\n  %s %s\n", styleToolStart.Render("⟳"), styleDim.Render(ev.ToolName)))

		case agentctx.StreamToolExec:
			m.output.WriteString(fmt.Sprintf("  %s %s\n", styleToolExec.Render("⚙"), styleDim.Render(ev.ToolName)))

		case agentctx.StreamToolEnd:
			// tool call block finished parsing — no visual action needed

		case agentctx.StreamError:
			m.err = ev.Error
			m.responding = false
			m.thinking = false

		case agentctx.StreamDone:
			m.responding = false
			if ev.Usage != nil {
				m.stats.TokensUsed += ev.Usage.TotalTokens
			}
		}

		// If still responding, keep listening
		if m.responding {
			return m, m.waitForStreamEvent()
		}

		// Agent finished — update stats and show context usage
		m.stats.Turns++
		if m.agent.Router != nil {
			cost, _, _ := m.agent.Router.SessionCost()
			m.stats.CostUSD = cost
		}
		return m, nil

	case thinkingTickMsg:
		if m.thinking {
			m.spinner++
			return m, m.thinkingTick()
		}
		return m, nil

	case agentResponseDoneMsg:
		// Legacy path — not used in streaming mode, but kept for compatibility
		m.messages = msg.ctx.Messages
		m.responding = false
		m.thinking = false
		m.stats.Turns++
		if m.agent.Router != nil {
			cost, _, _ := m.agent.Router.SessionCost()
			m.stats.CostUSD = cost
		}
		return m, nil
	}

	// Default: update text input
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m teaModel) View() string {
	var b strings.Builder

	// Show thinking indicator or agent response
	if m.thinking {
		spinFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		b.WriteString(styleThinking.Render(fmt.Sprintf("  ⚕ Thinking %s", spinFrames[m.spinner%len(spinFrames)])))
		b.WriteString("\n")
	} else if m.responding {
		// Show streaming output
		b.WriteString(m.output.String())
	} else if m.output.Len() > 0 {
		// Show completed response with context usage
		b.WriteString(m.output.String())
		b.WriteString("\n")
		tokenCount := m.tokenCount()
		ctxStr := m.formatContextUsage(tokenCount)
		if ctxStr != "" {
			b.WriteString(styleDim.Render("  📊 " + ctxStr))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		m.output.Reset() // clear after displaying
	}

	// Error display
	if m.err != nil {
		b.WriteString(styleError.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
		m.err = nil
	}

	// Input
	b.WriteString(m.input.View())

	return b.String()
}

// handleCommand processes slash commands. Returns (handled=true, cmd=nil) if input was a command.
func (m *teaModel) handleCommand(input string) (bool, tea.Cmd) {
	switch strings.TrimSpace(input) {
	case "/quit", "/exit":
		return true, tea.Quit
	case "/clear":
		m.messages = m.messages[:0]
		m.output.Reset()
		m.output.WriteString(styleContextOK.Render("✓") + styleDim.Render(" Context cleared\n"))
		return true, nil
	case "/hot":
		m.output.Reset()
		m.output.WriteString(m.hotBlock + "\n")
		return true, nil
	case "/history":
		m.output.Reset()
		if len(m.messages) == 0 {
			m.output.WriteString(styleDim.Render("  [No messages in current context]\n"))
		} else {
			for _, msg := range m.messages {
				m.output.WriteString(formatHistoryLine(msg.Role, msg.Content) + "\n")
			}
		}
		return true, nil
	case "/sessions":
		m.output.Reset()
		if !m.agent.Mnemo.IsConnected() {
			m.output.WriteString(styleDim.Render("  [Not connected to daemon]\n"))
		} else {
			sessions, err := m.agent.Mnemo.ListRecentSessions(m.ctx, 10)
			if err != nil {
				m.output.WriteString(styleError.Render(fmt.Sprintf("Error listing sessions: %v\n", err)))
			} else if len(sessions) == 0 {
				m.output.WriteString(styleDim.Render("  [No sessions found]\n"))
			} else {
				for _, s := range sessions {
					title := s.Title
					if title == "" {
						title = "(untitled)"
					}
					m.output.WriteString(formatSessionLine(s.ID[:25], title, int(s.MessageCount), s.Source) + "\n")
				}
			}
		}
		return true, nil
	}

	if strings.HasPrefix(input, "/resume") {
		id, ok := parseResumeCommand(input)
		if !ok {
			m.output.WriteString(styleError.Render("usage: /resume <session-id>\n"))
			return true, nil
		}
		if !m.agent.Mnemo.IsConnected() {
			m.output.WriteString(styleError.Render("not connected to daemon\n"))
			return true, nil
		}
		detail, err := m.agent.Mnemo.GetSession(m.ctx, id)
		if err != nil {
			m.output.WriteString(styleError.Render(fmt.Sprintf("resume: %v\n", err)))
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
		m.output.WriteString(formatResumedSession(title, len(m.messages)) + "\n")
		return true, nil
	}

	return false, nil
}

// startAgentResponse launches the agent streaming goroutine and returns
// a command to wait for the first stream event.
func (m *teaModel) startAgentResponse(input string) tea.Cmd {
	return func() tea.Msg {
		// Refresh hot block
		if hb, err := m.agent.GetHotBlock(m.ctx); err == nil && hb != "" {
			m.hotBlock = sanitizeUTF8(hb)
		}

		compacted := compactMessagesForNextTurn(m.messages)
		convCtx := &agentctx.ConversationContext{
			SessionID:       m.sessionID,
			AgentID:         m.agent.Config.AgentID,
			Messages:        compacted,
			SystemPrompt:    "",
			Tools:           m.toolDefs,
			MaxIterations:   m.agent.Config.MaxTurns,
			Iteration:       0,
			Interrupt:       agentctx.NewInterruptSignal(),
			HotBlockContent: m.hotBlock,
			SkillContent:     m.agent.GetSkillBlock(m.ctx),
			Temperature:     m.agent.Config.Temperature,
		}

		// Wire tool approval
		// TODO: need async tool approval in Bubble Tea model

		go m.agent.RunStreaming(m.ctx, convCtx, m.eventCh)

		// Wait for first event
		ev, ok := <-m.eventCh
		if !ok {
			return streamTickMsg{event: agent.StreamEvent{Type: agentctx.StreamDone}}
		}
		return streamTickMsg{event: ev}
	}
}

// waitForStreamEvent returns a command that waits for the next stream event.
func (m *teaModel) waitForStreamEvent() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.eventCh
		if !ok {
			return streamTickMsg{event: agent.StreamEvent{Type: agentctx.StreamDone}}
		}
		return streamTickMsg{event: ev}
	}
}

// thinkingTick returns a command that sends a thinkingTickMsg after 100ms.
func (m *teaModel) thinkingTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return thinkingTickMsg{}
	})
}

// tokenCount estimates tokens for context usage display.
func (m *teaModel) tokenCount() int {
	total := len(m.agent.Config.Personality) // rough: include system prompt
	for _, msg := range m.messages {
		total += len(msg.Content)
	}
	return total / 4
}

// formatContextUsage shows context token usage with color coding.
func (m *teaModel) formatContextUsage(used int) string {
	max := m.agent.Config.MaxContextTokens
	if max == 0 {
		return ""
	}
	pct := float64(used) / float64(max)
	usedStr := humanizeTokensCompact(used)
	maxStr := humanizeTokensCompact(max)

	var style lipgloss.Style
	switch {
	case pct > 0.8:
		style = styleContextCrit
	case pct > 0.6:
		style = styleContextWarn
	default:
		style = styleContextOK
	}
	return style.Render(fmt.Sprintf("%s/%s tokens", usedStr, maxStr))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}