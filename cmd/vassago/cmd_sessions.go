// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	memoclient "github.com/aj-nt/vassago/client"
)

// cmdSessions handles "vassago sessions" — session management.
func cmdSessions(args []string) {
	if len(args) < 1 {
		fmt.Println(`Usage: vassago sessions <subcommand> [args]

Subcommands:
  list              List recent sessions
  resume <id>       Resume a previous session (show messages)
  get <id>          Show session details

Use "vassago sessions <subcommand> --help" for more information.`)
		return
	}

	fs := flag.NewFlagSet("sessions", flag.ExitOnError)
	addr := fs.String("addr", "localhost:50051", "Daemon address")
	fs.Parse(args[1:])

	subcmd := args[0]
	remaining := fs.Args()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := memoclient.Connect(ctx, memoclient.Config{Address: *addr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer c.Close()

	switch subcmd {
	case "list":
		sessionsList(ctx, c, remaining)
	case "resume":
		sessionsResume(ctx, c, remaining)
	case "get":
		sessionsGet(ctx, c, remaining)
	default:
		fmt.Fprintf(os.Stderr, "Unknown sessions subcommand: %s\n", subcmd)
		os.Exit(1)
	}
}

func sessionsList(ctx context.Context, c memoclient.MnemoClient, args []string) {
	limit := 20
	if len(args) > 0 {
		fmt.Sscanf(args[0], "%d", &limit)
	}

	sessions, err := c.ListRecentSessions(ctx, int32(limit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return
	}

	fmt.Printf("%-36s  %-20s  %6s  %-10s  %s\n", "ID", "Title", "Msgs", "Source", "Created")
	fmt.Println(stringsRepeat("─", 100))
	for _, s := range sessions {
		title := s.Title
		if title == "" {
			title = "(untitled)"
		}
		if len(title) > 18 {
			title = title[:15] + "..."
		}
		created := time.Unix(s.LastActive, 0).Format("2006-01-02 15:04")
		fmt.Printf("%-36s  %-20s  %6d  %-10s  %s\n",
			s.ID, title, s.MessageCount, s.Source, created)
	}
	fmt.Printf("\n%d session(s)\n", len(sessions))
}

func sessionsResume(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vassago sessions resume <session-id>")
		fmt.Fprintln(os.Stderr, "\nUse 'vassago sessions list' to see session IDs.")
		os.Exit(1)
	}

	sessionID := args[0]
	detail, err := c.GetSession(ctx, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Session: %s\n", detail.Title)
	fmt.Printf("Source:  %s\n", detail.Source)
	fmt.Printf("Created: %s\n", time.Unix(detail.CreatedAt, 0).Format(time.RFC3339))
	fmt.Printf("Messages: %d\n\n", len(detail.Messages))

	fmt.Println("─" + stringsRepeat("─", 78))
	for _, m := range detail.Messages {
		preview := m.Content
		if len(preview) > 200 {
			preview = preview[:197] + "..."
		}
		fmt.Printf("[%s] %s\n\n", m.Role, preview)
	}
}

func sessionsGet(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vassago sessions get <session-id>")
		os.Exit(1)
	}

	sessionID := args[0]
	detail, err := c.GetSession(ctx, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("ID:      %s\n", detail.ID)
	fmt.Printf("Title:   %s\n", detail.Title)
	fmt.Printf("Source:  %s\n", detail.Source)
	fmt.Printf("Created: %s\n", time.Unix(detail.CreatedAt, 0).Format(time.RFC3339))
	fmt.Printf("Messages: %d\n", len(detail.Messages))
}

func stringsRepeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
