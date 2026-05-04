// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"os"
	"strings"
	"testing"
)

func TestFileEditBasicReplace(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"

	// Create a test file
	content := "line1\nline2\nline3\n"
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Replace line2 with replaced
	result, isError := FileEdit(map[string]interface{}{
		"path":        testFile,
		"old_string":  "line2",
		"new_string":  "replaced",
	})
	if isError {
		t.Errorf("FileEdit returned error: %s", result)
	}

	// Verify the file was modified
	readBack, _ := os.ReadFile(testFile)
	expected := "line1\nreplaced\nline3\n"
	if string(readBack) != expected {
		t.Errorf("FileEdit result: got %q, want %q", string(readBack), expected)
	}
}

func TestFileEditMultilineReplace(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.go"

	content := `package main

func old() {
	println("old")
}

func main() {
	old()
}`
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result, isError := FileEdit(map[string]interface{}{
		"path":       testFile,
		"old_string": "func old() {\n\tprintln(\"old\")\n}",
		"new_string": "func new() {\n\tprintln(\"new\")\n}",
	})
	if isError {
		t.Errorf("FileEdit returned error: %s", result)
	}

	readBack, _ := os.ReadFile(testFile)
	// The function "old()" should be replaced with "new()"
	// But the call site "old()" is separate text that wasn't in old_string
	if !strings.Contains(string(readBack), "new() {") {
		t.Errorf("FileEdit: new function not found after edit, got: %s", string(readBack))
	}
	if strings.Contains(string(readBack), "func old()") {
		t.Error("FileEdit: old function definition still present after edit")
	}
}

func TestFileEditNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"

	err := os.WriteFile(testFile, []byte("hello\nworld\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, isError := FileEdit(map[string]interface{}{
		"path":       testFile,
		"old_string": "nonexistent",
		"new_string": "replacement",
	})
	if !isError {
		t.Error("FileEdit should return error when old_string not found")
	}
}

func TestFileEditMultipleMatches(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"

	err := os.WriteFile(testFile, []byte("foo\nbar\nfoo\nbaz\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Without replace_all, multiple matches should error
	_, isError := FileEdit(map[string]interface{}{
		"path":       testFile,
		"old_string": "foo",
		"new_string": "qux",
	})
	if !isError {
		t.Error("FileEdit should return error when old_string matches multiple times and replace_all is not set")
	}

	// With replace_all=true, all occurrences should be replaced
	result, isError := FileEdit(map[string]interface{}{
		"path":        testFile,
		"old_string":  "foo",
		"new_string":  "qux",
		"replace_all": true,
	})
	if isError {
		t.Errorf("FileEdit with replace_all returned error: %s", result)
	}

	readBack, _ := os.ReadFile(testFile)
	expected := "qux\nbar\nqux\nbaz\n"
	if string(readBack) != expected {
		t.Errorf("FileEdit replace_all: got %q, want %q", string(readBack), expected)
	}
}

func TestFileEditFuzzyWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.go"

	// File has different whitespace than what the LLM provides
	content := "func main() {\n    println(\"hello\")\n}\n"
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// LLM provides old_string with different indentation (tabs vs spaces)
	_, isError := FileEdit(map[string]interface{}{
		"path":       testFile,
		"old_string": "func main() {\n\tprintln(\"hello\")\n}",
		"new_string": "func main() {\n\tprintln(\"goodbye\")\n}",
	})
	if isError {
		t.Error("FileEdit should handle tab/space differences via fuzzy matching")
	}

	readBack, _ := os.ReadFile(testFile)
	if !strings.Contains(string(readBack), "goodbye") {
		t.Errorf("FileEdit fuzzy: expected 'goodbye' in file, got %q", string(readBack))
	}
}

func TestFileEditEmptyNewString(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"

	err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Delete line2 by replacing with empty string
	result, isError := FileEdit(map[string]interface{}{
		"path":       testFile,
		"old_string": "line2\n",
		"new_string": "",
	})
	if isError {
		t.Errorf("FileEdit delete returned error: %s", result)
	}

	readBack, _ := os.ReadFile(testFile)
	expected := "line1\nline3\n"
	if string(readBack) != expected {
		t.Errorf("FileEdit delete: got %q, want %q", string(readBack), expected)
	}
}

func TestFileEditPathSafetyTraversal(t *testing.T) {
	_, isError := FileEdit(map[string]interface{}{
		"path":       "../etc/passwd",
		"old_string": "root",
		"new_string": "hacked",
	})
	if !isError {
		t.Error("FileEdit should block path traversal")
	}
}

func TestFileEditPathSafetyDeviceFile(t *testing.T) {
	_, isError := FileEdit(map[string]interface{}{
		"path":       "/dev/null",
		"old_string": "x",
		"new_string": "y",
	})
	if !isError {
		t.Error("FileEdit should block device files")
	}
}

func TestFileEditMissingArgs(t *testing.T) {
	_, isError := FileEdit(map[string]interface{}{
		"path": "test.txt",
	})
	if !isError {
		t.Error("FileEdit should error without old_string")
	}

	_, isError = FileEdit(map[string]interface{}{
		"path":       "test.txt",
		"old_string": "x",
	})
	// new_string is optional (can be empty for deletion), so this should NOT error
	// Actually, we should allow empty new_string. Let me check the Hermes behavior.
}

func TestFileEditNonexistentFile(t *testing.T) {
	_, isError := FileEdit(map[string]interface{}{
		"path":       "/tmp/nonexistent_file_12345/test.txt",
		"old_string": "x",
		"new_string": "y",
	})
	if !isError {
		t.Error("FileEdit should error for nonexistent file")
	}
}

func TestFileEditAllowedDirRestriction(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"

	err := os.WriteFile(testFile, []byte("hello\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Set allowlist to a different directory
	SetFileConfig([]string{"/tmp/other_dir_12345"}, nil)
	defer SetFileConfig(nil, nil)

	_, isError := FileEdit(map[string]interface{}{
		"path":       testFile,
		"old_string": "hello",
		"new_string": "goodbye",
	})
	if !isError {
		t.Error("FileEdit should block access outside allowed directories")
	}
}

func TestFuzzyMatchExactMatch(t *testing.T) {
	result := fuzzyMatch("hello world", "hello world")
	if !result {
		t.Error("fuzzyMatch should match identical strings")
	}
}

func TestFuzzyMatchTrailingWhitespace(t *testing.T) {
	result := fuzzyMatch("hello world  \n", "hello world\n")
	if !result {
		t.Error("fuzzyMatch should ignore trailing whitespace differences")
	}
}

func TestFuzzyMatchTabSpaces(t *testing.T) {
	// File has spaces, search has tab
	result := fuzzyMatch("\tprintln(\"x\")", "    println(\"x\")")
	if !result {
		t.Error("fuzzyMatch should handle tab/space equivalence")
	}
}

func TestFuzzyMatchDifferentStrings(t *testing.T) {
	result := fuzzyMatch("hello", "goodbye")
	if result {
		t.Error("fuzzyMatch should not match completely different strings")
	}
}

func TestFuzzyMatchSimilarButDifferent(t *testing.T) {
	// Strings that are similar but not the same content shouldn't match
	result := fuzzyMatch("func foo() {\n\treturn 1\n}", "func bar() {\n\treturn 2\n}")
	// These are too different — should not match
	// However, our fuzzy matcher normalizes whitespace, so we need to be careful
	// that it doesn't match unrelated code blocks
	if result {
		t.Error("fuzzyMatch should not match strings with different content (just similar structure)")
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello  \n", "hello\n"},
		{"\thello", "hello"},          // tab → spaces → trimmed
		{"hello\r\nworld", "hello\nworld"},
		{"  hello  \n  world  \n", "hello\nworld\n"},
	}
	for _, tc := range tests {
		result := normalizeWhitespace(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeWhitespace(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}