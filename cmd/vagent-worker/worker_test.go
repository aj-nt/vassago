package main

import (
	"strings"
	"testing"
)

func TestTruncate_ShorterThanMax(t *testing.T) {
	result := truncate("hello", 10)
	if result != "hello" {
		t.Errorf("expected 'hello', got '%s'", result)
	}
}

func TestTruncate_ExactlyMax(t *testing.T) {
	result := truncate("hello", 5)
	if result != "hello" {
		t.Errorf("expected 'hello', got '%s'", result)
	}
}

func TestTruncate_LongerThanMax(t *testing.T) {
	result := truncate("hello world", 5)
	if result != "hello..." {
		t.Errorf("expected 'hello...', got '%s'", result)
	}
}

func TestTruncate_Empty(t *testing.T) {
	result := truncate("", 5)
	if result != "" {
		t.Errorf("expected '', got '%s'", result)
	}
}

func TestBuildExecutionPrompt_ContainsGoal(t *testing.T) {
	prompt := buildExecutionPrompt("analyze code", `{"repo_path":"/tmp/test"}`)

	if !strings.Contains(prompt, "analyze code") {
		t.Errorf("prompt missing goal: %s", prompt)
	}
	if !strings.Contains(prompt, "GOAL:") {
		t.Errorf("prompt missing GOAL header")
	}
	if !strings.Contains(prompt, "CONTEXT:") {
		t.Errorf("prompt missing CONTEXT header")
	}
	if !strings.Contains(prompt, "PROTOCOL") {
		t.Errorf("prompt missing PROTOCOL section")
	}
	if !strings.Contains(prompt, `{"repo_path":"/tmp/test"}`) {
		t.Errorf("prompt missing context content")
	}
}

func TestBuildExecutionPrompt_ProtocolCommands(t *testing.T) {
	prompt := buildExecutionPrompt("task", "{}")

	// All three protocol commands must be present
	for _, cmd := range []string{"CMD:", "DONE:", "FAIL:"} {
		if !strings.Contains(prompt, cmd) {
			t.Errorf("prompt missing protocol command %s", cmd)
		}
	}
}

func TestBuildExecutionPrompt_Rules(t *testing.T) {
	prompt := buildExecutionPrompt("task", "{}")

	// Safety rules must be present
	if !strings.Contains(prompt, "not destructive") && !strings.Contains(prompt, "Never run destructive") {
		t.Errorf("prompt missing destructive command warning")
	}
	if !strings.Contains(prompt, "60 seconds") && !strings.Contains(prompt, "60 second") {
		t.Errorf("prompt missing timeout warning")
	}
}
