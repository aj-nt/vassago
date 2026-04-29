package tools

import (
	"testing"
)

func TestExecute(t *testing.T) {
	result, isError := Execute(map[string]interface{}{"command": "echo 'Hello, world!'"})
	if isError {
		t.Errorf("Execute returned error: %s", result)
		return
	}
	// echo output includes trailing newline
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

func TestIsDangerousCommand(t *testing.T) {
	testCases := []struct {
		command  string
		expected bool
	}{
		{"rm -rf /", true},
		{"sudo apt update", true},
		{"mkfs.ext4 /dev/sda1", true},
		{"dd if=/dev/zero of=/dev/sda", true},
		{":(){:|:&};:", true},
		{"echo 'hello'", false},
		{"ls -la", false},
		{"cat file.txt", false},
		{"grep pattern file", false},
	}

	for _, tc := range testCases {
		result := isDangerousCommand(tc.command)
		if result != tc.expected {
			t.Errorf("isDangerousCommand(%q) = %v, expected %v", tc.command, result, tc.expected)
		}
	}
}