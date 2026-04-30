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

	fmt.Println("Mnemosyne Agent — type /quit to exit, /clear to reset context, /hot to show hot block, /history to show context, /sessions to list recent sessions")
	fmt.Println()

	// Start a session with the daemon
	var sessionID string
	if c.Agent.Mnemo != nil {
		var err error
		sessionID, err = c.Agent.Mnemo.CreateSession(ctx, c.Agent.Config.AgentID, "cli", "Interactive session")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create session: %v\n", err)
		}
	}

	// Get hot block once at start — refresh on each turn
	hotBlock, err := c.Agent.GetHotBlock(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get hot block: %v\n", err)
		hotBlock = ""
	}
	// Sanitize UTF-8 on read — belt and suspenders with the daemon-side fix (#17)
	hotBlock = sanitizeUTF8(hotBlock)

	// Persistent conversation context across turns
	messages := []agentctx.Message{}
	toolDefs := c.Agent.BuildToolDefinitions()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
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
			fmt.Println("Goodbye!")
			return nil
		case "/clear":
			messages = messages[:0]
			fmt.Println("[Context cleared]")
			continue
		case "/hot":
			fmt.Println(hotBlock)
			continue
		case "/history":
			// Show current conversation history
			if len(messages) == 0 {
				fmt.Println("[No messages in current context]")
			} else {
				for _, m := range messages {
					var preview string
					if len(m.Content) > 100 {
						preview = m.Content[:100] + "..."
					} else {
						preview = m.Content
					}
					fmt.Printf("  [%s] %s\n", m.Role, preview)
				}
			}
			continue
		case "/sessions":
			// List recent sessions from the daemon
			if c.Agent.Mnemo != nil {
				sessions, err := c.Agent.Mnemo.ListRecentSessions(ctx, 10)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
				} else if len(sessions) == 0 {
					fmt.Println("[No sessions found]")
				} else {
					for _, s := range sessions {
						title := s.Title
						if title == "" {
							title = "(untitled)"
						}
						fmt.Printf("  %-25s  %-20s  %3d msg  [%s]\n", s.ID[:25], title, s.MessageCount, s.Source)
					}
				}
			} else {
				fmt.Println("[Not connected to daemon]")
			}
			continue
		}

		// Add user message to history
		messages = append(messages, agentctx.Message{Role: "user", Content: input})

		// Refresh hot block each turn — memory may have been updated
		if hb, err := c.Agent.GetHotBlock(ctx); err == nil && hb != "" {
			hotBlock = hb
		}

		// Compact messages: strip old tool call/result pairs from prior turns.
		// Tool results are needed within the current agentic loop but cause
		// repetition when sent on subsequent user turns (the model re-reads
		// prior tool outputs and re-states the facts from them).
		compacted := compactMessagesForNextTurn(messages)

		// Build conversation context — fresh iteration counter per user turn
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
				fmt.Println("\nInterrupting...")
				interrupt.Set()
				cancel()
			case <-turnCtx.Done():
			}
		}()

		// Run streaming!
		eventCh := make(chan agent.StreamEvent, 64)
		go c.Agent.RunStreaming(turnCtx, convCtx, eventCh)

		// Display streaming events
		for event := range eventCh {
			switch event.Type {
			case agentctx.StreamText:
				fmt.Print(event.Text)
			case agentctx.StreamToolStart:
				fmt.Fprintf(os.Stderr, "  → %s\n", event.ToolName)
			case agentctx.StreamDone:
				// Stream complete
			default:
				if event.Error != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", event.Error)
				}
			}
		}
		cancel()

		// Update message history from the context (includes tool calls, results, etc.)
		messages = convCtx.Messages
		fmt.Println()
		fmt.Println()
	}

	return scanner.Err()
}

// RunOneShot runs the agent with a single message, streaming the output.
func (c *CLI) RunOneShot(ctx context.Context, message string) error {
	// Get hot block
	hotBlock, err := c.Agent.GetHotBlock(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get hot block: %v\n", err)
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

	for event := range eventCh {
		switch event.Type {
		case agentctx.StreamText:
			fmt.Print(event.Text)
		case agentctx.StreamToolStart:
			fmt.Fprintf(os.Stderr, "  → %s\n", event.ToolName)
		case agentctx.StreamDone:
			// done
		default:
			if event.Error != nil {
				return event.Error
			}
		}
	}
	fmt.Println()
	return nil
}