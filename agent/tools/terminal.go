package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Execute runs a shell command and returns stdout/stderr.
func Execute(args map[string]interface{}) (string, bool) {
	command := strArg(args, "command", "")
	timeout := intArg(args, "timeout", 30)
	if command == "" {
		return "command is required", true
	}

	// Check for dangerous commands
	if isDangerousCommand(command) {
		return "dangerous command detected: requires approval", true
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "command timed out", true
		}
		return fmt.Sprintf("command failed: %v\n%s", err, string(output)), true
	}

	return string(output), false
}

// RegisterTerminalTools adds all terminal tool handlers to the registry.
func RegisterTerminalTools(registry *ToolRegistry) {
	registry.Register("Execute", Execute, ExecuteSchema)
}

// isDangerousCommand checks if a command matches dangerous patterns.
func isDangerousCommand(command string) bool {
	dangerousPatterns := []string{
		"rm -rf",
		"sudo ",
		"mkfs",
		"dd ",
		":(){:|:&};:",
	}

	cmdLower := strings.ToLower(command)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(cmdLower, pattern) {
			return true
		}
	}

	return false
}

// Tool schema for terminal execution
var ExecuteSchema = map[string]interface{}{
	"name":        "Execute",
	"description": "Execute a shell command.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Shell command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default: 30)",
				"default":     30,
			},
		},
		"required": []string{"command"},
	},
}
