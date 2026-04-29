package iface

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
)

// CLI provides a terminal interface for the agent.
type CLI struct {
	Agent *agent.Agent
}

// NewCLI creates a new CLI interface.
func NewCLI(a *agent.Agent) *CLI {
	return &CLI{Agent: a}
}

// RunInteractive starts a persistent interactive chat loop with full
// conversation history across turns.
func (c *CLI) RunInteractive(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Mnemosyne Agent — type /quit to exit, /clear to reset context, /hot to show hot block")
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

	// Get hot block once at start (it persists across turns)
	hotBlock, err := c.Agent.GetHotBlock(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get hot block: %v\n", err)
		hotBlock = ""
	}

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
		}

		// Add user message to history
		messages = append(messages, agentctx.Message{Role: "user", Content: input})

		// Build conversation context — fresh iteration counter per user turn
		interrupt := agentctx.NewInterruptSignal()
		convCtx := &agentctx.ConversationContext{
			SessionID:       sessionID,
			AgentID:         c.Agent.Config.AgentID,
			Messages:        messages,
			SystemPrompt:    "",
			Tools:           toolDefs,
			MaxIterations:   c.Agent.Config.MaxTurns,
			Iteration:       0,
			Interrupt:       interrupt,
			HotBlockContent: hotBlock,
			Temperature:     c.Agent.Config.Temperature,
		}

		// Subscribe to events for progress display
		eventCh := c.Agent.EventBus.SubscribeAll()

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

		// Run the agent loop
		result, err := c.Agent.RunWithContext(turnCtx, convCtx)
		cancel()

		// Drain and display events, then unsubscribe
		c.drainEvents(eventCh)
		c.Agent.EventBus.Unsubscribe("*", eventCh)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			// Remove the failed user message so history stays clean
			if len(messages) > 0 && messages[len(messages)-1].Role == "user" {
				messages = messages[:len(messages)-1]
			}
			continue
		}

		// Update message history from the context (includes tool calls, results, etc.)
		messages = convCtx.Messages

		// Display the response
		if result.Response != nil && result.Response.Text != "" {
			fmt.Println(result.Response.Text)
		} else if result.MaxIterationsHit {
			fmt.Fprintf(os.Stderr, "[max iterations reached: %d]\n", result.Iterations)
		}

		if result.Interrupted {
			fmt.Fprintf(os.Stderr, "[interrupted]\n")
		}

		fmt.Println() // Blank line between turns
	}

	return scanner.Err()
}

// drainEvents reads and displays any remaining events from the channel.
func (c *CLI) drainEvents(eventCh <-chan eventbus.Event) {
	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			c.displayEvent(event)
		default:
			return
		}
	}
}

// displayEvent prints a human-readable representation of a pipeline event.
func (c *CLI) displayEvent(event eventbus.Event) {
	switch event.Kind {
	case "tool_call":
		if name, ok := event.Data.(string); ok {
			fmt.Fprintf(os.Stderr, "  → %s\n", name)
		}
	case "tool_result":
		if result, ok := event.Data.(string); ok {
			// Show first 500 chars of tool results
			truncated := result
			if len(truncated) > 500 {
				truncated = truncated[:500] + "..."
			}
			fmt.Fprintf(os.Stderr, "  ← %s\n", truncated)
		}
	}
}

// RunOneShot runs the agent with a single message (piped input or arg).
func (c *CLI) RunOneShot(ctx context.Context, message string) error {
	result, err := c.Agent.Run(ctx, message)
	if err != nil {
		return err
	}
	if result.Response != nil {
		fmt.Println(result.Response.Text)
	}
	return nil
}