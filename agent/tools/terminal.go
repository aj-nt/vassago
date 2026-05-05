// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Execute runs a terminal command and returns its output.
// If a context is provided via the "context" key, the command can be cancelled.
func Execute(args map[string]interface{}) (string, bool) {
	command, ok := args["command"].(string)
	if !ok || command == "" {
		return "Execute requires a 'command' argument", true
	}

	// Split chained commands and validate each individually.
	// This allows safe chaining (git fetch && git rebase) while blocking
	// chains that include dangerous commands.
	subCommands := splitChainedCommands(command)
	for _, sub := range subCommands {
		if isDangerousCommand(sub) {
			return fmt.Sprintf("Command blocked for safety: %q (in chain). This command matches a dangerous pattern.", sub), true
		}
		if hasShellInjection(sub) {
			return fmt.Sprintf("Command blocked: shell metacharacters detected in %q. No subshells or unsafe redirects.", sub), true
		}
	}

	cmd := exec.Command("sh", "-c", command)

	// Set working directory if provided
	if cwd, ok := args["cwd"].(string); ok && cwd != "" {
		// Expand ~ to home directory
		if strings.HasPrefix(cwd, "~") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Sprintf("Error expanding home directory: %v", err), true
			}
			cwd = filepath.Join(homeDir, cwd[1:])
		}

		// Validate that cwd is within allowed directories
		if !IsUnderAllowedDir(cwd) {
			return fmt.Sprintf("Access denied: working directory %q is not in an allowed directory", cwd), true
		}

		cmd.Dir = cwd
	}

	// Support context cancellation if provided
	if ctx, ok := args["_context"].(context.Context); ok {
		cmd.Cancel = func() error {
			return cmd.Process.Kill()
		}
		if deadline, ok := args["_deadline"].(time.Time); ok {
			timeout := time.Until(deadline)
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
		}
		cmd.WaitDelay = 3 * time.Second // Give process 3s to clean up after Kill
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Distinguish between signal-killed and other errors
		if ctx, ok := args["_context"].(context.Context); ok {
			select {
			case <-ctx.Done():
				return "Command interrupted", true
			default:
			}
		}
		return fmt.Sprintf("Error: %v\nOutput: %s", err, string(output)), true
	}

	// Truncate output for command results
	const maxResultChars = 4000
	return truncateOutput(string(output), maxResultChars), false
}

// splitChainedCommands splits a command string on &&, ||, and ; separators,
// returning each individual sub-command. It respects quoted strings so that
// separators inside quotes are not treated as chain operators.
func splitChainedCommands(command string) []string {
	var parts []string
	var current strings.Builder
	quoteChar := byte(0) // 0=none, '\''=single, '"'=double
	escaped := false

	for i := 0; i < len(command); i++ {
		ch := command[i]

		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			current.WriteByte(ch)
			escaped = true
			continue
		}

		// Track quote state
		if ch == '\'' && quoteChar != '"' {
			if quoteChar == '\'' {
				quoteChar = 0
			} else {
				quoteChar = '\''
			}
			current.WriteByte(ch)
			continue
		}
		if ch == '"' && quoteChar != '\'' {
			if quoteChar == '"' {
				quoteChar = 0
			} else {
				quoteChar = '"'
			}
			current.WriteByte(ch)
			continue
		}

		// Only split on separators when not inside quotes
		if quoteChar == 0 {
			// Check for && and ||
			if ch == '&' && i+1 < len(command) && command[i+1] == '&' {
				part := strings.TrimSpace(current.String())
				if part != "" {
					parts = append(parts, part)
				}
				current.Reset()
				i++ // skip second &
				continue
			}
			if ch == '|' && i+1 < len(command) && command[i+1] == '|' {
				part := strings.TrimSpace(current.String())
				if part != "" {
					parts = append(parts, part)
				}
				current.Reset()
				i++ // skip second |
				continue
			}
			// Check for ; (but not inside && or ||)
			if ch == ';' && !(i > 0 && (command[i-1] == '&' || command[i-1] == '|')) {
				part := strings.TrimSpace(current.String())
				if part != "" {
					parts = append(parts, part)
				}
				current.Reset()
				continue
			}
		}

		current.WriteByte(ch)
	}

	// Add final part
	part := strings.TrimSpace(current.String())
	if part != "" {
		parts = append(parts, part)
	}

	// If no splitting occurred, return the whole command
	if len(parts) == 0 {
		return []string{command}
	}
	return parts
}

// isDangerousCommand checks if a command matches dangerous patterns.
// Blocklist approach: known-destructive commands are rejected.
// This is applied to each sub-command in a chain individually.
func isDangerousCommand(command string) bool {
	// Destructive file operations
	dangerousPatterns := []string{
		// File destruction
		"rm -rf",
		"rm -r ",
		"rmdir",

		// Privilege escalation
		"sudo ",
		"su ",
		"chown",
		"chmod 777",
		"chmod 000",
		"chmod a+w",

		// Disk/filesystem destruction
		"mkfs",
		"fdisk",
		"format",
		"dd if=",
		"dd of=",

		// System control
		"shutdown",
		"reboot",
		"halt",
		"poweroff",
		"init 0",
		"init 6",
		"kill -9 1",
		"killall",

		// Fork bomb
		":(){:|:&};:",

		// Network exfiltration / reverse shells
		"nc -l",
		"ncat -l",
		"nc -e",
		"/dev/tcp/",

		// Package/dependency attacks — pipes to interpreters
		"curl|sh",
		"curl|bash",
		"wget|sh",
		"wget|bash",
		"curl | sh",
		"curl | bash",
		"wget | sh",
		"wget | bash",
		"| sh",
		"| bash",
		"| zsh",
		"| fish",
		"| python",
		"| perl",

		// Sensitive file access
		"passwd",
		"shadow",
		"/etc/passwd",
		"/etc/shadow",
		"ssh-keygen",
		"ssh-keysign",

		// System modification
		"launchctl unload",
		"launchctl remove",
		"defaults write",
		"csrutil",
		"nvram",
		"diskutil erase",
		"diskutil partition",
	}

	cmdLower := strings.ToLower(command)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(cmdLower, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// hasShellInjection detects unsafe shell metacharacters: subshell execution
// and dangerous redirects. Command chaining (&&, ||, ;) is now handled by
// splitChainedCommands + per-sub-command validation, so those are no longer
// blocked here.
func hasShellInjection(command string) bool {
	// Subshell substitution — always block
	subshellPatterns := []string{
		"$(",   // command substitution: $(cmd)
		"`",    // backtick substitution: `cmd`
	}

	for _, pattern := range subshellPatterns {
		if strings.Contains(command, pattern) {
			return true
		}
	}

	// Dangerous redirects
	// Block redirects to system files/devices
	redirectPatterns := []string{
		"> /etc",
		"> /dev",
		">>",
	}

	// Allow safe redirects (overwrite and append to safe locations)
	safeRedirects := []string{
		"> /dev/null",
		"> /tmp/",
		">> /tmp/",
		"2>/dev/null",
		"2> /dev/null",
	}

	for _, pattern := range redirectPatterns {
		if strings.Contains(command, pattern) {
			isSafe := false
			for _, safe := range safeRedirects {
				if strings.Contains(command, safe) {
					isSafe = true
					break
				}
			}
			// Additional check: if > /dev appears but is part of > /dev/null, it's safe
			// This handles the case where "> /dev" matches "> /dev/null"
			if pattern == "> /dev" && strings.Contains(command, "> /dev/null") {
				isSafe = true
			}
			if !isSafe {
				return true
			}
		}
	}

	return false
}

// RegisterTerminalTools adds all terminal tool handlers to the registry.
func RegisterTerminalTools(registry *ToolRegistry) {
	registry.Register("Execute", Execute, ExecuteSchema)
}

var ExecuteSchema = map[string]interface{}{
	"name":        "Execute",
	"description": "Execute a terminal command and return its output. Dangerous commands are blocked. Command chaining with &&, ||, and ; is allowed for safe commands.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The command to execute. Chaining with &&, ||, and ; is supported for safe commands.",
			},
			"cwd": map[string]interface{}{
				"type":        "string",
				"description": "Working directory for the command. Defaults to the agent's current directory.",
			},
		},
		"required": []string{"command"},
	},
}