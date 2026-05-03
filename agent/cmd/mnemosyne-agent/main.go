package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/aj-nt/mnemosyne/agent/setup"
	iface "github.com/aj-nt/mnemosyne/agent/interface"
)

func main() {
	configPath := flag.String("config", "configs/agent-dev.yaml", "Path to agent config file")
	oneShot := flag.String("message", "", "Single message (non-interactive)")
	serve := flag.Bool("serve", false, "Server mode: start gRPC chat server and wait (no interactive CLI)")
	flag.Parse()

	ctx := context.Background()

	result, err := setup.InitAgent(ctx, *configPath)
	if err != nil {
		log.Fatalf("Failed to initialize agent: %v", err)
	}
	defer result.Close()

	// Run
	if *serve {
		// Server mode: just wait for gRPC connections, no interactive CLI
		if result.Config.Mnemosyne.ChatListen == "" {
			log.Fatal("--serve requires chat_listen to be configured")
		}
		log.Printf("Agent running in server mode (gRPC chat on %s)", result.Config.Mnemosyne.ChatListen)
		setup.WaitForSignal()
		return
	}

	cli := iface.NewCLI(result.Agent)
	if *oneShot != "" {
		if err := cli.RunOneShot(ctx, *oneShot); err != nil {
			log.Fatalf("Error: %v", err)
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
				if err := cli.RunOneShot(ctx, msg); err != nil {
					log.Fatalf("Error: %v", err)
				}
			}
		} else {
			if err := cli.RunInteractive(ctx); err != nil {
				log.Fatalf("Error: %v", err)
			}
		}
	}
}