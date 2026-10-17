// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/aj-nt/vassago-sdk/client"
	"github.com/google/uuid"
)

// Worker polls the daemon for ready tasks, claims them, executes via LLM,
// and reports results.
type Worker struct {
	cfg         *Config
	mnemo       client.MnemoClient
	ollama      *OllamaClient
	agentID     string // nolint:unused
	completed   int
	failed      int
}

func NewWorker(cfg *Config) (*Worker, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := client.Connect(ctx, client.Config{Address: cfg.DaemonAddr})
	if err != nil {
		return nil, fmt.Errorf("connect to daemon at %s: %w", cfg.DaemonAddr, err)
	}

	return &Worker{
		cfg:    cfg,
		mnemo:  c,
		ollama: NewOllamaClient(cfg.Ollama),
	}, nil
}

func (w *Worker) Close() {
	w.mnemo.Close()
}

func (w *Worker) Register(ctx context.Context) (agentID string, isNew bool, err error) {
	return w.mnemo.RegisterAgent(ctx, w.cfg.AgentID, w.cfg.AgentName, "worker")
}

func (w *Worker) HeartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.mnemo.Heartbeat(ctx, w.cfg.AgentID); err != nil {
				slog.Warn("Heartbeat failed", "error", err)
			}
		}
	}
}

// Run is the main polling loop. It blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	pollInterval := time.Duration(w.cfg.PollSec) * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		claimed := false
		for _, at := range w.cfg.AgentTypes {
			tasks, err := w.mnemo.FindReadyTasks(ctx, at, 1)
			if err != nil {
				slog.Warn("FindReadyTasks failed", "agent_type", at, "error", err)
				continue
			}
			if len(tasks) == 0 {
				continue
			}

			task := tasks[0]
			tid := task.Id
			slog.Info("Claiming task", "id", tid, "type", at)

			claimedTask, err := w.mnemo.ClaimTask(ctx, tid, w.cfg.AgentID)
			if err != nil {
				slog.Warn("ClaimTask failed", "id", tid, "error", err)
				continue
			}

			// ClaimTask returns the task entry on success, or nil if already claimed.
			// If the status isn't "claimed", someone else got it.
			if claimedTask == nil || claimedTask.Status != "claimed" {
				slog.Info("Task already claimed by another worker", "id", tid)
				continue
			}

			claimed = true
			slog.Info("Executing task", "id", tid, "type", at, "goal", truncate(claimedTask.Goal, 100))

			result := w.executeTask(ctx, claimedTask)
			key := fmt.Sprintf("result_%s_%s", tid, uuid.New().String()[:8])

			// Store result in memory blackboard
			resultJSON, _ := json.Marshal(result)
			if _, err := w.mnemo.AddMemory(ctx, "memory", "orchestrator_results", key,
				string(resultJSON), 4, w.cfg.AgentID); err != nil {
				slog.Warn("Failed to store result", "id", tid, "error", err)
			}

			if result.Status == "success" {
				if _, err := w.mnemo.CompleteTask(ctx, tid, key); err != nil {
					slog.Warn("Failed to mark task complete", "id", tid, "error", err)
					continue
				}
				w.completed++
				slog.Info("Task completed", "id", tid, "key", key,
					"completed", w.completed, "failed", w.failed)
			} else {
				if _, err := w.mnemo.FailTask(ctx, tid); err != nil {
					slog.Warn("Failed to mark task failed", "id", tid, "error", err)
				}
				w.failed++
				slog.Warn("Task failed", "id", tid, "error", result.Error,
					"completed", w.completed, "failed", w.failed)
			}
			break // one task per poll cycle
		}

		if !claimed {
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollInterval):
			}
		}
	}
}

// executeTask runs the LLM execution loop for a single task.
func (w *Worker) executeTask(ctx context.Context, task *client.TaskEntry) *TaskResult {
	t0 := time.Now()
	goal := task.Goal
	contextStr := task.Context
	if contextStr == "" {
		contextStr = "{}"
	}

	// Parse context for repo_path (used as CWD for commands)
	var taskCtx map[string]interface{}
	_ = json.Unmarshal([]byte(contextStr), &taskCtx)

	messages := []message{{
		Role:    "user",
		Content: buildExecutionPrompt(goal, contextStr),
	}}

	findings := make(map[string]Finding)
	maxTurns := w.cfg.MaxTurns

	for turn := 1; turn <= maxTurns; turn++ {
		// Check if context is cancelled between LLM calls
		select {
		case <-ctx.Done():
			return &TaskResult{TaskID: task.Id, Status: "failed", Error: "worker shutting down"}
		default:
		}

		text, err := w.ollama.Chat(messages)
		if err != nil {
			return &TaskResult{
				TaskID:     task.Id,
				Status:     "failed",
				Error:      fmt.Sprintf("LLM error: %v", err),
				Findings:   findings,
				DurationMs: time.Since(t0).Milliseconds(),
			}
		}

		text = strings.TrimSpace(text)
		messages = append(messages, message{Role: "assistant", Content: text})

		switch {
		case strings.HasPrefix(text, "CMD:"):
			cmd := strings.TrimSpace(text[4:])
			slog.Info("Running command", "turn", turn, "cmd", truncate(cmd, 80))

			output := runCommand(cmd, taskCtx)
			findings[truncate(cmd, 60)] = Finding{
				ExitCode:     output.ExitCode,
				OutputLength: len(output.Stdout) + len(output.Stderr),
			}

			response := fmt.Sprintf("Command output:\n%s", output.Stdout)
			if output.Stderr != "" {
				response += fmt.Sprintf("\n[stderr]: %s", truncate(output.Stderr, 1000))
			}
			if output.ExitCode != 0 {
				response += fmt.Sprintf("\n[exit: %d]", output.ExitCode)
			}
			messages = append(messages, message{Role: "user", Content: response})

		case strings.HasPrefix(text, "DONE:"):
			summary := strings.TrimSpace(text[5:])
			slog.Info("Task DONE", "id", task.Id, "summary", truncate(summary, 100), "turns", turn)
			return &TaskResult{
				TaskID:     task.Id,
				AgentType:  task.AgentType,
				Status:     "success",
				Summary:    summary,
				Findings:   findings,
				Model:      w.cfg.Ollama.Model,
				LLMTurns:   turn,
				DurationMs: time.Since(t0).Milliseconds(),
				CompletedAt: time.Now().Unix(),
			}

		case strings.HasPrefix(text, "FAIL:"):
			reason := strings.TrimSpace(text[5:])
			slog.Warn("Task FAIL", "id", task.Id, "reason", truncate(reason, 100), "turns", turn)
			return &TaskResult{
				TaskID:     task.Id,
				Status:     "failed",
				Error:      reason,
				Findings:   findings,
				Model:      w.cfg.Ollama.Model,
				LLMTurns:   turn,
				DurationMs: time.Since(t0).Milliseconds(),
			}

		default:
			// Unstructured — nudge toward protocol
			messages = append(messages, message{
				Role:    "user",
				Content: "Respond with CMD:, DONE:, or FAIL:",
			})
		}
	}

	return &TaskResult{
		TaskID:     task.Id,
		Status:     "failed",
		Error:      fmt.Sprintf("Exceeded %d turns without DONE/FAIL", maxTurns),
		Findings:   findings,
		Model:      w.cfg.Ollama.Model,
		LLMTurns:   maxTurns,
		DurationMs: time.Since(t0).Milliseconds(),
	}
}

// TaskResult is the result of executing a task.
type TaskResult struct {
	TaskID      string             `json:"task_id"`
	AgentType   string             `json:"agent_type,omitempty"`
	Status      string             `json:"status"`
	Summary     string             `json:"summary,omitempty"`
	Error       string             `json:"error,omitempty"`
	Findings    map[string]Finding `json:"findings"`
	Model       string             `json:"model,omitempty"`
	LLMTurns    int                `json:"llm_turns"`
	DurationMs  int64              `json:"duration_ms"`
	CompletedAt int64              `json:"completed_at,omitempty"`
}

// Finding records the result of a single command execution.
type Finding struct {
	ExitCode     int    `json:"exit_code"`
	OutputLength int    `json:"output_length"`
	Error        string `json:"error,omitempty"`
}

// CommandOutput holds the result of running a shell command.
type CommandOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// runCommand executes a shell command with a 60-second timeout.
func runCommand(cmd string, taskCtx map[string]interface{}) CommandOutput {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, "sh", "-c", cmd)

	// Set working directory from task context if available
	if repoPath, ok := taskCtx["repo_path"].(string); ok && repoPath != "" {
		c.Dir = repoPath
	}

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	exitCode := 0
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return CommandOutput{Stdout: "", Stderr: "[TIMEOUT after 60s]", ExitCode: -1}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	// Limit output size
	out := stdout.String()
	if len(out) > 4000 {
		out = out[len(out)-4000:]
	}
	errOut := stderr.String()
	if len(errOut) > 1000 {
		errOut = errOut[len(errOut)-1000:]
	}

	return CommandOutput{Stdout: out, Stderr: errOut, ExitCode: exitCode}
}

// buildExecutionPrompt constructs the system+user prompt for task execution.
func buildExecutionPrompt(goal, contextStr string) string {
	return fmt.Sprintf(`You are an autonomous worker agent executing a task.

Your job: accomplish the goal using shell commands. You have full terminal access.

PROTOCOL — respond with exactly one of these on each turn:
CMD: <shell command>
DONE: <summary of what you accomplished>
FAIL: <reason>

RULES:
- One command per CMD: line. Keep commands focused.
- Use head/tail to limit large output.
- Never run destructive commands (rm -rf, git push --force, etc.).
- Commands timeout after 60 seconds.
- You have a budget of ~10 turns. After turn 8, start wrapping up.
- When you have enough data to answer, DONE: immediately.
- Analyze output before issuing the next command.

CONTEXT:
%s

GOAL:
%s

Start now: output your first CMD: line.`, contextStr, goal)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
