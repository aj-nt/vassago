// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aj-nt/vassago/daemon/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	subcmd := os.Args[1]
	subArgs := os.Args[2:]

	s := mustOpenStore()
	defer s.Close()
	_ = context.Background()

	switch subcmd {
	case "add":
		cmdAdd(s, subArgs)
	case "get":
		cmdGet(s, subArgs)
	case "list":
		cmdList(s, subArgs)
	case "list-all":
		cmdListAll(s, subArgs)
	case "claim":
		cmdClaim(s, subArgs)
	case "complete":
		cmdComplete(s, subArgs)
	case "fail":
		cmdFail(s, subArgs)
	case "reset-timed-out":
		cmdResetTimedOut(s)
	case "delete":
		cmdDelete(s, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", subcmd)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`vassago-task <subcommand> [args]

Subcommands:
  add <id> <agent_type> <goal> [context_json]   Add a new task
  get <id>                                        Get a task by ID
  list [agent_type]                               List ready tasks
  list-all [status]                               List all tasks by status
  claim <id> <agent_id>                           Claim a ready task
  complete <id> <result_key>                      Complete a task
  fail <id>                                        Fail a claimed task
  reset-timed-out                                  Reset timed-out claims
  delete <id>                                      Remove a task

Flags (before subcommand):
  --db <path>    Path to vassago.db (default: ~/.vassago/vassago.db)`)
}

func mustOpenStore() *store.Store {
	dbPath := os.Getenv("VASSAGO_DB")
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = home + "/.vassago/vassago.db"
	}
	// --db flag
	for i, arg := range os.Args {
		if arg == "--db" && i+1 < len(os.Args) {
			dbPath = os.Args[i+1]
		}
	}
	s, err := store.New(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	return s
}

func cmdAdd(s *store.Store, args []string) {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	prio := fs.Int("prio", 3, "Priority 1-5")
	ttl := fs.Int("ttl", 300, "TTL seconds")
	retries := fs.Int("retries", 2, "Max retries")
	_ = fs.Parse(args)
	if fs.NArg() < 3 {
		fmt.Fprintln(os.Stderr, "Usage: vassago-task add <id> <agent_type> <goal> [context_json]")
		os.Exit(1)
	}
	ctx := "{}"
	if fs.NArg() > 3 {
		ctx = fs.Arg(3)
	}
	t := &store.TaskEntry{
		ID: fs.Arg(0), AgentType: fs.Arg(1), Goal: fs.Arg(2),
		Context: ctx, Priority: int32(*prio), TTLSeconds: int32(*ttl),
		MaxRetries: int32(*retries), Status: store.TaskReady,
		CreatedAt: time.Now().Unix(),
	}
	entry, err := s.AddTask(t)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Added task: %s (agent_type=%s, status=%s, prio=%d, ttl=%ds)\n",
		entry.ID, entry.AgentType, entry.Status, entry.Priority, entry.TTLSeconds)
}

func cmdGet(s *store.Store, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vassago-task get <id>")
		os.Exit(1)
	}
	entry, err := s.GetTask(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if entry == nil {
		fmt.Fprintf(os.Stderr, "Task not found: %s\n", args[0])
		os.Exit(1)
	}
	printTask(entry)
}

func cmdList(s *store.Store, args []string) {
	at := ""
	if len(args) > 0 {
		at = args[0]
	}
	tasks, err := s.FindReadyTasks(at, 50)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(tasks) == 0 {
		fmt.Println("No ready tasks found.")
		return
	}
	fmt.Printf("%d ready tasks:\n\n", len(tasks))
	for _, t := range tasks {
		printCompact(t)
	}
}

func cmdListAll(s *store.Store, args []string) {
	var st store.TaskStatus
	if len(args) > 0 {
		st = store.TaskStatus(args[0])
	}
	tasks, err := s.ListTasksByStatus(st, 100)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return
	}
	fmt.Printf("%d tasks:\n\n", len(tasks))
	for _, t := range tasks {
		printCompact(t)
	}
}

func cmdClaim(s *store.Store, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: vassago-task claim <id> <agent_id>")
		os.Exit(1)
	}
	entry, err := s.ClaimTask(args[0], args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if entry == nil {
		fmt.Println("{\"claimed\": false, \"reason\": \"not_ready_or_not_found\"}")
		return
	}
	fmt.Printf("Claimed: %s by %s\n", entry.ID, entry.AssignedAgent)
}

func cmdComplete(s *store.Store, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: vassago-task complete <id> <result_key>")
		os.Exit(1)
	}
	entry, err := s.CompleteTask(args[0], args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if entry == nil {
		fmt.Fprintf(os.Stderr, "Task not found: %s\n", args[0])
		os.Exit(1)
	}
	fmt.Printf("Completed: %s (result=%s)\n", entry.ID, entry.ResultKey)
}

func cmdFail(s *store.Store, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vassago-task fail <id>")
		os.Exit(1)
	}
	entry, err := s.FailTask(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if entry == nil {
		fmt.Fprintf(os.Stderr, "Task not found: %s\n", args[0])
		os.Exit(1)
	}
	action := "readied for retry"
	if entry.Status == store.TaskDeadLetter {
		action = "dead-lettered"
	}
	fmt.Printf("Failed: %s -> %s (retry %d/%d)\n", entry.ID, action, entry.RetryCount, entry.MaxRetries)
}

func cmdResetTimedOut(s *store.Store) {
	ids, err := s.ResetTimedOutTasks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(ids) == 0 {
		fmt.Println("No timed-out tasks to reset.")
		return
	}
	fmt.Printf("Reset %d timed-out tasks:\n", len(ids))
	for _, id := range ids {
		fmt.Printf("  %s\n", id)
	}
}

func cmdDelete(s *store.Store, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vassago-task delete <id>")
		os.Exit(1)
	}
	deleted, err := s.DeleteTask(args[0])
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

func printTask(t *store.TaskEntry) {
	fmt.Printf("ID:             %s\n", t.ID)
	fmt.Printf("Status:         %s\n", t.Status)
	fmt.Printf("Agent Type:     %s\n", t.AgentType)
	if t.AssignedAgent != "" {
		fmt.Printf("Assigned:       %s\n", t.AssignedAgent)
	}
	fmt.Printf("Goal:           %s\n", t.Goal)
	fmt.Printf("Context:        %s\n", t.Context)
	if t.ResultKey != "" {
		fmt.Printf("Result Key:     %s\n", t.ResultKey)
	}
	fmt.Printf("Priority:       %d\n", t.Priority)
	fmt.Printf("TTL:            %ds\n", t.TTLSeconds)
	fmt.Printf("Retries:        %d/%d\n", t.RetryCount, t.MaxRetries)
	if len(t.ParentIDs) > 0 {
		b, _ := json.Marshal(t.ParentIDs)
		fmt.Printf("Parents:        %s\n", string(b))
	}
	fmt.Printf("Created:        %s\n", time.Unix(t.CreatedAt, 0).Format(time.RFC3339))
	if t.ClaimedAt > 0 {
		fmt.Printf("Claimed:        %s\n", time.Unix(t.ClaimedAt, 0).Format(time.RFC3339))
	}
	if t.CompletedAt > 0 {
		fmt.Printf("Completed:      %s\n", time.Unix(t.CompletedAt, 0).Format(time.RFC3339))
	}
}

func printCompact(t *store.TaskEntry) {
	p := ""
	if len(t.ParentIDs) > 0 {
		b, _ := json.Marshal(t.ParentIDs)
		p = fmt.Sprintf(" parents=%s", string(b))
	}
	a := ""
	if t.AssignedAgent != "" {
		a = fmt.Sprintf(" agent=%s", t.AssignedAgent)
	}
	fmt.Printf("  [%s] %s/%s: %s (p%d, ttl=%d%s%s)\n",
		t.ID, string(t.Status), t.AgentType, trunc(t.Goal, 80),
		t.Priority, t.TTLSeconds, a, p)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
