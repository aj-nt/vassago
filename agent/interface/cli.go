package iface

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
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

		// Handle Ctrl+C gracefully
		turnCtx, cancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-sigCh:
				fmt.Println(formatInterruptSignal())
				interrupt.Set()
				cancel()
			case <-turnCtx.Done():
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

		// Update message history from the context
		messages = convCtx.Messages

		// Wrap the response in a bordered panel
		if responseText.Len() > 0 {
			fmt.Println()
			fmt.Println(formatResponse(responseText.String(), c.Agent.Config.AgentName))
		}
		fmt.Println()
	}

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
	if responseText.Len() > 0 {
		fmt.Println(formatResponse(responseText.String(), c.Agent.Config.AgentName))
	}
	return nil
}