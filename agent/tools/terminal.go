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

	if isDangerousCommand(command) {
		return fmt.Sprintf("Command blocked for safety: %q. This command matches a dangerous pattern.", command), true
	}

	// Check for shell injection metacharacters that allow command chaining
	if hasShellInjection(command) {
		return fmt.Sprintf("Command blocked: shell metacharacters detected in %q. Use single commands only — no chaining, pipes to shell, or subshells.", command), true
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

// isDangerousCommand checks if a command matches dangerous patterns.
// Blocklist approach: known-destructive commands are rejected.
// This is supplemented by hasShellInjection() for chaining/pipe attacks.
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

		// Package/dependency attacks
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

// hasShellInjection detects shell metacharacters that allow command chaining,
// subshells, or piping to interpreters. These let an LLM bypass the single-command
// restriction by appending destructive commands.
func hasShellInjection(command string) bool {
	// Allow safe pipes (e.g., grep pattern | sort, ls | wc -l)
	// Block pipes to shells/interpreters (handled above in isDangerousCommand)
	// Block command chaining and subshells

	injectionPatterns := []string{
		";",      // command separator: cmd1; cmd2
		"&&",     // AND chaining: cmd1 && cmd2
		"||",     // OR chaining: cmd1 || cmd2
		"$(",     // command substitution: $(cmd)
		"`",      // backtick substitution: `cmd`
		"> /etc", // redirect to system files
		"> /dev", // redirect to devices (except /dev/null below)
		">>",     // append redirect — allow for legitimate use? Block for safety.
	}

	// Allow specific safe redirects
	safeExceptions := []string{
		"> /dev/null",
		"> /tmp/",
		"2>/dev/null",
		"2> /dev/null",
	}

	for _, pattern := range injectionPatterns {
		if strings.Contains(command, pattern) {
			// Check if it's a safe exception
			isException := false
			for _, ex := range safeExceptions {
				if strings.Contains(command, ex) {
					isException = true
					break
				}
			}
			if !isException {
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
	"description": "Execute a terminal command and return its output. Dangerous commands and shell chaining are blocked for safety.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The command to execute",
			},
			"cwd": map[string]interface{}{
				"type":        "string",
				"description": "Working directory for the command. Defaults to the agent's current directory.",
			},
		},
		"required": []string{"command"},
	},
}