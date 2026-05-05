// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcessState represents the state of a background process.
type ProcessState int

const (
	ProcessRunning ProcessState = iota
	ProcessExited
	ProcessKilled
)

func (s ProcessState) String() string {
	switch s {
	case ProcessRunning:
		return "running"
	case ProcessExited:
		return "exited"
	case ProcessKilled:
		return "killed"
	default:
		return "unknown"
	}
}

// BackgroundProc holds state for a running or completed process.
type BackgroundProc struct {
	ID             string
	Command        string
	Cmd            *exec.Cmd
	State          ProcessState
	ExitCode       int
	StartTime      time.Time
	EndTime        time.Time
	Stdout         *RingBuffer
	Stderr         *RingBuffer
	stdoutPolledAt int // TotalWritten offset last time poll read stdout
	stderrPolledAt int // TotalWritten offset last time poll read stderr
	cancel         context.CancelFunc
	done           chan struct{} // closed when process exits
	mu             sync.Mutex
}

// BackgroundProcessConfig holds configuration for background process management.
type BackgroundProcessConfig struct {
	Enabled           bool `yaml:"enabled"`
	MaxProcesses      int  `yaml:"max_processes"`
	MaxOutputKB       int  `yaml:"max_output_kb"`
	MaxAgeMin         int  `yaml:"max_age_min"`
	ApprovalRequired  bool `yaml:"approval_required"`
}

// DefaultBackgroundProcessConfig returns a config with sensible defaults.
func DefaultBackgroundProcessConfig() BackgroundProcessConfig {
	return BackgroundProcessConfig{
		Enabled:      true,
		MaxProcesses:  5,
		MaxOutputKB:   64,
		MaxAgeMin:    30,
	}
}

// processManager manages all background processes globally.
type processManager struct {
	processes sync.Map // session_id → *BackgroundProc
	config    BackgroundProcessConfig
}

var bgManager = &processManager{}

// SetBackgroundProcessConfig configures the global process manager.
func SetBackgroundProcessConfig(cfg BackgroundProcessConfig) {
	if cfg.MaxProcesses <= 0 {
		cfg.MaxProcesses = 5
	}
	if cfg.MaxOutputKB <= 0 {
		cfg.MaxOutputKB = 64
	}
	if cfg.MaxAgeMin <= 0 {
		cfg.MaxAgeMin = 30
	}
	bgManager.config = cfg
}

// generateSessionID generates a random 8-char hex session ID.
func generateSessionID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// countRunning returns the number of currently running processes.
func (pm *processManager) countRunning() int {
	count := 0
	pm.processes.Range(func(_, v interface{}) bool {
		p := v.(*BackgroundProc)
		p.mu.Lock()
		if p.State == ProcessRunning {
			count++
		}
		p.mu.Unlock()
		return true
	})
	return count
}

// cleanupOld removes processes that ended more than maxAgeMin minutes ago.
func (pm *processManager) cleanupOld(maxAgeMin int) {
	cutoff := time.Now().Add(-time.Duration(maxAgeMin) * time.Minute)
	pm.processes.Range(func(k, v interface{}) bool {
		p := v.(*BackgroundProc)
		p.mu.Lock()
		ended := !p.EndTime.IsZero() && p.EndTime.Before(cutoff)
		p.mu.Unlock()
		if ended {
			pm.processes.Delete(k)
		}
		return true
	})
}

// extractSessionID parses "session_id" from the start result string.
func extractSessionID(startResult string) string {
	// Result format: "Process started\nSession ID: abc12345"
	const prefix = "Session ID: "
	idx := strings.Index(startResult, prefix)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(startResult[idx+len(prefix):])
}

// RegisterBackgroundProcessTools registers the BackgroundProcess tool.
func RegisterBackgroundProcessTools(registry *ToolRegistry, cfg BackgroundProcessConfig) {
	SetBackgroundProcessConfig(cfg)
	registry.Register("BackgroundProcess", BackgroundProcessHandler, BackgroundProcessSchema)
}

// BackgroundProcessSchema defines the tool schema for the LLM.
var BackgroundProcessSchema = map[string]interface{}{
	"name":        "BackgroundProcess",
	"description": "Manage background processes — start long-running commands, poll their output, kill or wait for them. Use this instead of Execute for servers, watchers, and build processes that run for more than a few seconds.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: 'start' launches a command in background, 'poll' checks output + status, 'kill' terminates a process, 'wait' blocks until process exits, 'list' shows all running processes",
				"enum":        []string{"start", "poll", "kill", "wait", "list"},
			},
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Session ID (required for poll/kill/wait)",
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Shell command to start (required for 'start' action)",
			},
			"cwd": map[string]interface{}{
				"type":        "string",
				"description": "Working directory for the command (optional, for 'start')",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Max seconds to wait for 'wait' action (default 60)",
			},
		},
		"required": []string{"action"},
	},
}

// BackgroundProcessHandler is the main tool handler.
func BackgroundProcessHandler(args map[string]interface{}) (string, bool) {
	action, _ := args["action"].(string)

	switch action {
	case "start":
		command, _ := args["command"].(string)
		cwd, _ := args["cwd"].(string)
		return handleBgStart(command, cwd)
	case "poll":
		sessionID, _ := args["session_id"].(string)
		return handleBgPoll(sessionID)
	case "kill":
		sessionID, _ := args["session_id"].(string)
		return handleBgKill(sessionID)
	case "wait":
		sessionID, _ := args["session_id"].(string)
		timeout, _ := args["timeout"].(float64)
		timeoutInt := int(timeout)
		if timeoutInt <= 0 {
			timeoutInt = 60
		}
		return handleBgWait(sessionID, timeoutInt)
	case "list":
		return handleBgList()
	default:
		return fmt.Sprintf("Invalid action %q. Use: start, poll, kill, wait, list", action), true
	}
}

func handleBgStart(command, cwd string) (string, bool) {
	if command == "" {
		return "Missing required parameter: command", true
	}

	// Safety: check each sub-command in the chain individually
	// (same logic as Execute — allows safe chaining, blocks dangerous sub-commands)
	subCommands := splitChainedCommands(command)
	for _, sub := range subCommands {
		if isDangerousCommand(sub) {
			return fmt.Sprintf("Command blocked for safety: %q (in chain)", sub), true
		}
		if hasShellInjection(sub) {
			return fmt.Sprintf("Command blocked: shell metacharacters detected in %q. No subshells or unsafe redirects.", sub), true
		}
	}

	// Validate cwd if provided (expand ~ first, same as Execute)
	if cwd != "" {
		if strings.HasPrefix(cwd, "~") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Sprintf("Error expanding home directory: %v", err), true
			}
			cwd = filepath.Join(homeDir, cwd[1:])
		}
		cwd = filepath.Clean(cwd)
		if !IsUnderAllowedDir(cwd) {
			return fmt.Sprintf("Access denied: working directory %q is not in an allowed directory", cwd), true
		}
	}

	// Cleanup old processes first
	bgManager.cleanupOld(bgManager.config.MaxAgeMin)

	// Check process limit
	if bgManager.countRunning() >= bgManager.config.MaxProcesses {
		return fmt.Sprintf("Maximum processes limit (%d) reached. Kill an existing process first.", bgManager.config.MaxProcesses), true
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return "Failed to generate session ID", true
	}

	// Build command
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if cwd != "" {
		cmd.Dir = cwd
	}

	bufSize := bgManager.config.MaxOutputKB * 1024
	stdoutBuf := NewRingBuffer(bufSize)
	stderrBuf := NewRingBuffer(bufSize)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Sprintf("Failed to start process: %v", err), true
	}

	proc := &BackgroundProc{
		ID:        sessionID,
		Command:   command,
		Cmd:       cmd,
		State:     ProcessRunning,
		StartTime: time.Now(),
		Stdout:    stdoutBuf,
		Stderr:    stderrBuf,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	bgManager.processes.Store(sessionID, proc)

	// Monitor process exit in background
	go func() {
		defer close(proc.done)
		waitErr := cmd.Wait()
		proc.mu.Lock()
		if proc.State == ProcessRunning { // not killed
			proc.State = ProcessExited
			if waitErr != nil {
				if exitErr, ok := waitErr.(*exec.ExitError); ok {
					proc.ExitCode = exitErr.ExitCode()
				} else {
					proc.ExitCode = -1
				}
			}
			proc.EndTime = time.Now()
		}
		proc.mu.Unlock()
	}()

	return fmt.Sprintf("Process started\nSession ID: %s", sessionID), false
}

func handleBgPoll(sessionID string) (string, bool) {
	if sessionID == "" {
		return "Missing required parameter: session_id", true
	}

	val, ok := bgManager.processes.Load(sessionID)
	if !ok {
		return fmt.Sprintf("Unknown session ID: %s", sessionID), true
	}

	proc := val.(*BackgroundProc)
	proc.mu.Lock()
	defer proc.mu.Unlock()

	// Get new output since last poll
	stdoutNew := string(proc.Stdout.ReadFrom(proc.stdoutPolledAt))
	stderrNew := string(proc.Stderr.ReadFrom(proc.stderrPolledAt))
	proc.stdoutPolledAt = proc.Stdout.TotalWritten()
	proc.stderrPolledAt = proc.Stderr.TotalWritten()

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Session: %s\nState: %s\n", proc.ID, proc.State))
	if proc.State != ProcessRunning {
		result.WriteString(fmt.Sprintf("Exit Code: %d\n", proc.ExitCode))
	}
	if stdoutNew != "" {
		result.WriteString(fmt.Sprintf("Stdout: %s", stdoutNew))
	}
	if stderrNew != "" {
		result.WriteString(fmt.Sprintf("Stderr: %s", stderrNew))
	}
	if stdoutNew == "" && stderrNew == "" {
		result.WriteString("Output: (no new output)\n")
	}

	return result.String(), false
}

func handleBgKill(sessionID string) (string, bool) {
	if sessionID == "" {
		return "Missing required parameter: session_id", true
	}

	val, ok := bgManager.processes.Load(sessionID)
	if !ok {
		return fmt.Sprintf("Unknown session ID: %s", sessionID), true
	}

	proc := val.(*BackgroundProc)
	proc.mu.Lock()
	if proc.State != ProcessRunning {
		proc.mu.Unlock()
		return fmt.Sprintf("Process already %s (exit code: %d)", proc.State, proc.ExitCode), false
	}
	proc.mu.Unlock()

	// Send SIGTERM to process group
	if proc.Cmd.Process != nil {
		syscall.Kill(-proc.Cmd.Process.Pid, syscall.SIGTERM)
	}

	// Wait up to 5s then SIGKILL
	go func() {
		select {
		case <-proc.done:
			// Process exited from SIGTERM
		case <-time.After(5 * time.Second):
			if proc.Cmd.Process != nil {
				syscall.Kill(-proc.Cmd.Process.Pid, syscall.SIGKILL)
			}
		}
	}()

	// Mark as killed
	proc.mu.Lock()
	proc.State = ProcessKilled
	proc.EndTime = time.Now()
	proc.ExitCode = -1
	proc.mu.Unlock()
	proc.cancel()

	// Wait for the goroutine to finish
	<-proc.done

	return fmt.Sprintf("Process killed\nSession: %s", sessionID), false
}

func handleBgWait(sessionID string, timeout int) (string, bool) {
	if sessionID == "" {
		return "Missing required parameter: session_id", true
	}

	val, ok := bgManager.processes.Load(sessionID)
	if !ok {
		return fmt.Sprintf("Unknown session ID: %s", sessionID), true
	}

	proc := val.(*BackgroundProc)

	// Wait with timeout
	select {
	case <-proc.done:
		// Process exited
	case <-time.After(time.Duration(timeout) * time.Second):
		proc.mu.Lock()
		state := proc.State
		proc.mu.Unlock()
		return fmt.Sprintf("Timeout reached (%ds) — process still %s\nSession: %s", timeout, state, sessionID), true
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	// Return remaining output since last poll
	stdoutNew := string(proc.Stdout.ReadFrom(proc.stdoutPolledAt))
	stderrNew := string(proc.Stderr.ReadFrom(proc.stderrPolledAt))
	proc.stdoutPolledAt = proc.Stdout.TotalWritten()
	proc.stderrPolledAt = proc.Stderr.TotalWritten()

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Session: %s\nState: %s\nExit Code: %d\n", proc.ID, proc.State, proc.ExitCode))
	if stdoutNew != "" {
		result.WriteString(fmt.Sprintf("Stdout: %s", stdoutNew))
	}
	if stderrNew != "" {
		result.WriteString(fmt.Sprintf("Stderr: %s", stderrNew))
	}

	return result.String(), false
}

func handleBgList() (string, bool) {
	bgManager.cleanupOld(bgManager.config.MaxAgeMin)

	var procs []string
	bgManager.processes.Range(func(_, v interface{}) bool {
		p := v.(*BackgroundProc)
		p.mu.Lock()
		state := p.State
		exitCode := p.ExitCode
		runtime := time.Since(p.StartTime).Truncate(time.Second)
		p.mu.Unlock()

		entry := fmt.Sprintf("  %s: %s (%s, running %s",
			p.ID, p.Command, state, runtime)
		if state != ProcessRunning {
			entry += fmt.Sprintf(", exit code %d", exitCode)
		}
		entry += ")"
		procs = append(procs, entry)
		return true
	})

	if len(procs) == 0 {
		return "No background processes", false
	}

	return fmt.Sprintf("Background processes (%d):\n%s", len(procs), strings.Join(procs, "\n")), false
}

// CleanupBackgroundProcesses kills all running processes. Called on agent shutdown.
func CleanupBackgroundProcesses() {
	bgManager.processes.Range(func(_, v interface{}) bool {
		p := v.(*BackgroundProc)
		p.mu.Lock()
		if p.State == ProcessRunning && p.Cmd.Process != nil {
			syscall.Kill(-p.Cmd.Process.Pid, syscall.SIGTERM)
			p.State = ProcessKilled
			p.EndTime = time.Now()
		}
		p.mu.Unlock()
		p.cancel()
		return true
	})
}