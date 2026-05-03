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
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// --- Startup display ---
	fmt.Println(formatBanner(c.Agent.Config.AgentName, c.Agent.Config.Personality))
	fmt.Println()

	// Start session
	var sessionID string
	if c.Agent.Mnemo != nil {
		var err error
		sessionID, err = c.Agent.Mnemo.CreateSession(ctx, c.Agent.Config.AgentID, "cli", "Interactive session")
		if err != nil {
			fmt.Fprintln(os.Stderr, formatWarning(fmt.Sprintf("could not create session: %v", err)))
		}
	}

	// Get hot block — refresh on each turn
	hotBlock, err := c.Agent.GetHotBlock(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, formatWarning(fmt.Sprintf("could not get hot block: %v", err)))
		hotBlock = ""
	}
	hotBlock = sanitizeUTF8(hotBlock)

	// Show startup info
	fmt.Println(formatStartupInfo(0, utf8.RuneCountInString(hotBlock), ""))
	fmt.Println()
	fmt.Println(formatCommandsHelp())
	fmt.Println()

	// Persistent conversation context across turns
	messages := []agentctx.Message{}
	toolDefs := c.Agent.BuildToolDefinitions()
	stats := &SessionStats{}

	scanner := bufio.NewScanner(os.Stdin)

	// Wire approval flow for dangerous tools
	c.Agent.Stage4.Approver = func(toolName string, args map[string]interface{}) bool {
		if !tools.IsApprovalRequired(toolName) {
			return true // No approval needed
		}
		// Prompt the user
		fmt.Println(formatApprovalRequest(toolName, args))
		fmt.Print("  > ")
		if !scanner.Scan() {
			return false
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}

	for {
		fmt.Print(formatPrompt(c.Agent.Config.AgentName))
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case "/quit", "/exit":
			if c.Agent.Mnemo != nil && sessionID != "" {
				_ = c.Agent.Mnemo.EndSession(ctx, sessionID)
			}
			fmt.Println(formatGoodbye(stats))
			return nil
		case "/clear":
			messages = messages[:0]
			fmt.Println(formatContextCleared())
			continue
		case "/hot":
			fmt.Println(hotBlock)
			continue
		case "/history":
			if len(messages) == 0 {
				fmt.Println(colorize(dim, "  [No messages in current context]"))
			} else {
				for _, m := range messages {
					fmt.Println(formatHistoryLine(m.Role, m.Content))
				}
			}
			continue
		case "/sessions":
			if c.Agent.Mnemo != nil {
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
			} else {
				fmt.Println(colorize(dim, "  [Not connected to daemon]"))
			}
			continue
		}

	// /resume <session-id> — restore a previous session's messages
	if strings.HasPrefix(input, "/resume") {
		sessionIDArg, ok := parseResumeCommand(input)
		if !ok {
			fmt.Fprintln(os.Stderr, formatError(fmt.Errorf("usage: /resume <session-id>")))
			continue
		}
		if c.Agent.Mnemo == nil {
			fmt.Fprintln(os.Stderr, formatError(fmt.Errorf("not connected to daemon")))
			continue
		}
		detail, err := c.Agent.Mnemo.GetSession(ctx, sessionIDArg)
		if err != nil {
			fmt.Fprintln(os.Stderr, formatError(fmt.Errorf("resume: %w", err)))
			continue
		}
		// Load messages from the session
		resumedMsgs := make([]agentctx.Message, 0, len(detail.Messages))
		for _, m := range detail.Messages {
			resumedMsgs = append(resumedMsgs, agentctx.Message{Role: m.Role, Content: m.Content})
		}
		messages = resumedMsgs
		// Update session ID to the resumed session
		sessionID = sessionIDArg
		title := detail.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Println(formatResumedSession(title, len(messages)))
		continue
	}

		// Add user message to history
		messages = append(messages, agentctx.Message{Role: "user", Content: input})

		// Refresh hot block each turn
		if hb, err := c.Agent.GetHotBlock(ctx); err == nil && hb != "" {
			hotBlock = hb
		}

		// Compact messages: strip old tool call/result pairs from prior turns
		compacted := compactMessagesForNextTurn(messages)

		// Build conversation context
		interrupt := agentctx.NewInterruptSignal()
		convCtx := &agentctx.ConversationContext{
			SessionID:       sessionID,
			AgentID:         c.Agent.Config.AgentID,
			Messages:        compacted,
			SystemPrompt:    "",
			Tools:           toolDefs,
			MaxIterations:   c.Agent.Config.MaxTurns,
			Iteration:       0,
			Interrupt:       interrupt,
			HotBlockContent: hotBlock,
			Temperature:     c.Agent.Config.Temperature,
		}

		// Handle Ctrl+C gracefully — first press interrupts the agent,
		// second press within 2 seconds force-quits the process.
		turnCtx, cancel := context.WithCancel(ctx)
		go func() {
			interruptCount := 0
			for {
				select {
				case <-sigCh:
					interruptCount++
					if interruptCount >= 2 {
						// Double Ctrl+C — force quit
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
		}()

		// --- Streaming display ---
		eventCh := make(chan agent.StreamEvent, 64)
		go c.Agent.RunStreaming(turnCtx, convCtx, eventCh)

		var responseText strings.Builder
		thinkingShown := true // Show thinking indicator immediately
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
				// Tool_use block finished parsing — dispatch comes after

			case agentctx.StreamToolExec:
				// Agent is dispatching the tool now
				fmt.Println(formatToolExec(event.ToolName))

			case agentctx.StreamDone:
				// Stream complete — record usage
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
		cancel()

		stats.Turns++
		stats.ToolCalls += toolCalls

		// Track cost from router
		if c.Agent.Router != nil {
			cost, _, _ := c.Agent.Router.SessionCost()
			stats.CostUSD = cost
		}

		// Update message history from the context
		messages = convCtx.Messages

		// Replace the streamed text with a bordered panel.
		// The streaming text is already on screen. We erase it and redraw
		// as a clean panel. This avoids the double-display bug where both
		// the raw streamed text AND the panel appear.
		if responseText.Len() > 0 && useColor() {
			// Count lines in the streamed response to erase them
			lines := countLines(responseText.String())
			if lines > 0 {
				fmt.Print(eraseLines(lines))
			}
			fmt.Println(formatResponse(responseText.String(), c.Agent.Config.AgentName))
		} else if responseText.Len() > 0 {
			// No color mode: just print the text (already streamed)
			fmt.Println()
		}

		// Show context usage
		tokenCount := convCtx.TokenCount()
		contextStr := formatContextUsage(tokenCount, c.Agent.Config.MaxContextTokens)
		if contextStr != "" {
			fmt.Println(colorize(dim, "  📊 "+contextStr))
		}
		fmt.Println()
	}

	// Ensure session is persisted on exit
	defer func() {
		if c.Agent.Mnemo != nil && sessionID != "" && len(messages) > 0 {
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
	}()

	return scanner.Err()
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
	var sessionID string
	if c.Agent.Mnemo != nil {
		sessionID, _ = c.Agent.Mnemo.CreateSession(ctx, c.Agent.Config.AgentID, "cli", "")
	}

	// Build context
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
	if responseText.Len() > 0 && useColor() {
		// In single-query mode, text was already streamed. Erase it and show a clean panel.
		lines := countLines(responseText.String())
		if lines > 0 {
			fmt.Print(eraseLines(lines))
		}
		fmt.Println(formatResponse(responseText.String(), c.Agent.Config.AgentName))
	} else if responseText.Len() > 0 {
		// No color: text already streamed
	}
	return nil
}