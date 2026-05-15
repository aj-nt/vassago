// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// --- groupsToToolNames tests ---

func TestGroupsToToolNames_AllGroups(t *testing.T) {
	allGroups := []string{
		"memory", "files", "terminal", "web", "sessions", "todos",
		"cron", "messaging", "delegation", "browse", "vision",
		"image_gen", "multi_agent",
	}
	names := groupsToToolNames(allGroups)

	expectedTools := []string{
		"AddMemory", "SearchMemories", "ReplaceMemory", "RemoveMemory",
		"Consolidate", "GetHotBlock",
		"FileRead", "FileSearch", "FileWrite",
		"Execute",
		"SearchWeb", "FetchURL",
		"SearchSessions", "ListSessions", "GetSession",
		"AddTodo", "ListTodos", "CompleteTodo", "RemoveTodo",
		"ScheduleCron",
		"SendMessage",
		"Delegate",
		"Browse",
		"AnalyzeImage",
		"GenerateImage",
		"ListAgents", "RegisterAgent", "FindAgent",
	}

	sort.Strings(names)
	sort.Strings(expectedTools)
	if len(names) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), len(names))
	}
	for i, name := range names {
		if name != expectedTools[i] {
			t.Errorf("tool[%d] = %q, want %q", i, name, expectedTools[i])
		}
	}
}

func TestGroupsToToolNames_Empty(t *testing.T) {
	names := groupsToToolNames(nil)
	if len(names) != 0 {
		t.Errorf("expected 0 tools for empty groups, got %d", len(names))
	}
}

func TestGroupsToToolNames_MemoryOnly(t *testing.T) {
	names := groupsToToolNames([]string{"memory"})
	expected := []string{"AddMemory", "SearchMemories", "ReplaceMemory", "RemoveMemory", "Consolidate", "GetHotBlock"}
	if len(names) != len(expected) {
		t.Errorf("expected %d memory tools, got %d", len(expected), len(names))
	}
	for _, e := range expected {
		found := false
		for _, n := range names {
			if n == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %q not found", e)
		}
	}
}

func TestGroupsToToolNames_UnknownGroup(t *testing.T) {
	names := groupsToToolNames([]string{"unknown_group"})
	if len(names) != 0 {
		t.Errorf("expected 0 tools for unknown group, got %d", len(names))
	}
}

// --- parseInitFlags tests ---

func TestParseInitFlags_Defaults(t *testing.T) {
	flags := parseInitFlags([]string{})
	home, _ := os.UserHomeDir()
	expectedDir := filepath.Join(home, ".vassago")
	if flags.dir != expectedDir {
		t.Errorf("default dir = %q, want %q", flags.dir, expectedDir)
	}
	if flags.force {
		t.Error("default force should be false")
	}
	if flags.nonInteractive {
		t.Error("default nonInteractive should be false")
	}
	if flags.help {
		t.Error("default help should be false")
	}
}

func TestParseInitFlags_CustomDir(t *testing.T) {
	flags := parseInitFlags([]string{"--dir", "/custom/path"})
	if flags.dir != "/custom/path" {
		t.Errorf("dir = %q, want /custom/path", flags.dir)
	}
}

func TestParseInitFlags_Force(t *testing.T) {
	flags := parseInitFlags([]string{"--force"})
	if !flags.force {
		t.Error("--force should set force=true")
	}
}

func TestParseInitFlags_ShortForce(t *testing.T) {
	flags := parseInitFlags([]string{"-f"})
	if !flags.force {
		t.Error("-f should set force=true")
	}
}

func TestParseInitFlags_NonInteractive(t *testing.T) {
	flags := parseInitFlags([]string{"--non-interactive"})
	if !flags.nonInteractive {
		t.Error("--non-interactive should set nonInteractive=true")
	}
}

func TestParseInitFlags_ShortNonInteractive(t *testing.T) {
	flags := parseInitFlags([]string{"-y"})
	if !flags.nonInteractive {
		t.Error("-y should set nonInteractive=true")
	}
}

func TestParseInitFlags_Help(t *testing.T) {
	flags := parseInitFlags([]string{"--help"})
	if !flags.help {
		t.Error("--help should set help=true")
	}
}

func TestParseInitFlags_ShortHelp(t *testing.T) {
	flags := parseInitFlags([]string{"-h"})
	if !flags.help {
		t.Error("-h should set help=true")
	}
}

func TestParseInitFlags_Combined(t *testing.T) {
	flags := parseInitFlags([]string{"--dir", "/tmp/mnemo", "--force", "-y"})
	if flags.dir != "/tmp/mnemo" {
		t.Errorf("dir = %q, want /tmp/mnemo", flags.dir)
	}
	if !flags.force {
		t.Error("force should be true")
	}
	if !flags.nonInteractive {
		t.Error("nonInteractive should be true")
	}
}

// --- maskKey tests ---

func TestMaskKey_ShortKey(t *testing.T) {
	result := maskKey("abc")
	if result != "****" {
		t.Errorf("maskKey(%q) = %q, want %q", "abc", result, "****")
	}
}

func TestMaskKey_ExactCutoff(t *testing.T) {
	// For a key of exactly 8 chars: len <= 8, so returns "****"
	result := maskKey("abcdefgh")
	if result != "****" {
		t.Errorf("maskKey of 8-char key = %q, want %q", result, "****")
	}
}

func TestMaskKey_NormalKey(t *testing.T) {
	key := "sk-1234567890abcdef"
	result := maskKey(key)
	// key[:4] = "sk-1", len-8=12 stars, key[len-4:] = "cdef"
	if len(result) < 8 {
		t.Errorf("maskKey result too short: %q", result)
	}
	// Check prefix and suffix visible
	if result[:4] != "sk-1" {
		t.Errorf("maskKey prefix = %q, want %q", result[:4], "sk-1")
	}
	if result[len(result)-4:] != "cdef" {
		t.Errorf("maskKey suffix = %q, want %q", result[len(result)-4:], "cdef")
	}
}

func TestMaskKey_EmptyKey(t *testing.T) {
	result := maskKey("")
	if result != "****" {
		t.Errorf("maskKey(%q) = %q, want %q", "", result, "****")
	}
}

// --- expandHome tests ---

func TestExpandHome_Tilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	result := expandHome("~/test")
	expected := filepath.Join(home, "test")
	if result != expected {
		t.Errorf("expandHome(%q) = %q, want %q", "~/test", result, expected)
	}
}

func TestExpandHome_AbsolutePath(t *testing.T) {
	result := expandHome("/tmp/test")
	if result != "/tmp/test" {
		t.Errorf("expandHome(%q) = %q, want %q", "/tmp/test", result, "/tmp/test")
	}
}

func TestExpandHome_EmptyString(t *testing.T) {
	result := expandHome("")
	if result != "" {
		t.Errorf("expandHome(%q) = %q, want %q", "", result, "")
	}
}

// --- getConfigPaths tests ---

func TestGetConfigPaths(t *testing.T) {
	dir := "/home/user/.vassago"
	daemonConfigPath, agentConfigPath := getConfigPaths(dir)
	if daemonConfigPath != filepath.Join(dir, "config.yaml") {
		t.Errorf("daemonConfigPath = %q, want %q", daemonConfigPath, filepath.Join(dir, "config.yaml"))
	}
	if agentConfigPath != filepath.Join(dir, "agent.yaml") {
		t.Errorf("agentConfigPath = %q, want %q", agentConfigPath, filepath.Join(dir, "agent.yaml"))
	}
}

// --- checkExistingConfigs tests ---

func TestCheckExistingConfigs_AllExist(t *testing.T) {
	tmpDir := t.TempDir()
	daemonPath := filepath.Join(tmpDir, "config.yaml")
	agentPath := filepath.Join(tmpDir, "agent.yaml")

	// Create files
	os.WriteFile(daemonPath, []byte("test"), 0644)
	os.WriteFile(agentPath, []byte("test"), 0644)

	daemonExists, agentExists := checkExistingConfigs(daemonPath, agentPath)
	if !daemonExists {
		t.Error("daemon config should exist")
	}
	if !agentExists {
		t.Error("agent config should exist")
	}
}

func TestCheckExistingConfigs_NoneExist(t *testing.T) {
	tmpDir := t.TempDir()
	daemonPath := filepath.Join(tmpDir, "config.yaml")
	agentPath := filepath.Join(tmpDir, "agent.yaml")

	daemonExists, agentExists := checkExistingConfigs(daemonPath, agentPath)
	if daemonExists {
		t.Error("daemon config should not exist")
	}
	if agentExists {
		t.Error("agent config should not exist")
	}
}

func TestCheckExistingConfigs_Partial(t *testing.T) {
	tmpDir := t.TempDir()
	daemonPath := filepath.Join(tmpDir, "config.yaml")
	agentPath := filepath.Join(tmpDir, "agent.yaml")

	// Create only daemon config
	os.WriteFile(daemonPath, []byte("test"), 0644)

	daemonExists, agentExists := checkExistingConfigs(daemonPath, agentPath)
	if !daemonExists {
		t.Error("daemon config should exist")
	}
	if agentExists {
		t.Error("agent config should not exist")
	}
}
