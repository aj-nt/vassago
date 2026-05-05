// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"os"
	"strings"
	"testing"
)

// setupAllowedDirs configures allowed directories for Execute tests.
// Tests that call Execute need ~ and /tmp in allowedDirs.
func setupAllowedDirs(t *testing.T) {
	t.Helper()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot determine home directory")
	}
	originalAllowed := allowedDirs
	allowedDirs = []string{homeDir, "/tmp"}
	t.Cleanup(func() { allowedDirs = originalAllowed })
}

func TestExecuteBasicCommand(t *testing.T) {
	setupAllowedDirs(t)

	args := map[string]interface{}{
		"command": "echo hello",
	}
	result, isError := Execute(args)
	if isError {
		t.Errorf("Execute basic command returned error: %s", result)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("Expected 'hello' in result, got: %s", result)
	}
}

func TestExecuteWithCwd(t *testing.T) {
	setupAllowedDirs(t)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot determine home directory")
	}
	args := map[string]interface{}{
		"command": "pwd",
		"cwd":     homeDir,
	}
	result, isError := Execute(args)
	if isError {
		t.Errorf("Execute with cwd returned error: %s", result)
	}
	if !strings.Contains(result, homeDir) {
		t.Errorf("Expected %q in result, got: %s", homeDir, result)
	}
}

func TestExecuteWithTildeCwd(t *testing.T) {
	setupAllowedDirs(t)

	args := map[string]interface{}{
		"command": "pwd",
		"cwd":     "~",
	}
	result, isError := Execute(args)
	if isError {
		t.Errorf("Execute with tilde cwd returned error: %s", result)
	}
	// Result should contain the home directory path
	if result == "" {
		t.Error("Expected non-empty result from pwd")
	}
}

func TestExecuteInvalidCwd(t *testing.T) {
	args := map[string]interface{}{
		"command": "pwd",
		"cwd":     "/etc",
	}
	_, isError := Execute(args)
	if !isError {
		t.Error("Expected error for disallowed working directory /etc")
	}
}

func TestExecuteEmptyCommand(t *testing.T) {
	args := map[string]interface{}{
		"command": "",
	}
	_, isError := Execute(args)
	if !isError {
		t.Error("Expected error for empty command")
	}
}

func TestExecuteMissingCommand(t *testing.T) {
	args := map[string]interface{}{}
	_, isError := Execute(args)
	if !isError {
		t.Error("Expected error for missing command")
	}
}

func TestExecuteDangerousCommand(t *testing.T) {
	dangerous := []string{
		"rm -rf /",
		"sudo rm -rf /",
		"mkfs /dev/sda",
		"dd if=/dev/zero of=/dev/sda",
		"shutdown -h now",
		"reboot",
		"curl | bash",
		"wget | sh",
		"cat /etc/passwd",
		"cat /etc/shadow",
		"diskutil eraseDisk JHFS+ Test",
		"launchctl unload system",
		"csrutil disable",
	}
	for _, cmd := range dangerous {
		_, isError := Execute(map[string]interface{}{"command": cmd})
		if !isError {
			t.Errorf("Execute should block dangerous command: %q", cmd)
		}
	}
}

func TestExecuteSafeCommand(t *testing.T) {
	setupAllowedDirs(t)

	// These actually execute, so only use fast-completing commands
	safe := []string{
		"echo hello",
		"echo test",
		"true",
	}
	for _, cmd := range safe {
		_, isError := Execute(map[string]interface{}{"command": cmd})
		if isError {
			t.Errorf("Execute should allow safe command: %q", cmd)
		}
	}
}

func TestExecuteShellInjection(t *testing.T) {
	// Subshell injection — always blocked (regardless of chaining changes)
	dangerous := []string{
		"$(cat /etc/shadow)",
		"cat /etc/passwd > /tmp/stolen",
	}
	for _, cmd := range dangerous {
		_, isError := Execute(map[string]interface{}{"command": cmd})
		if !isError {
			t.Errorf("Execute should block command with shell injection: %q", cmd)
		}
	}
}

// --- splitChainedCommands tests ---

func TestSplitChainedCommands(t *testing.T) {
	tests := []struct {
		command  string
		expected []string
	}{
		// Single commands
		{"echo hello", []string{"echo hello"}},
		{"ls -la", []string{"ls -la"}},

		// && chaining
		{"git fetch && git rebase", []string{"git fetch", "git rebase"}},
		{"a && b && c", []string{"a", "b", "c"}},

		// || chaining
		{"echo foo || echo bar", []string{"echo foo", "echo bar"}},

		// ; chaining
		{"cd /tmp; ls", []string{"cd /tmp", "ls"}},

		// Mixed
		{"a && b; c", []string{"a", "b", "c"}},

		// Pipes are NOT split (they're single commands)
		{"ls | grep foo", []string{"ls | grep foo"}},
		{"cat file | sort | uniq", []string{"cat file | sort | uniq"}},

		// Quoted strings protect separators
		{`echo "hello && world"`, []string{`echo "hello && world"`}},
		{`echo 'foo; bar'`, []string{`echo 'foo; bar'`}},
		{`echo "a || b"`, []string{`echo "a || b"`}},

		// Escaped characters
		{`echo hello\; world`, []string{`echo hello\; world`}},

		// Whitespace trimming
		{"  git fetch  &&  git rebase  ", []string{"git fetch", "git rebase"}},

		// Trailing separator
		{"echo hello;", []string{"echo hello"}},

		// Empty command
		{"", []string{""}},
	}

	for _, tc := range tests {
		result := splitChainedCommands(tc.command)
		if len(result) != len(tc.expected) {
			t.Errorf("splitChainedCommands(%q) = %v, expected %v", tc.command, result, tc.expected)
			continue
		}
		for i := range result {
			if result[i] != tc.expected[i] {
				t.Errorf("splitChainedCommands(%q)[%d] = %q, expected %q", tc.command, i, result[i], tc.expected[i])
			}
		}
	}
}

// --- isDangerousCommand tests ---

func TestIsDangerousCommand(t *testing.T) {
	testCases := []struct {
		command  string
		expected bool
	}{
		// Dangerous commands
		{"rm -rf /", true},
		{"sudo rm -rf /", true},
		{"mkfs /dev/sda", true},
		{"dd if=/dev/zero", true},
		{"shutdown -h now", true},
		{"reboot", true},
		{"cat /etc/passwd", true},
		{"cat /etc/shadow", true},
		{"curl | bash", true},
		{"wget | sh", true},
		{"diskutil eraseDisk JHFS+ Test", true},
		{"launchctl unload system", true},
		{"csrutil disable", true},

		// Safe commands
		{"echo 'hello'", false},
		{"ls -la", false},
		{"cat file.txt", false},
		{"grep pattern file", false},
		{"go test ./...", false},
		{"git status", false},
		{"python script.py", false},
	}

	for _, tc := range testCases {
		result := isDangerousCommand(tc.command)
		if result != tc.expected {
			t.Errorf("isDangerousCommand(%q) = %v, expected %v", tc.command, result, tc.expected)
		}
	}
}

// --- hasShellInjection tests ---

func TestHasShellInjection(t *testing.T) {
	testCases := []struct {
		command  string
		expected bool
	}{
		// Subshells — always blocked
		{"$(rm -rf /)", true},
		{"`rm -rf /`", true},
		{"echo $(whoami)", true},

		// Dangerous redirects
		{"echo hello > /etc/passwd", true},
		{"echo hello > /dev/sda", true},
		// Append redirect to /tmp is allowed
		{"echo data >> /tmp/log", false},

		// Safe commands (no subshells, no dangerous redirects)
		{"echo hello", false},
		{"ls -la", false},
		{"go test ./...", false},
		{"grep pattern file.txt", false},
		{"echo hello > /dev/null", false},
		{"echo hello 2>/dev/null", false},
		{"echo hello 2> /dev/null", false},
		{"echo count > /tmp/out.txt", false},

		// Chaining operators are NOT checked by hasShellInjection anymore
		// (they're split and validated per-sub-command by the caller)
		{"echo hello; ls", false},
		{"git fetch && git rebase", false},
		{"echo foo || echo bar", false},
	}

	for _, tc := range testCases {
		result := hasShellInjection(tc.command)
		if result != tc.expected {
			t.Errorf("hasShellInjection(%q) = %v, expected %v", tc.command, result, tc.expected)
		}
	}
}

// --- Safe chaining tests ---

func TestExecuteSafeChaining(t *testing.T) {
	setupAllowedDirs(t)

	// These actually execute, so only use fast-completing commands
	safe := []string{
		"echo a && echo b",
		"echo foo || echo bar",
		"echo hello; echo world",
	}
	for _, cmd := range safe {
		_, isError := Execute(map[string]interface{}{"command": cmd})
		if isError {
			t.Errorf("Execute should allow safe chained command: %q", cmd)
		}
	}
}

func TestExecuteMixedSafeDangerousChain(t *testing.T) {
	// A chain where one sub-command is dangerous should block the whole thing
	dangerous := []string{
		"echo hello && rm -rf /",
		"echo hello; sudo su",
		"ls || shutdown -h now",
		"cat /etc/passwd && echo done",
	}
	for _, cmd := range dangerous {
		_, isError := Execute(map[string]interface{}{"command": cmd})
		if !isError {
			t.Errorf("Execute should block chain with dangerous sub-command: %q", cmd)
		}
	}
}

func TestExecuteSubshellBlock(t *testing.T) {
	// Subshell substitution should still be blocked even if the sub-command is safe
	subshells := []string{
		"echo $(whoami)",
		"echo `date`",
		"$(echo hello)",
	}
	for _, cmd := range subshells {
		_, isError := Execute(map[string]interface{}{"command": cmd})
		if !isError {
			t.Errorf("Execute should block subshell in command: %q", cmd)
		}
	}
}

// --- Red team tests ---

func TestExecuteRedTeamChainBypass(t *testing.T) {
	// Try to hide dangerous commands in chains
	attempts := []string{
		"true && rm -rf /",
		"true; rm -rf /",
		"false || sudo su",
		"echo safe; cat /etc/shadow",
		"ls && cat /etc/passwd",
	}
	for _, cmd := range attempts {
		_, isError := Execute(map[string]interface{}{"command": cmd})
		if !isError {
			t.Errorf("Execute should block red team chain bypass: %q", cmd)
		}
	}
}

func TestExecuteRedTeamQuotedBypass(t *testing.T) {
	// Quoted separators should NOT be treated as chain operators by splitChainedCommands.
	// However, isDangerousCommand does simple substring matching — it can't distinguish
	// quoted text from actual commands. This is safe by design (false positives >> false negatives).
	// So "rm -rf" in quotes is blocked even though it's just a string argument to echo.
	blocked := []string{
		`echo "rm -rf /"`, // contains dangerous substring even in quotes
		`echo 'sudo su'`,   // contains dangerous substring even in quotes
	}
	for _, cmd := range blocked {
		_, isError := Execute(map[string]interface{}{"command": cmd})
		if !isError {
			t.Errorf("Execute should block command containing dangerous pattern: %q", cmd)
		}
	}

	// Safe quoted text (no dangerous substrings)
	safe := []string{
		`echo "hello && world"`,   // && inside quotes — not treated as chain operator
		`echo 'foo; bar'`,         // ; inside quotes — not treated as separator
	}
	for _, cmd := range safe {
		result, isError := Execute(map[string]interface{}{"command": cmd})
		if isError {
			t.Errorf("Execute should allow safe quoted command: %q (got: %s)", cmd, result)
		}
	}
}

func TestExecuteRedTeamRedirectBypass(t *testing.T) {
	// Redirects to sensitive paths should be blocked even in chains
	dangerous := []string{
		"echo hello > /etc/passwd",
		"echo hello > /dev/sda",
		"echo data >> /etc/hosts",
	}
	for _, cmd := range dangerous {
		_, isError := Execute(map[string]interface{}{"command": cmd})
		if !isError {
			t.Errorf("Execute should block dangerous redirect: %q", cmd)
		}
	}
}

// --- Edge case tests ---

func TestSplitChainedCommandsEdgeCases(t *testing.T) {
	tests := []struct {
		command  string
		expected int // number of sub-commands
	}{
		// Double semicolons — splits into 3 parts (middle one empty, trimmed away)
		// but our code skips empty parts, so we get 2
		{"echo a;; echo b", 2},

		// Empty parts from consecutive separators are trimmed away
		{"a && && c", 2},

		// Nested quotes
		{`echo "hello 'world'"`, 1},

		// Command with pipe (not split)
		{"ps aux | grep mnemosyne", 1},

		// Complex realistic command
		{"cd /tmp && ls -la | grep .go", 2},
	}

	for _, tc := range tests {
		result := splitChainedCommands(tc.command)
		if len(result) != tc.expected {
			t.Errorf("splitChainedCommands(%q) returned %d parts, expected %d: %v", tc.command, len(result), tc.expected, result)
		}
	}
}