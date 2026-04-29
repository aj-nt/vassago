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

	fmt.Println("Mnemosyne Agent — type /quit to exit, /clear to reset context")
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

	// Persistent conversation context
	messages := []agentctx.Message{}

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
		if input == "/quit" || input == "/exit" {
			// End session
			if c.Agent.Mnemo != nil && sessionID != "" {
				c.Agent.Mnemo.EndSession(ctx, sessionID)
			}
			fmt.Println("Goodbye!")
			return nil
		}
		if input == "/clear" {
			messages = messages[:0]
			fmt.Println("[Context cleared]")
			continue
		}
		if input == "/hot" {
			fmt.Println(hotBlock)
			continue
		}

		// Add user message to history
		messages = append(messages, agentctx.Message{Role: "user", Content: input})

		// Build conversation context
		interrupt := agentctx.NewInterruptSignal()
		convCtx := &agentctx.ConversationContext{
			SessionID:       sessionID,
			AgentID:         c.Agent.Config.AgentID,
			Messages:        messages,
			SystemPrompt:    "",
			Tools:           c.Agent.BuildToolDefinitions(),
			MaxIterations:   c.Agent.Config.MaxTurns,
			Iteration:       0,
			Interrupt:       interrupt,
			HotBlockContent: hotBlock,
		}

		// Run the agent loop with per-turn context
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

		result, err := c.Agent.RunWithContext(turnCtx, convCtx)
		cancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		// Update message history from the context (includes tool calls, etc.)
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
	}

	return scanner.Err()
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