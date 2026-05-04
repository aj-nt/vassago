// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/client"
)

var addr = flag.String("addr", "localhost:50051", "Daemon address (host:port or unix:///path/to/socket)")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mnemosyne-cli [flags] <command> [args...]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  add <target> <category> <key> <content>  Add or update a memory\n")
		fmt.Fprintf(os.Stderr, "  get <id>                                   Get a memory by ID\n")
		fmt.Fprintf(os.Stderr, "  search <query>                             FTS5 search\n")
		fmt.Fprintf(os.Stderr, "  list [target]                              List memories (default: memory)\n")
		fmt.Fprintf(os.Stderr, "  remove <id>                                Remove a memory\n")
		fmt.Fprintf(os.Stderr, "  hot-block [target]                         Show computed hot block\n")
		fmt.Fprintf(os.Stderr, "  consolidate <scope>                        Run consolidation (expired|idle|observations|all)\n")
		fmt.Fprintf(os.Stderr, "  stats                                       Memory statistics\n")
		fmt.Fprintf(os.Stderr, "  create-session <agent-id> <source> [title] Create a session\n")
		fmt.Fprintf(os.Stderr, "  health                                      Health check\n")
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	command := args[0]
	cmdArgs := args[1:]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := client.Connect(ctx, client.Config{Address: *addr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer c.Close()

	// For RPCs not in the client package, create a raw gRPC connection
	rawConn, rawAPI, err := client.RawConnection(*addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer rawConn.Close()

	switch command {
	case "add":
		cmdAdd(ctx, c, cmdArgs)
	case "get":
		cmdGet(ctx, c, cmdArgs)
	case "search":
		cmdSearch(ctx, c, cmdArgs)
	case "list":
		cmdList(ctx, c, cmdArgs)
	case "remove":
		cmdRemove(ctx, c, cmdArgs)
	case "hot-block":
		cmdHotBlock(ctx, c, cmdArgs)
	case "consolidate":
		cmdConsolidate(ctx, rawAPI, cmdArgs)
	case "stats":
		cmdStats(ctx, c)
	case "create-session":
		cmdCreateSession(ctx, c, cmdArgs)
	case "health":
		cmdHealth(ctx, c)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

func cmdAdd(ctx context.Context, c *client.Client, args []string) {
	if len(args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: mnemosyne-cli add <target> <category> <key> <content> [priority]\n")
		os.Exit(1)
	}
	target, category, key, content := args[0], args[1], args[2], args[3]
	priority := int32(3) // default
	if len(args) > 4 {
		_, _ = fmt.Sscanf(args[4], "%d", &priority)
	}

	entry, err := c.AddMemory(ctx, target, category, key, content, priority, "cli")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Added memory: %s (target=%s, category=%s, key=%s, priority=%d)\n",
		entry.Id, entry.Target, entry.Category, entry.Key, entry.Priority)
}

func cmdGet(ctx context.Context, c *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: mnemosyne-cli get <id>\n")
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

func cmdSearch(ctx context.Context, c *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: mnemosyne-cli search <query> [target]\n")
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
			e.Id[:8], e.Category, e.Key, truncate(e.Content, 80), e.Priority,
			time.Unix(e.CreatedAt, 0).Format("2006-01-02"))
	}
}

func cmdList(ctx context.Context, c *client.Client, args []string) {
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
			e.Id[:8], e.Category, e.Key, truncate(e.Content, 60), e.Priority,
			time.Unix(e.CreatedAt, 0).Format("2006-01-02"))
	}
}

func cmdRemove(ctx context.Context, c *client.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: mnemosyne-cli remove <id>\n")
		os.Exit(1)
	}
	err := c.RemoveMemory(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Removed memory %s\n", args[0])
}

func cmdHotBlock(ctx context.Context, c *client.Client, args []string) {
	target := "memory"
	if len(args) > 0 {
		target = args[0]
	}
	content, err := c.GetHotBlock(ctx, target, "cli", 3000, nil, nil, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(content)
}

func cmdConsolidate(ctx context.Context, api pb.MnemosyneClient, args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: mnemosyne-cli consolidate <scope> (expired|idle|observations|all)\n")
		os.Exit(1)
	}
	scope := args[0]

	var pbScope pb.ConsolidateScope
	switch strings.ToLower(scope) {
	case "expired":
		pbScope = pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED
	case "idle":
		pbScope = pb.ConsolidateScope_CONSOLIDATE_SCOPE_ALL // idle maps to ALL for now
	case "observations":
		pbScope = pb.ConsolidateScope_CONSOLIDATE_SCOPE_OBSERVATIONS
	case "all":
		pbScope = pb.ConsolidateScope_CONSOLIDATE_SCOPE_ALL
	default:
		fmt.Fprintf(os.Stderr, "Unknown scope: %s (use: expired, idle, observations, all)\n", scope)
		os.Exit(1)
	}

	resp, err := api.Consolidate(ctx, &pb.ConsolidateRequest{Scope: pbScope})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Consolidated %d entries (scope=%s)\n", resp.Removed, scope)
}

func cmdStats(ctx context.Context, c *client.Client) {
	// Stats via listing all memories and counting
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

	// Count by category
	categories := make(map[string]int)
	for _, e := range memEntries {
		categories[e.Category]++
	}

	fmt.Printf("Mnemosyne Statistics\n")
	fmt.Printf("=====================\n\n")
	fmt.Printf("Memory entries:  %d\n", len(memEntries))
	fmt.Printf("User entries:     %d\n", len(userEntries))
	fmt.Printf("Total entries:    %d\n\n", len(memEntries)+len(userEntries))

	if len(categories) > 0 {
		fmt.Printf("By category:\n")
		for cat, count := range categories {
			fmt.Printf("  %-15s %d\n", cat, count)
		}
	}
}

func cmdCreateSession(ctx context.Context, c *client.Client, args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: mnemosyne-cli create-session <agent-id> <source> [title]\n")
		os.Exit(1)
	}
	agentID, source := args[0], args[1]
	title := "CLI Session"
	if len(args) > 2 {
		title = strings.Join(args[2:], " ")
	}

	id, err := c.CreateSession(ctx, agentID, source, title)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created session: %s\n", id)
}

func cmdHealth(ctx context.Context, c *client.Client) {
	err := c.HealthCheck(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Health check FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK — daemon is healthy.")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func formatTime(unix int64) string {
	if unix == 0 {
		return "never"
	}
	return time.Unix(unix, 0).Format(time.RFC3339)
}