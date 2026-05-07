// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/tools"
	"github.com/aj-nt/mnemosyne/client"
)

// sanitizeUTF8 replaces invalid UTF-8 bytes with the Unicode replacement character.
// This prevents gRPC protobuf marshaling errors and Anthropic API rejections.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString("\ufffd")
		} else {
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// CLI provides a terminal interface for the agent.
type CLI struct {
	Agent *agent.Agent
}

// NewCLI creates a new CLI interface.
func NewCLI(a *agent.Agent) *CLI {
	return &CLI{Agent: a}
}

// RunInteractive starts a persistent interactive chat loop with streaming.
func (c *CLI) RunInteractive(ctx context.Context) error {
	sigCh := setupSignalHandling()
	sessionID := initializeSession(ctx, c)
	hotBlock := getHotBlock(ctx, c)
	displayStartupInfo(c, hotBlock)

	messages := []agentctx.Message{}
	toolDefs := c.Agent.BuildToolDefinitions()
	stats := &SessionStats{}

	scanner := bufio.NewScanner(os.Stdin)
	pasteCollapse := NewPasteCollapse()
	EnableBracketedPaste()
	defer DisableBracketedPaste()

	handleToolApproval(c, scanner)

	for {
		input, isPaste := handleUserInput(scanner, pasteCollapse, c.Agent.Config.AgentName)
		if input == "" && !isPaste {
			break
		}
		if input == "" {
			continue
		}

		if !processCommand(input, c, ctx, &sessionID, &messages, hotBlock) {
			continue
		}

		messages = append(messages, agentctx.Message{Role: "user", Content: input})
		hotBlock = refreshHotBlock(ctx, c, hotBlock)
		compacted := compactMessagesForNextTurn(messages)
		convCtx := buildConversationContext(ctx, c, sessionID, compacted, toolDefs, hotBlock)

		turnCtx, cancel := context.WithCancel(ctx)
			go handleInterrupt(sigCh, turnCtx, cancel, convCtx.Interrupt)

		responseText, toolCalls := streamAgentResponse(turnCtx, c, convCtx, stats)
		cancel()

		updateStats(stats, toolCalls, c)
		messages = convCtx.Messages
		displayResponse(responseText, c.Agent.Config.AgentName, convCtx, c)
	}

	cleanupAndExit(c, ctx, sessionID, messages, stats)
	return scanner.Err()
}

// setupSignalHandling initializes signal handling for interrupt and termination signals.
func setupSignalHandling() chan os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	return sigCh
}

// initializeSession creates a new session if Mnemo is available.
func initializeSession(ctx context.Context, c *CLI) string {
	sessionID, err := c.Agent.Mnemo.CreateSession(ctx, c.Agent.Config.AgentID, "cli", "Interactive session")
	if err != nil {
		fmt.Fprintln(os.Stderr, formatWarning(fmt.Sprintf("could not create session: %v", err)))
	}
	return sessionID
}

// getHotBlock retrieves the hot block content.
func getHotBlock(ctx context.Context, c *CLI) string {
	hotBlock, err := c.Agent.GetHotBlock(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, formatWarning(fmt.Sprintf("could not get hot block: %v", err)))
		return ""
	}
	return sanitizeUTF8(hotBlock)
}

// displayStartupInfo displays the startup information.
func displayStartupInfo(c *CLI, hotBlock string) {
	fmt.Println(formatBanner(c.Agent.Config.AgentName, c.Agent.Config.Personality))
	fmt.Println()
	fmt.Println(formatStartupInfo(0, utf8.RuneCountInString(hotBlock), ""))
	fmt.Println()
	fmt.Println(formatCommandsHelp())
	fmt.Println()
}

// handleUserInput reads and processes user input.
func handleUserInput(scanner *bufio.Scanner, pasteCollapse *PasteCollapse, agentName string) (string, bool) {
	fmt.Print(formatPrompt(agentName))
	input, isPaste := ReadInput(scanner, pasteCollapse)
	return strings.TrimSpace(input), isPaste
}

// processCommand processes user commands.
func processCommand(input string, c *CLI, ctx context.Context, sessionID *string, messages *[]agentctx.Message, hotBlock string) bool {
	switch input {
	case "/quit", "/exit":
		if *sessionID != "" {
			_ = c.Agent.Mnemo.EndSession(ctx, *sessionID)
		}
		fmt.Println(formatGoodbye(&SessionStats{}))
		os.Exit(0)
	case "/clear":
		*messages = (*messages)[:0]
		fmt.Println(formatContextCleared())
		return false
	case "/hot":
		fmt.Println(hotBlock)
		return false
	case "/history":
		displayHistory(*messages)
		return false
	case "/sessions":
		listSessions(ctx, c)
		return false
	}

	if strings.HasPrefix(input, "/resume") {
		resumeSession(ctx, c, sessionID, messages, input)
		return false
	}
	return true
}

// handleToolApproval sets up the tool approval function.
func handleToolApproval(c *CLI, scanner *bufio.Scanner) {
	c.Agent.Stage4.Approver = func(toolName string, args map[string]interface{}) bool {
		if !tools.NeedsApproval(toolName, args) {
			return true
		}
		fmt.Println(formatApprovalRequest(toolName, args))
		fmt.Print("  > ")
		if !scanner.Scan() {
			return false
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
}

// refreshHotBlock refreshes the hot block content.
func refreshHotBlock(ctx context.Context, c *CLI, hotBlock string) string {
	if hb, err := c.Agent.GetHotBlock(ctx); err == nil && hb != "" {
		return hb
	}
	return hotBlock
}

// buildConversationContext builds the conversation context.
func buildConversationContext(ctx context.Context, c *CLI, sessionID string, messages []agentctx.Message, toolDefs []agentctx.ToolDefinition, hotBlock string) *agentctx.ConversationContext {
	interrupt := agentctx.NewInterruptSignal()
	skillBlock := c.Agent.GetSkillBlock(ctx)
	return &agentctx.ConversationContext{
		SessionID:       sessionID,
		AgentID:         c.Agent.Config.AgentID,
		Messages:        messages,
		SystemPrompt:    "",
		Tools:           toolDefs,
		MaxIterations:   c.Agent.Config.MaxTurns,
		Iteration:       0,
		Interrupt:       interrupt,
		HotBlockContent: hotBlock,
		SkillContent:    skillBlock,
		Temperature:     c.Agent.Config.Temperature,
	}
}

// handleInterrupt handles interrupt signals.
func handleInterrupt(sigCh chan os.Signal, turnCtx context.Context, cancel context.CancelFunc, interrupt *agentctx.InterruptSignal) {
	interruptCount := 0
	for {
		select {
		case <-sigCh:
			interruptCount++
			if interruptCount >= 2 {
				fmt.Println("\nForce quit.")
				os.Exit(130)
			}
			fmt.Println(formatInterruptSignal())
			interrupt.Set()
			cancel()
		case <-turnCtx.Done():
			return
		}
	}
}

// streamAgentResponse streams the agent's response.
func streamAgentResponse(turnCtx context.Context, c *CLI, convCtx *agentctx.ConversationContext, stats *SessionStats) (strings.Builder, int) {
	eventCh := make(chan agent.StreamEvent, 64)
	go c.Agent.RunStreaming(turnCtx, convCtx, eventCh)

	var responseText strings.Builder
	thinkingShown := true
	fmt.Print(formatThinkingIndicator())
	toolCalls := 0

	for event := range eventCh {
		switch event.Type {
		case agentctx.StreamText:
			if thinkingShown {
				fmt.Print(formatClearThinking())
				thinkingShown = false
			}
			fmt.Print(event.Text)
			responseText.WriteString(event.Text)
		case agentctx.StreamToolStart:
			if thinkingShown {
				fmt.Print(formatClearThinking())
				thinkingShown = false
			}
			toolCalls++
			fmt.Println(formatToolStart(event.ToolName))
		case agentctx.StreamToolEnd:
			// Tool_use block finished parsing
		case agentctx.StreamToolExec:
			fmt.Println(formatToolExec(event.ToolName))
		case agentctx.StreamDone:
			if event.Usage != nil {
				stats.TokensUsed += event.Usage.TotalTokens
			}
		case agentctx.StreamError:
			if event.Error != nil {
				fmt.Fprintln(os.Stderr, formatError(event.Error))
			}
		default:
			if event.Error != nil {
				fmt.Fprintln(os.Stderr, formatError(event.Error))
			}
		}
	}
	return responseText, toolCalls
}

// updateStats updates the session statistics.
func updateStats(stats *SessionStats, toolCalls int, c *CLI) {
	stats.Turns++
	stats.ToolCalls += toolCalls
	if c.Agent.Router != nil {
		cost, _, _ := c.Agent.Router.SessionCost()
		stats.CostUSD = cost
	}
}

// displayResponse displays the agent's response.
// In streaming mode, text was already printed in real-time via fmt.Print(event.Text).
// We do NOT erase and redraw — the line count estimation is unreliable (tool call
// indicators take lines too, word-wrap on narrow terminals adds lines, and GLM's
// fragmented streaming produces partial lines). Instead, we just add a newline to
// separate the response from the prompt, then show context usage.
func displayResponse(responseText strings.Builder, agentName string, convCtx *agentctx.ConversationContext, c *CLI) {
	// Text was already streamed to the terminal — just add spacing
	if responseText.Len() > 0 {
		fmt.Println()
	}

	tokenCount := convCtx.TokenCount()
	contextStr := formatContextUsage(tokenCount, c.Agent.Config.MaxContextTokens)
	if contextStr != "" {
		fmt.Println(colorize(dim, "  📊 "+contextStr))
	}
	fmt.Println()
}

// cleanupAndExit ensures the session is persisted and displays goodbye message.
func cleanupAndExit(c *CLI, ctx context.Context, sessionID string, messages []agentctx.Message, stats *SessionStats) {
	if sessionID != "" && len(messages) > 0 {
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		msgs := make([]client.Message, 0, len(messages))
		for _, m := range messages {
			msgs = append(msgs, client.Message{Role: m.Role, Content: m.Content})
		}
		if err := c.Agent.Mnemo.AddMessages(persistCtx, sessionID, msgs); err != nil {
			fmt.Fprintln(os.Stderr, formatWarning(fmt.Sprintf("session flush failed: %v", err)))
		}
	}
	fmt.Println(formatGoodbye(stats))
}

// displayHistory displays the message history.
func displayHistory(messages []agentctx.Message) {
	if len(messages) == 0 {
		fmt.Println(colorize(dim, "  [No messages in current context]"))
	} else {
		for _, m := range messages {
			fmt.Println(formatHistoryLine(m.Role, m.Content))
		}
	}
}

// listSessions lists recent sessions.
func listSessions(ctx context.Context, c *CLI) {
	if !c.Agent.Mnemo.IsConnected() {
		fmt.Println(colorize(dim, "  [Not connected to daemon]"))
		return
	}
	sessions, err := c.Agent.Mnemo.ListRecentSessions(ctx, 10)
	if err != nil {
		fmt.Fprintln(os.Stderr, formatError(fmt.Errorf("listing sessions: %w", err)))
	} else if len(sessions) == 0 {
		fmt.Println(colorize(dim, "  [No sessions found]"))
	} else {
		for _, s := range sessions {
			title := s.Title
			if title == "" {
				title = "(untitled)"
			}
			fmt.Println(formatSessionLine(s.ID[:25], title, int(s.MessageCount), s.Source))
		}
	}
}

// resumeSession resumes a previous session.
func resumeSession(ctx context.Context, c *CLI, sessionID *string, messages *[]agentctx.Message, input string) {
	sessionIDArg, ok := parseResumeCommand(input)
	if !ok {
		fmt.Fprintln(os.Stderr, formatError(fmt.Errorf("usage: /resume <session-id>")))
		return
	}
	if !c.Agent.Mnemo.IsConnected() {
		fmt.Fprintln(os.Stderr, formatError(fmt.Errorf("not connected to daemon")))
		return
	}
	detail, err := c.Agent.Mnemo.GetSession(ctx, sessionIDArg)
	if err != nil {
		fmt.Fprintln(os.Stderr, formatError(fmt.Errorf("resume: %w", err)))
		return
	}
	resumedMsgs := make([]agentctx.Message, 0, len(detail.Messages))
	for _, m := range detail.Messages {
		resumedMsgs = append(resumedMsgs, agentctx.Message{Role: m.Role, Content: m.Content})
	}
	*messages = resumedMsgs
	*sessionID = sessionIDArg
	title := detail.Title
	if title == "" {
		title = "(untitled)"
	}
	fmt.Println(formatResumedSession(title, len(*messages)))
}

// RunOneShot runs the agent with a single message, streaming the output.
func (c *CLI) RunOneShot(ctx context.Context, message string) error {
	// Get hot block
	hotBlock, err := c.Agent.GetHotBlock(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, formatWarning(fmt.Sprintf("could not get hot block: %v", err)))
		hotBlock = ""
	}

	// Create session
	sessionID, _ := c.Agent.Mnemo.CreateSession(ctx, c.Agent.Config.AgentID, "cli", "")

	// Build context
	skillBlock := c.Agent.GetSkillBlock(ctx)
	convCtx := &agentctx.ConversationContext{
		SessionID:       sessionID,
		AgentID:         c.Agent.Config.AgentID,
		Messages:        []agentctx.Message{{Role: "user", Content: message}},
		SystemPrompt:    "",
		Tools:           c.Agent.BuildToolDefinitions(),
		MaxIterations:   c.Agent.Config.MaxTurns,
		Iteration:       0,
		Interrupt:       agentctx.NewInterruptSignal(),
		HotBlockContent: hotBlock,
		SkillContent:    skillBlock,
		Temperature:     c.Agent.Config.Temperature,
	}

	// Stream the response
	eventCh := make(chan agent.StreamEvent, 64)
	go c.Agent.RunStreaming(ctx, convCtx, eventCh)

	var responseText strings.Builder
	thinkingShown := true
	fmt.Print(formatThinkingIndicator())

	for event := range eventCh {
		switch event.Type {
		case agentctx.StreamText:
			if thinkingShown {
				fmt.Print(formatClearThinking())
				thinkingShown = false
			}
			fmt.Print(event.Text)
			responseText.WriteString(event.Text)
		case agentctx.StreamToolStart:
			if thinkingShown {
				fmt.Print(formatClearThinking())
				thinkingShown = false
			}
			fmt.Println(formatToolStart(event.ToolName))
		case agentctx.StreamToolEnd:
			// Tool_use block finished parsing
		case agentctx.StreamToolExec:
			fmt.Println(formatToolExec(event.ToolName))
		case agentctx.StreamDone:
			// done
		case agentctx.StreamError:
			if event.Error != nil {
				return event.Error
			}
		default:
			if event.Error != nil {
				return event.Error
			}
		}
	}

	fmt.Println()
	// In single-query mode, text was already streamed.
	// For color terminals, show a clean boxed panel by erasing streamed text.
	// We add a generous buffer to the line count to account for tool call
	// indicators and word-wrap that countLines doesn't track.
	if responseText.Len() > 0 && useColor() {
		lines := countLines(responseText.String())
		// Add 3 extra lines as buffer for tool indicators, wrapping, etc.
		if lines > 0 {
			fmt.Print(eraseLines(lines + 3))
		}
		fmt.Println(formatResponse(responseText.String(), c.Agent.Config.AgentName))
	} else if responseText.Len() > 0 {
		// No color: text already streamed, nothing more to do
	}
	return nil
}