// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"bufio"
	"context"
	"flag"
	"log/slog"
	"os"
	"strings"

	agentlog "github.com/aj-nt/mnemosyne/agent/log"
	"github.com/aj-nt/mnemosyne/agent/setup"
	iface "github.com/aj-nt/mnemosyne/agent/interface"
)

func main() {
	// Phase 33: JSON structured logging to stderr
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	configPath := flag.String("config", "configs/agent-dev.yaml", "Path to agent config file")
	oneShot := flag.String("message", "", "Single message (non-interactive)")
	serve := flag.Bool("serve", false, "Server mode: start gRPC chat server and wait (no interactive CLI)")
	flag.Parse()

	ctx := context.Background()

	result, err := setup.InitAgent(ctx, *configPath)
	if err != nil {
		agentlog.Fatalf("Failed to initialize agent: %v", err)
	}
	defer result.Close()

	// Run
	if *serve {
		// Server mode: just wait for gRPC connections, no interactive CLI
		if result.Config.Mnemosyne.ChatListen == "" {
			agentlog.Fatalf("--serve requires chat_listen to be configured")
		}
		agentlog.Printf("Agent running in server mode (gRPC chat on %s)", result.Config.Mnemosyne.ChatListen)
		setup.WaitForSignal()
		return
	}

	if *oneShot != "" {
		if err := iface.RunOneShotBubble(ctx, result.Agent, *oneShot); err != nil {
			agentlog.Fatalf("Error: %v", err)
		}
	} else {
		// Check if stdin has data (piped input)
		stat, _ := os.Stdin.Stat()
		if stat.Mode()&os.ModeCharDevice == 0 {
			// Read all of stdin as a single message
			var input strings.Builder
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				input.WriteString(scanner.Text())
				input.WriteString("\n")
			}
			msg := strings.TrimSpace(input.String())
			if msg != "" {
				if err := iface.RunOneShotBubble(ctx, result.Agent, msg); err != nil {
					agentlog.Fatalf("Error: %v", err)
				}
			}
		} else {
			if err := iface.RunInteractiveBubble(ctx, result.Agent); err != nil {
				agentlog.Fatalf("Error: %v", err)
			}
		}
	}
}