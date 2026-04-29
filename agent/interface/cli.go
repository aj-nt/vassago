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

// RunInteractive starts a persistent interactive chat loop with streaming.
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

	// Get hot block once at start
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