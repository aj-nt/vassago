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

// truncate shortens s to maxLen chars with "..." suffix if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// startWorker runs the orchestrator task execution loop in a goroutine.
// It blocks until ctx is cancelled. Safe to call from main's goroutine.
func startWorker(ctx context.Context, c client.MnemoClient, agentID string, cfg WorkerConfig) {
	pollInterval := time.Duration(cfg.PollSec) * time.Second
	ollama := newOllamaClient(cfg.Ollama)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		claimed := false
		for _, at := range cfg.AgentTypes {
			tasks, err := c.FindReadyTasks(ctx, at, 1)
			if err != nil {
				slog.Warn("Worker FindReadyTasks failed", "agent_type", at, "error", err)
				continue
			}
			if len(tasks) == 0 {
				continue
			}

			task := tasks[0]
			tid := task.Id
			slog.Info("Worker claiming task", "id", tid, "type", at)

			claimedTask, err := c.ClaimTask(ctx, tid, agentID)
			if err != nil {
				slog.Warn("Worker ClaimTask failed", "id", tid, "error", err)
				continue
			}

			if claimedTask == nil || claimedTask.Status != "claimed" {
				slog.Info("Worker task already claimed", "id", tid)
				continue
			}

			claimed = true
			slog.Info("Worker executing task", "id", tid, "type", at, "goal", truncate(claimedTask.Goal, 100))

			result := executeTask(ctx, ollama, claimedTask, cfg.MaxTurns)
			key := fmt.Sprintf("result_%s_%s", tid, uuid.New().String()[:8])

			resultJSON, _ := json.Marshal(result)
			if _, err := c.AddMemory(ctx, "memory", "orchestrator_results", key,
				string(resultJSON), 4, agentID); err != nil {
				slog.Warn("Worker failed to store result", "id", tid, "error", err)
			}

			if result.Status == "success" {
				if _, err := c.CompleteTask(ctx, tid, key); err != nil {
					slog.Warn("Worker failed to mark task complete", "id", tid, "error", err)
				} else {
					slog.Info("Worker task completed", "id", tid, "key", key,
						"duration_ms", result.DurationMs, "turns", result.LLMTurns)
				}
			} else {
				if _, err := c.FailTask(ctx, tid); err != nil {
					slog.Warn("Worker failed to mark task failed", "id", tid, "error", err)
				}
				slog.Warn("Worker task failed", "id", tid, "error", result.Error,
					"duration_ms", result.DurationMs, "turns", result.LLMTurns)
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
func executeTask(ctx context.Context, ollama *OllamaClient, task *client.TaskEntry, maxTurns int) *workerTaskResult {
	t0 := time.Now()
	goal := task.Goal
	contextStr := task.Context
	if contextStr == "" {
		contextStr = "{}"
	}

	var taskCtx map[string]interface{}
	_ = json.Unmarshal([]byte(contextStr), &taskCtx)

	messages := []ollamaMessage{{
		Role:    "user",
		Content: buildWorkerPrompt(goal, contextStr),
	}}

	for turn := 1; turn <= maxTurns; turn++ {
		select {
		case <-ctx.Done():
			return &workerTaskResult{TaskID: task.Id, Status: "failed", Error: "worker shutting down"}
		default:
		}

		text, err := ollama.Chat(messages)
		if err != nil {
			return &workerTaskResult{
				TaskID:     task.Id,
				Status:     "failed",
				Error:      fmt.Sprintf("LLM error: %v", err),
				DurationMs: time.Since(t0).Milliseconds(),
			}
		}

		text = strings.TrimSpace(text)
		messages = append(messages, ollamaMessage{Role: "assistant", Content: text})

		switch {
		case strings.HasPrefix(text, "CMD:"):
			cmd := strings.TrimSpace(text[4:])
			slog.Info("Worker running command", "turn", turn, "cmd", truncate(cmd, 80))

			output := runWorkerCommand(cmd, taskCtx)

			response := fmt.Sprintf("Command output:\n%s", output.Stdout)
			if output.Stderr != "" {
				response += fmt.Sprintf("\n[stderr]: %s", truncate(output.Stderr, 1000))
			}
			if output.ExitCode != 0 {
				response += fmt.Sprintf("\n[exit: %d]", output.ExitCode)
			}
			messages = append(messages, ollamaMessage{Role: "user", Content: response})

		case strings.HasPrefix(text, "DONE:"):
			summary := strings.TrimSpace(text[5:])
			slog.Info("Worker task DONE", "id", task.Id, "summary", truncate(summary, 100), "turns", turn)
			return &workerTaskResult{
				TaskID:      task.Id,
				AgentType:   task.AgentType,
				Status:      "success",
				Summary:     summary,
				Model:       ollama.model,
				LLMTurns:    turn,
				DurationMs:  time.Since(t0).Milliseconds(),
				CompletedAt: time.Now().Unix(),
			}

		case strings.HasPrefix(text, "FAIL:"):
			reason := strings.TrimSpace(text[5:])
			slog.Warn("Worker task FAIL", "id", task.Id, "reason", truncate(reason, 100), "turns", turn)
			return &workerTaskResult{
				TaskID:     task.Id,
				Status:     "failed",
				Error:      reason,
				Model:      ollama.model,
				LLMTurns:   turn,
				DurationMs: time.Since(t0).Milliseconds(),
			}

		default:
			messages = append(messages, ollamaMessage{
				Role:    "user",
				Content: "Respond with CMD:, DONE:, or FAIL:",
			})
		}
	}

	return &workerTaskResult{
		TaskID:     task.Id,
		Status:     "failed",
		Error:      fmt.Sprintf("Exceeded %d turns without DONE/FAIL", maxTurns),
		Model:      ollama.model,
		LLMTurns:   maxTurns,
		DurationMs: time.Since(t0).Milliseconds(),
	}
}

// workerTaskResult is the result of executing a task.
type workerTaskResult struct {
	TaskID      string `json:"task_id"`
	AgentType   string `json:"agent_type,omitempty"`
	Status      string `json:"status"`
	Summary     string `json:"summary,omitempty"`
	Error       string `json:"error,omitempty"`
	Model       string `json:"model,omitempty"`
	LLMTurns    int    `json:"llm_turns"`
	DurationMs  int64  `json:"duration_ms"`
	CompletedAt int64  `json:"completed_at,omitempty"`
}

// workerCommandOutput holds the result of running a shell command.
type workerCommandOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// runWorkerCommand executes a shell command with a 60-second timeout.
func runWorkerCommand(cmd string, taskCtx map[string]interface{}) workerCommandOutput {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, "sh", "-c", cmd)

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
			return workerCommandOutput{Stdout: "", Stderr: "[TIMEOUT after 60s]", ExitCode: -1}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	out := stdout.String()
	if len(out) > 4000 {
		out = out[len(out)-4000:]
	}
	errOut := stderr.String()
	if len(errOut) > 1000 {
		errOut = errOut[len(errOut)-1000:]
	}

	return workerCommandOutput{Stdout: out, Stderr: errOut, ExitCode: exitCode}
}

// buildWorkerPrompt constructs the system prompt for task execution.
func buildWorkerPrompt(goal, contextStr string) string {
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
