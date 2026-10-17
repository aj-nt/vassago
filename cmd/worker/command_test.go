// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"strings"
	"testing"
)

func TestRunCommand_Echo(t *testing.T) {
	output := runCommand("echo hello", nil)

	if output.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", output.ExitCode)
	}
	if !strings.Contains(output.Stdout, "hello") {
		t.Errorf("expected 'hello' in stdout, got '%s'", output.Stdout)
	}
	if output.Stderr != "" {
		t.Errorf("expected empty stderr, got '%s'", output.Stderr)
	}
}

func TestRunCommand_Stderr(t *testing.T) {
	output := runCommand("echo error >&2", nil)

	if output.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", output.ExitCode)
	}
	if !strings.Contains(output.Stderr, "error") {
		t.Errorf("expected 'error' in stderr, got '%s'", output.Stderr)
	}
}

func TestRunCommand_ExitCode(t *testing.T) {
	output := runCommand("exit 42", nil)

	if output.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", output.ExitCode)
	}
}

func TestRunCommand_WorkingDirectory(t *testing.T) {
	taskCtx := map[string]interface{}{"repo_path": "/tmp"}
	output := runCommand("pwd", taskCtx)

	if output.ExitCode != 0 {
		t.Fatalf("command failed: %s", output.Stderr)
	}
	if !strings.Contains(strings.TrimSpace(output.Stdout), "/tmp") {
		t.Errorf("expected CWD to contain /tmp, got '%s'", output.Stdout)
	}
}

func TestRunCommand_NonexistentCommand(t *testing.T) {
	output := runCommand("nonexistent_command_xyzzy_12345", nil)

	// Shell returns 127 for "command not found"
	if output.ExitCode != 127 {
		t.Errorf("expected exit code 127 for nonexistent command, got %d", output.ExitCode)
	}
}

func TestRunCommand_LongOutput(t *testing.T) {
	// Generate >4000 chars of output and verify truncation
	cmd := "python3 -c \"print('x' * 5000)\""
	output := runCommand(cmd, nil)

	if output.ExitCode != 0 && output.ExitCode != -1 {
		// Python3 may not be available — skip if command fails for that reason
		if strings.Contains(output.Stderr, "python3") {
			t.Skip("python3 not available")
		}
		t.Fatalf("command failed: %s", output.Stderr)
	}

	if len(output.Stdout) > 4000 {
		t.Errorf("stdout should be truncated to 4000 chars, got %d", len(output.Stdout))
	}
}

func TestCommandOutput_ZeroValue(t *testing.T) {
	var co CommandOutput
	if co.ExitCode != 0 {
		t.Errorf("zero value ExitCode should be 0, got %d", co.ExitCode)
	}
	if co.Stdout != "" {
		t.Errorf("zero value Stdout should be empty, got '%s'", co.Stdout)
	}
}
