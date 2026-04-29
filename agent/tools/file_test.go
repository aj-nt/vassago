package tools

import (
	"os"
	"strings"
	"testing"
)

func TestFileRead(t *testing.T) {
	// Create a test file
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

func TestFileWrite(t *testing.T) {
	testFile := "test_write.txt"
	defer os.Remove(testFile)

	_, isError := FileWrite(map[string]interface{}{"path": testFile, "content": "test"})
	if !isError {
		t.Error("FileWrite should require approval by default")
		os.Remove(testFile) // Clean up if file was created
	}
}

func TestFileSearch(t *testing.T) {
	// Create test files
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

	// Should find test1.txt
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