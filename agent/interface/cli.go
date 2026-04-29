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
)

// CLI provides a terminal interface for the agent.
type CLI struct {
	Agent *agent.Agent
}

// NewCLI creates a new CLI interface.
func NewCLI(a *agent.Agent) *CLI {
	return &CLI{Agent: a}
}

// RunInteractive starts an interactive chat loop.
func (c *CLI) RunInteractive(ctx context.Context) error {
	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)


	fmt.Println("Mnemosyne Agent \u2014 type /quit to exit")
	fmt.Println()

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
			fmt.Println("Goodbye!")
			return nil
		}

		// Handle Ctrl+C to interrupt the current request
		ctx, cancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-sigCh:
				fmt.Println("\nInterrupting...")
				cancel()
			case <-ctx.Done():
			}
		}()

		// Run the agent
		result, err := c.Agent.Run(ctx, input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			cancel()
			continue
		}

		// Display the response
		if result.Response != nil && result.Response.Text != "" {
			fmt.Println(result.Response.Text)
		}

		// Display metadata
		if result.MaxIterationsHit {
			fmt.Fprintf(os.Stderr, "[max iterations reached: %d]\n", result.Iterations)
		}
		if result.Interrupted {
			fmt.Fprintf(os.Stderr, "[interrupted]\n")
		}

		cancel()
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

