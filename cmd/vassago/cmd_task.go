// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	memoclient "github.com/aj-nt/vassago-sdk/client"
)

// cmdTask handles "vassago task" — task management for multi-agent orchestration.
// Talks to the daemon via gRPC (same pattern as cmd_memory.go).
func cmdTask(args []string) {
	if len(args) < 1 {
		printTaskHelp()
		return
	}

	// Extract --addr from anywhere in args before determining subcommand.
	addr := "localhost:50051"
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--addr" && i+1 < len(args):
			addr = args[i+1]; i++
		case strings.HasPrefix(arg, "--addr="):
			addr = strings.TrimPrefix(arg, "--addr=")
		default:
			filtered = append(filtered, arg)
		}
	}
	args = filtered

	if len(args) < 1 {
		printTaskHelp()
		return
	}

	subcmd := args[0]
	subArgs := args[1:]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := memoclient.Connect(ctx, memoclient.Config{Address: addr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer c.Close()

	switch subcmd {
	case "add":
		taskAdd(ctx, c, subArgs)
	case "get":
		taskGet(ctx, c, subArgs)
	case "list":
		taskList(ctx, c, subArgs)
	case "list-all":
		taskListAll(ctx, c, subArgs)
	case "claim":
		taskClaim(ctx, c, subArgs)
	case "complete":
		taskComplete(ctx, c, subArgs)
	case "fail":
		taskFail(ctx, c, subArgs)
	case "reset-timed-out":
		taskResetTimedOut(ctx, c)
	case "delete":
		taskDelete(ctx, c, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown task subcommand: %s\n", subcmd)
		printTaskHelp()
		os.Exit(1)
	}
}

func taskAdd(ctx context.Context, c memoclient.MnemoClient, args []string) {
	prio := 3
	ttl := 300
	retries := 2

	// Manually extract flags — Go's flag package stops at first non-flag.
	// This lets users write: vassago task add mytask worker "goal" --prio=4 --ttl=120
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--prio" && i+1 < len(args):
			fmt.Sscanf(args[i+1], "%d", &prio); i++
		case strings.HasPrefix(arg, "--prio="):
			fmt.Sscanf(arg, "--prio=%d", &prio)
		case arg == "--ttl" && i+1 < len(args):
			fmt.Sscanf(args[i+1], "%d", &ttl); i++
		case strings.HasPrefix(arg, "--ttl="):
			fmt.Sscanf(arg, "--ttl=%d", &ttl)
		case arg == "--retries" && i+1 < len(args):
			fmt.Sscanf(args[i+1], "%d", &retries); i++
		case strings.HasPrefix(arg, "--retries="):
			fmt.Sscanf(arg, "--retries=%d", &retries)
		default:
			remaining = append(remaining, arg)
		}
	}
	args = remaining

	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: vassago task add [--prio=N] [--ttl=N] [--retries=N] <id> <agent_type> <goal> [context_json]")
		os.Exit(1)
	}

	tid, agentType, goal := args[0], args[1], args[2]
	contextJSON := "{}"
	if len(args) > 3 {
		contextJSON = args[3]
	}

	entry, err := c.AddTask(ctx, tid, agentType, goal, contextJSON, int32(prio), int32(ttl), int32(retries))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added task: %s (agent_type=%s, status=%s, prio=%d, ttl=%ds)\n",
		entry.Id, entry.AgentType, entry.Status, entry.Priority, entry.TtlSeconds)
}

func taskGet(ctx context.Context, c memoclient.MnemoClient, args []string) {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "Output as JSON")
	fs.SetOutput(os.Stderr)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vassago task get [--json] <id>")
		os.Exit(1)
	}

	entry, err := c.GetTask(ctx, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		out := map[string]interface{}{
			"id":             entry.Id,
			"parent_ids":     entry.ParentIds,
			"status":         entry.Status,
			"agent_type":     entry.AgentType,
			"assigned_agent": entry.AssignedAgent,
			"goal":           entry.Goal,
			"context":        entry.Context,
			"result_key":     entry.ResultKey,
			"priority":       entry.Priority,
			"created_at":     entry.CreatedAt,
			"claimed_at":     entry.ClaimedAt,
			"completed_at":   entry.CompletedAt,
			"ttl_seconds":    entry.TtlSeconds,
			"retry_count":    entry.RetryCount,
			"max_retries":    entry.MaxRetries,
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
		return
	}

	fmt.Printf("ID:             %s\n", entry.Id)
	fmt.Printf("Status:         %s\n", entry.Status)
	fmt.Printf("Agent Type:     %s\n", entry.AgentType)
	if entry.AssignedAgent != "" {
		fmt.Printf("Assigned:       %s\n", entry.AssignedAgent)
	}
	fmt.Printf("Goal:           %s\n", entry.Goal)
	if entry.Context != "" && entry.Context != "{}" {
		fmt.Printf("Context:        %s\n", entry.Context)
	}
	if entry.ResultKey != "" {
		fmt.Printf("Result Key:     %s\n", entry.ResultKey)
	}
	fmt.Printf("Priority:       %d\n", entry.Priority)
	fmt.Printf("TTL:            %ds\n", entry.TtlSeconds)
	fmt.Printf("Retries:        %d/%d\n", entry.RetryCount, entry.MaxRetries)
	if entry.ParentIds != "" && entry.ParentIds != "[]" {
		fmt.Printf("Parents:        %s\n", entry.ParentIds)
	}
	fmt.Printf("Created:        %s\n", time.Unix(entry.CreatedAt, 0).Format(time.RFC3339))
	if entry.ClaimedAt > 0 {
		fmt.Printf("Claimed:        %s\n", time.Unix(entry.ClaimedAt, 0).Format(time.RFC3339))
	}
	if entry.CompletedAt > 0 {
		fmt.Printf("Completed:      %s\n", time.Unix(entry.CompletedAt, 0).Format(time.RFC3339))
	}
}

func taskList(ctx context.Context, c memoclient.MnemoClient, args []string) {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "Output as JSON array")
	limit := fs.Int("limit", 20, "Max results")
	fs.SetOutput(os.Stderr)
	_ = fs.Parse(args)

	agentType := ""
	if fs.NArg() > 0 {
		agentType = fs.Arg(0)
	}

	tasks, err := c.FindReadyTasks(ctx, agentType, int32(*limit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(tasks) == 0 {
		if *jsonOut {
			fmt.Println("[]")
		} else {
			filter := "ready"
			if agentType != "" {
				filter += " " + agentType
			}
			fmt.Printf("No %s tasks found.\n", filter)
		}
		return
	}

	if *jsonOut {
		list := make([]map[string]interface{}, len(tasks))
		for i, t := range tasks {
			list[i] = taskToMap(t)
		}
		b, _ := json.Marshal(list)
		fmt.Println(string(b))
		return
	}

	fmt.Printf("%d tasks:\n\n", len(tasks))
	for _, t := range tasks {
		fmt.Printf("  [%s] %s/%s: %s (p%d, ttl=%d)\n",
			t.Id, t.Status, t.AgentType, truncate(t.Goal, 80),
			t.Priority, t.TtlSeconds)
	}
}

func taskListAll(ctx context.Context, c memoclient.MnemoClient, args []string) {
	fs := flag.NewFlagSet("list-all", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "Output as JSON array")
	limit := fs.Int("limit", 50, "Max results")
	fs.SetOutput(os.Stderr)
	_ = fs.Parse(args)

	statusFilter := ""
	if fs.NArg() > 0 {
		statusFilter = fs.Arg(0)
	}

	tasks, err := c.ListTasksByStatus(ctx, statusFilter, int32(*limit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(tasks) == 0 {
		if *jsonOut {
			fmt.Println("[]")
		} else {
			fmt.Println("No tasks found.")
		}
		return
	}

	if *jsonOut {
		list := make([]map[string]interface{}, len(tasks))
		for i, t := range tasks {
			list[i] = taskToMap(t)
		}
		b, _ := json.Marshal(list)
		fmt.Println(string(b))
		return
	}

	fmt.Printf("%d tasks:\n\n", len(tasks))
	for _, t := range tasks {
		agent := ""
		if t.AssignedAgent != "" {
			agent = fmt.Sprintf(" agent=%s", t.AssignedAgent)
		}
		fmt.Printf("  [%s] %s/%s: %s (p%d, ttl=%d%s)\n",
			t.Id, t.Status, t.AgentType, truncate(t.Goal, 80),
			t.Priority, t.TtlSeconds, agent)
	}
}

func taskClaim(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: vassago task claim <id> <agent_id>")
		os.Exit(1)
	}

	entry, err := c.ClaimTask(ctx, args[0], args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if entry.Status == "claimed" && entry.AssignedAgent == "other" {
		fmt.Println("Claimed: false (already claimed by another agent)")
		return
	}

	fmt.Printf("Claimed: %s by %s\n", entry.Id, entry.AssignedAgent)
}

func taskComplete(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: vassago task complete <id> <result_key>")
		os.Exit(1)
	}

	entry, err := c.CompleteTask(ctx, args[0], args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Completed: %s (result=%s)\n", entry.Id, entry.ResultKey)
}

func taskFail(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vassago task fail <id>")
		os.Exit(1)
	}

	entry, err := c.FailTask(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	action := "readied for retry"
	if entry.Status == "dead_letter" {
		action = "dead-lettered"
	}
	fmt.Printf("Failed: %s -> %s (retry %d/%d)\n", entry.Id, action, entry.RetryCount, entry.MaxRetries)
}

func taskResetTimedOut(ctx context.Context, c memoclient.MnemoClient) {
	resp, err := c.ResetTimedOutTasks(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if resp.Count == 0 {
		fmt.Println("No timed-out tasks to reset.")
		return
	}

	fmt.Printf("Reset %d timed-out tasks:\n", resp.Count)
	for _, id := range resp.ResetIds {
		fmt.Printf("  %s\n", id)
	}
}

func taskDelete(ctx context.Context, c memoclient.MnemoClient, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vassago task delete <id>")
		os.Exit(1)
	}

	deleted, err := c.DeleteTask(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if deleted {
		fmt.Printf("Deleted: %s\n", args[0])
	} else {
		fmt.Fprintf(os.Stderr, "Task not found: %s\n", args[0])
	}
}

func taskToMap(t *memoclient.TaskEntry) map[string]interface{} {
	return map[string]interface{}{
		"id":             t.Id,
		"parent_ids":     t.ParentIds,
		"status":         t.Status,
		"agent_type":     t.AgentType,
		"assigned_agent": t.AssignedAgent,
		"goal":           t.Goal,
		"context":        t.Context,
		"result_key":     t.ResultKey,
		"priority":       t.Priority,
		"created_at":     t.CreatedAt,
		"claimed_at":     t.ClaimedAt,
		"completed_at":   t.CompletedAt,
		"ttl_seconds":    t.TtlSeconds,
		"retry_count":    t.RetryCount,
		"max_retries":    t.MaxRetries,
	}
}

func printTaskHelp() {
	fmt.Println(`Usage: vassago task <subcommand> [flags] [args]

Task lifecycle for multi-agent orchestration:
  add [--prio=N] [--ttl=N] [--retries=N] <id> <agent_type> <goal> [context_json]
  get [--json] <id>
  list [--json] [--limit=N] [agent_type]
  list-all [--json] [--limit=N] [status]
  claim <id> <agent_id>
  complete <id> <result_key>
  fail <id>
  reset-timed-out
  delete <id>

Task states: ready -> claimed -> done|failed -> (retry: ready) | dead_letter`)
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
// Duplicated from cmd_memory.go (truncateStr) to keep tasks self-contained.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
