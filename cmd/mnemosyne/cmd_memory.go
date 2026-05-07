// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	memoclient "github.com/aj-nt/mnemosyne/client"
)

// cmdMemory handles "mnemosyne memory" — memory management commands.
func cmdMemory(args []string) {
	if len(args) < 1 {
		fmt.Println(`Usage: mnemosyne memory <subcommand> [args]

Subcommands:
  add <target> <category> <key> <content>  Add or update a memory
  get <id>                                  Get a memory by ID
  search <query>                            FTS5 search
  list [target]                              List memories (default: memory)
  remove <id>                                Remove a memory
  consolidate <scope>                        Run consolidation (expired|idle|observations|all)
  stats                                       Memory statistics

Use "mnemosyne memory <subcommand> --help" for more information.`)
		return
	}

	subcmd := args[0]
	subArgs := args[1:]

	fs := flag.NewFlagSet("memory", flag.ExitOnError)
	addr := fs.String("addr", "localhost:50051", "Daemon address (host:port)")
	fs.Parse(subArgs)

	// Remaining args after flags
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
	case "add":
		memAdd(ctx, c, remaining)
	case "get":
		memGet(ctx, c, remaining)
	case "search":
		memSearch(ctx, c, remaining)
	case "list":
		memList(ctx, c, remaining)
	case "remove":
		memRemove(ctx, c, remaining)
	case "consolidate":
		memConsolidate(ctx, c, remaining)
	case "stats":
		memStats(ctx, c)
	default:
		fmt.Fprintf(os.Stderr, "Unknown memory subcommand: %s\n", subcmd)
		os.Exit(1)
	}
}

func memAdd(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 4 {
		fmt.Fprintln(os.Stderr, "Usage: mnemosyne memory add <target> <category> <key> <content> [priority]")
		os.Exit(1)
	}
	target, category, key, content := args[0], args[1], args[2], args[3]
	priority := int32(3)
	if len(args) > 4 {
		fmt.Sscanf(args[4], "%d", &priority)
	}

	entry, err := c.AddMemory(ctx, target, category, key, content, priority, "cli")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Added memory: %s (target=%s, category=%s, key=%s, priority=%d)\n",
		entry.Id, entry.Target, entry.Category, entry.Key, entry.Priority)
}

func memGet(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: mnemosyne memory get <id>")
		os.Exit(1)
	}
	entry, err := c.GetMemory(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("ID:          %s\n", entry.Id)
	fmt.Printf("Target:      %s\n", entry.Target)
	fmt.Printf("Category:    %s\n", entry.Category)
	fmt.Printf("Key:         %s\n", entry.Key)
	fmt.Printf("Priority:    %d\n", entry.Priority)
	fmt.Printf("Source:      %s\n", entry.SourceAgent)
	fmt.Printf("Created:     %s\n", time.Unix(entry.CreatedAt, 0).Format(time.RFC3339))
	fmt.Printf("Updated:     %s\n", time.Unix(entry.UpdatedAt, 0).Format(time.RFC3339))
	fmt.Printf("Expires:     %s\n", formatTime(entry.ExpiresAt))
	fmt.Printf("Content:\n%s\n", entry.Content)
}

func memSearch(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: mnemosyne memory search <query> [target]")
		os.Exit(1)
	}
	query := args[0]
	target := "memory"
	if len(args) > 1 {
		target = args[1]
	}
	limit := int32(20)

	entries, err := c.SearchMemories(ctx, query, target, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Println("No results found.")
		return
	}
	fmt.Printf("Found %d result(s):\n\n", len(entries))
	for _, e := range entries {
		fmt.Printf("  [%s] %s/%s: %s (priority=%d, created=%s)\n",
			e.Id[:8], e.Category, e.Key, truncateStr(e.Content, 80), e.Priority,
			time.Unix(e.CreatedAt, 0).Format("2006-01-02"))
	}
}

func memList(ctx context.Context, c memoclient.MnemoClient, args []string) {
	target := "memory"
	if len(args) > 0 {
		target = args[0]
	}
	entries, err := c.ListMemories(ctx, target, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Printf("No %s memories found.\n", target)
		return
	}
	fmt.Printf("%d %s memories:\n\n", len(entries), target)
	for _, e := range entries {
		fmt.Printf("  [%s] %s/%s: %s (p%d, %s)\n",
			e.Id[:8], e.Category, e.Key, truncateStr(e.Content, 60), e.Priority,
			time.Unix(e.CreatedAt, 0).Format("2006-01-02"))
	}
}

func memRemove(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: mnemosyne memory remove <id>")
		os.Exit(1)
	}
	err := c.RemoveMemory(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Removed memory %s\n", args[0])
}

func memConsolidate(ctx context.Context, c memoclient.MnemoClient, args []string) {
	fmt.Fprintln(os.Stderr, "Consolidation requires gRPC raw API — use mnemosyne-cli consolidate")
	os.Exit(1)
}

func memStats(ctx context.Context, c memoclient.MnemoClient) {
	memEntries, err := c.ListMemories(ctx, "memory", nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing memories: %v\n", err)
		os.Exit(1)
	}
	userEntries, err := c.ListMemories(ctx, "user", nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing user entries: %v\n", err)
		os.Exit(1)
	}

	categories := make(map[string]int)
	for _, e := range memEntries {
		categories[e.Category]++
	}

	fmt.Println("Mnemosyne Statistics")
	fmt.Println("=====================")
	fmt.Println()
	fmt.Printf("Memory entries:  %d\n", len(memEntries))
	fmt.Printf("User entries:    %d\n", len(userEntries))
	fmt.Printf("Total entries:   %d\n\n", len(memEntries)+len(userEntries))

	if len(categories) > 0 {
		fmt.Println("By category:")
		for cat, count := range categories {
			fmt.Printf("  %-15s %d\n", cat, count)
		}
	}
}

func formatTime(unix int64) string {
	if unix == 0 {
		return "never"
	}
	return time.Unix(unix, 0).Format(time.RFC3339)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// cmdHotBlock handles "mnemosyne hot-block".
func cmdHotBlock(args []string) {
	fs := flag.NewFlagSet("hot-block", flag.ExitOnError)
	addr := fs.String("addr", "localhost:50051", "Daemon address")
	target := fs.String("target", "memory", "Target (memory or user)")
	fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := memoclient.Connect(ctx, memoclient.Config{Address: *addr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer c.Close()

	content, err := c.GetHotBlock(ctx, *target, "cli", 3000, nil, nil, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(content)
}

// cmdHealth handles "mnemosyne health".
func cmdHealth(args []string) {
	addr := "localhost:50051"
	for i, arg := range args {
		if arg == "--addr" && i+1 < len(args) {
			addr = args[i+1]
		}
		if arg == "--help" || arg == "-h" {
			fmt.Println("Usage: mnemosyne health [--addr ADDRESS]")
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := memoclient.Connect(ctx, memoclient.Config{Address: addr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Health check FAILED: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	if err := c.HealthCheck(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Health check FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK — daemon is healthy.")
}