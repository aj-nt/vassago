// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// === File Tool Tests ===

func TestFileRead(t *testing.T) {
	testContent := "Hello, world!\nThis is a test file."
	testFile := "test_file.txt"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	result, isError := FileRead(map[string]interface{}{"path": testFile})
	if isError {
		t.Errorf("FileRead returned error: %s", result)
	}

	if result != testContent {
		t.Errorf("Expected content %q, got %q", testContent, result)
	}
}

func TestFileReadInvalidPath(t *testing.T) {
	_, isError := FileRead(map[string]interface{}{"path": "../test.txt"})
	if !isError {
		t.Error("FileRead should return error for path traversal")
	}
}

func TestFileReadDeviceFile(t *testing.T) {
	_, isError := FileRead(map[string]interface{}{"path": "/dev/null"})
	if !isError {
		t.Error("FileRead should block device files")
	}
}

func TestFileReadProcFile(t *testing.T) {
	_, isError := FileRead(map[string]interface{}{"path": "/proc/self/status"})
	if !isError {
		t.Error("FileRead should block /proc files")
	}
}

func TestFileReadMissingPath(t *testing.T) {
	_, isError := FileRead(map[string]interface{}{})
	if !isError {
		t.Error("FileRead should return error when path is missing")
	}
}

func TestIsPathSafeTraversal(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"../etc/passwd", false},
		{"foo/../../../etc/passwd", false},
		{"/tmp/test.txt", true},
		{"/dev/null", false},
		{"/dev/sda", false},
		{"/proc/self/status", false},
		{"/sys/fs/selinux", false},
		{"test.txt", true},
	}

	for _, tc := range tests {
		result := isPathSafe(tc.path)
		if result != tc.expected {
			t.Errorf("isPathSafe(%q) = %v, expected %v", tc.path, result, tc.expected)
		}
	}
}

func TestIsPathSafeWithAllowlist(t *testing.T) {
	// Set up allowlist
	SetFileConfig([]string{"/tmp", "."}, nil)
	defer SetFileConfig(nil, nil) // Reset

	tests := []struct {
		path     string
		expected bool
	}{
		{"/tmp/test.txt", true},
		{"/tmp/subdir/file.txt", true},
		{"/etc/passwd", false},
		{"/home/user/.ssh/id_rsa", false},
		{"test.txt", true}, // relative — resolves to cwd which should be under "."
	}

	for _, tc := range tests {
		result := isPathSafe(tc.path)
		if result != tc.expected {
			t.Errorf("isPathSafe(%q) with allowlist = %v, expected %v", tc.path, result, tc.expected)
		}
	}
}

func TestIsPathSafeSymlink(t *testing.T) {
	// Create a symlink pointing outside allowed dirs
	tmpDir := t.TempDir()
	linkPath := filepath.Join(tmpDir, "evil_link")
	// Only create the symlink if we can (may need permissions)
	err := os.Symlink("/etc/passwd", linkPath)
	if err != nil {
		t.Skip("Cannot create symlink for testing")
	}
	defer os.Remove(linkPath)

	// Without allowlist, symlinks are allowed
	SetFileConfig(nil, nil)
	if !isPathSafe(linkPath) {
		t.Error("isPathSafe without allowlist should allow symlinks (dev mode)")
	}

	// With allowlist, symlink resolving outside should be blocked
	SetFileConfig([]string{tmpDir}, nil)
	defer SetFileConfig(nil, nil)
	if isPathSafe(linkPath) {
		t.Error("isPathSafe with allowlist should block symlink pointing outside allowed dir")
	}
}

func TestIsWriteAllowedWithAllowlist(t *testing.T) {
	SetFileConfig([]string{"/tmp", "."}, nil)
	defer SetFileConfig(nil, nil)

	if !isWriteAllowed("/tmp/test.txt") {
		t.Error("Write to /tmp should be allowed")
	}
	if isWriteAllowed("/etc/hosts") {
		t.Error("Write to /etc should be blocked")
	}
}

func TestFileWrite(t *testing.T) {
	testFile := "test_write.txt"
	defer os.Remove(testFile)

	result, isError := FileWrite(map[string]interface{}{"path": testFile, "content": "test"})
	if isError {
		t.Errorf("FileWrite should allow writes: %s", result)
	}

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read written file: %v", err)
	}
	if string(content) != "test" {
		t.Errorf("Expected content 'test', got %q", string(content))
	}
}

func TestFileWriteMissingArgs(t *testing.T) {
	_, isError := FileWrite(map[string]interface{}{"path": "test.txt"})
	if !isError {
		t.Error("FileWrite should error without content")
	}
	_, isError = FileWrite(map[string]interface{}{"content": "test"})
	if !isError {
		t.Error("FileWrite should error without path")
	}
}

func TestFileSearch(t *testing.T) {
	testFiles := []string{"test1.txt", "test2.log", "other.txt"}
	for _, file := range testFiles {
		if err := os.WriteFile(file, []byte("test"), 0644); err != nil { //nolint:errcheck
			t.Fatalf("failed to create test file %s: %v", file, err)
		}
		defer os.Remove(file)
	}

	result, isError := FileSearch(map[string]interface{}{"pattern": "test*.txt"})
	if isError {
		t.Errorf("FileSearch returned error: %s", result)
	}

	if !strings.Contains(result, "test1.txt") {
		t.Errorf("Expected to find test1.txt in results: %s", result)
	}
}

func TestFileSearchInvalidDirectory(t *testing.T) {
	_, isError := FileSearch(map[string]interface{}{"pattern": "*.txt", "directory": "../"})
	if !isError {
		t.Error("FileSearch should return error for path traversal directory")
	}
}

func TestIsDevicePath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/dev/null", true},
		{"/dev/sda", true},
		{"/proc/self/status", true},
		{"/sys/kernel/notes", true},
		{"/tmp/file.txt", false},
		{"/home/user/.bashrc", false},
	}
	for _, tc := range tests {
		result := isDevicePath(tc.path)
		if result != tc.expected {
			t.Errorf("isDevicePath(%q) = %v, expected %v", tc.path, result, tc.expected)
		}
	}
}

func TestIsUnderAllowedDir(t *testing.T) {
	// Use a real temp dir that exists (avoids symlink issues on macOS where /tmp → /private/tmp)
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	SetFileConfig([]string{tmpDir, "/nonexistent/path"}, nil)
	defer SetFileConfig(nil, nil)

	tests := []struct {
		path     string
		expected bool
	}{
		{filepath.Join(tmpDir, "file.txt"), true},
		{filepath.Join(subDir, "deep", "file.txt"), true},
		{tmpDir, true},
		{"/etc/passwd", false},
		{"/home/user/.ssh/id_rsa", false},
		{tmpDir + "ish/main.go", false}, // prefix match must be directory boundary
	}
	for _, tc := range tests {
		result := IsUnderAllowedDir(tc.path)
		if result != tc.expected {
			t.Errorf("isUnderAllowedDir(%q) = %v, expected %v", tc.path, result, tc.expected)
		}
	}
}

// === Terminal Tool Tests ===

func TestExecute(t *testing.T) {
	result, isError := Execute(map[string]interface{}{"command": "echo 'Hello, world!'"})
	if isError {
		t.Errorf("Execute returned error: %s", result)
		return
	}
	if result == "" {
		t.Error("Execute returned empty result")
	}
}

func TestExecuteDangerousCommand(t *testing.T) {
	_, isError := Execute(map[string]interface{}{"command": "rm -rf /"})
	if !isError {
		t.Error("Execute should block dangerous commands")
	}
}

func TestExecuteMissingCommand(t *testing.T) {
	_, isError := Execute(map[string]interface{}{})
	if !isError {
		t.Error("Execute should error without command")
	}
}

func TestIsDangerousCommand(t *testing.T) {
	testCases := []struct {
		command  string
		expected bool
	}{
		// Original blocklist
		{"rm -rf /", true},
		{"sudo apt update", true},
		{"mkfs.ext4 /dev/sda1", true},
		{"dd if=/dev/zero of=/dev/sda", true},
		{":(){:|:&};:", true},

		// Expanded blocklist
		{"chmod 777 /etc/passwd", true},
		{"chown root:root /tmp/evil", true},
		{"shutdown -h now", true},
		{"reboot", true},
		{"kill -9 1", true},
		{"killall -9 python", true},
		{"nc -l 4444", true},
		{"ncat -l 8080", true},
		{"curl | sh", true},
		{"wget | bash", true},
		{"diskutil eraseDisk JHFS+ Test", true},
		{"launchctl unload system", true},
		{"csrutil disable", true},
		{"cat /etc/passwd", true},
		{"cat /etc/shadow", true},

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

func TestHasShellInjection(t *testing.T) {
	testCases := []struct {
		command  string
		expected bool
	}{
		// Dangerous chaining
		{"echo hello; rm -rf /", true},
		{"echo hello && rm -rf /", true},
		{"echo hello || rm -rf /", true},
		{"$(rm -rf /)", true},
		{"`rm -rf /`", true},
		{"echo hello > /etc/passwd", true},
		{"echo hello > /dev/sda", true},

		// Safe commands
		{"echo hello", false},
		{"ls -la", false},
		{"go test ./...", false},
		{"grep pattern file.txt", false},
		{"echo hello > /dev/null", false},
		{"echo hello 2>/dev/null", false},
		{"echo hello 2> /dev/null", false},
		{"echo count: $(ls | wc -l) > /tmp/out.txt", false}, // redirect to /tmp is allowed
	}

	for _, tc := range testCases {
		result := hasShellInjection(tc.command)
		if result != tc.expected {
			t.Errorf("hasShellInjection(%q) = %v, expected %v", tc.command, result, tc.expected)
		}
	}
}

func TestExecuteShellInjection(t *testing.T) {
	dangerous := []string{
		"echo hello; rm -rf /",
		"echo hello && sudo su",
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