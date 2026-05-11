// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// --- Phases ---

type phase int

const (
	phaseReady     phase = iota // waiting for user input
	phaseThinking               // agent thinking, spinner shown
	phaseStreaming              // agent streaming text response
	phaseToolCall               // agent executing tool calls
	phaseApproval               // waiting for user approval of a tool call
)

// --- Messages ---

type streamEventMsg struct{ event agent.StreamEvent }
type thinkingTickMsg struct{}

// approvalRequestMsg is sent when a tool needs user approval.
// The TUI transitions to phaseApproval and waits for the user's response.
type approvalRequestMsg struct {
	toolName string
	args     map[string]interface{}
}

// oneShotMsg is sent by Init to auto-start a one-shot message.
type oneShotMsg struct{ message string }

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
	turnText      strings.Builder
	reasoningText strings.Builder
	toolLogs      []string

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

	// Approval: the Approver callback (running in the streaming goroutine)
	// sends approvalRequestMsg via p.Send(), then blocks on approvalCh.
	// The TUI's Update() handles approvalRequestMsg, prompts the user,
	// and sends the result to approvalCh to unblock the Approver.
	program                 *tea.Program
	approvalCh              chan bool
	pendingApprovalToolName string
	pendingApprovalArgs     map[string]interface{}

	// One-shot mode: auto-send a message and quit after response
	oneShot    bool
	oneShotMsg string
}

// --- Constructors ---

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
		agent:      a,
		ctx:        ctx,
		sessionID:  sessionID,
		toolDefs:   a.BuildToolDefinitions(),
		hotBlock:   sanitizeUTF8(hotBlock),
		input:      ta,
		viewport:   vp,
		eventCh:    make(chan agent.StreamEvent, 64),
		approvalCh: make(chan bool, 1),
		width:      80,
		height:     24,
		phase:      phaseReady,
		interrupt:  agentctx.NewInterruptSignal(),
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

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p // store for p.Send() in Approver callback
	_, err := p.Run()
	return err
}

// RunOneShotBubble runs a single query using the Bubble Tea TUI, then exits.
// It shows the response in a scrollable viewport and quits when done.
func RunOneShotBubble(ctx context.Context, a *agent.Agent, message string) error {
	ta := textarea.New()
	ta.Placeholder = "Sending..."
	ta.Prompt = "❯ "
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.SetWidth(70)
	ta.SetHeight(1)

	sessionID, _ := a.Mnemo.CreateSession(ctx, a.Config.AgentID, "cli", "One-shot query")
	hotBlock, _ := a.GetHotBlock(ctx)

	vp := viewport.New(80, 20)

	m := teaModel{
		agent:      a,
		ctx:        ctx,
		sessionID:  sessionID,
		toolDefs:   a.BuildToolDefinitions(),
		hotBlock:   sanitizeUTF8(hotBlock),
		input:      ta,
		viewport:   vp,
		eventCh:    make(chan agent.StreamEvent, 64),
		approvalCh: make(chan bool, 1),
		width:      80,
		height:     24,
		phase:      phaseReady,
		interrupt:  agentctx.NewInterruptSignal(),
		historyIdx: -1,
		oneShot:    true,
		oneShotMsg: message,
	}

	// Show minimal startup info for one-shot
	banner := fmt.Sprintf("  ⚕ %s\n", a.Config.AgentName)
	banner += formatStartupInfo(0, utf8.RuneCountInString(m.hotBlock), "")
	banner += "\n"
	m.chatHistory = []string{banner}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p
	_, err := p.Run()
	return err
}

// --- tea.Model interface (Init) ---

func (m teaModel) Init() tea.Cmd {
	if m.oneShot {
		// One-shot mode: auto-send the message immediately
		return func() tea.Msg {
			return oneShotMsg{message: m.oneShotMsg}
		}
	}
	return textarea.Blink
}
