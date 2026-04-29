package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileRead reads a file and returns its content.
func FileRead(args map[string]interface{}) (string, bool) {
	path := strArg(args, "path", "")
	if path == "" {
		return "path is required", true
	}

	// Path safety check
	if !isPathSafe(path) {
		return "invalid path: traversal or absolute paths not allowed", true
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("failed to read file: %v", err), true
	}

	return string(content), false
}

// FileWrite writes content to a file.
func FileWrite(args map[string]interface{}) (string, bool) {
	path := strArg(args, "path", "")
	content := strArg(args, "content", "")
	if path == "" || content == "" {
		return "path and content are required", true
	}

	// Path safety check
	if !isPathSafe(path) {
		return "invalid path: traversal or absolute paths not allowed", true
	}

	// Check if write is allowed (requires approval unless config says enabled)
	if !isWriteAllowed() {
		return "file write operations require approval", true
	}

	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return fmt.Sprintf("failed to write file: %v", err), true
	}

	return "file written successfully", false
}

// FileSearch searches for files by name pattern.
func FileSearch(args map[string]interface{}) (string, bool) {
	pattern := strArg(args, "pattern", "")
	directory := strArg(args, "directory", ".")
	if pattern == "" {
		return "pattern is required", true
	}

	// Path safety check
	if !isPathSafe(directory) {
		return "invalid directory: traversal or absolute paths not allowed", true
	}

	matches, err := filepath.Glob(filepath.Join(directory, pattern))
	if err != nil {
		return fmt.Sprintf("failed to search files: %v", err), true
	}

	if len(matches) == 0 {
		return "no files found", false
	}

	result := fmt.Sprintf("Found %d files:\n", len(matches))
	for _, match := range matches {
		result += fmt.Sprintf("- %s\n", match)
	}

	return result, false
}

// isPathSafe checks if a path is safe (no traversal, no absolute paths outside allowed dirs).
func isPathSafe(path string) bool {
	// Reject paths with "../" traversal
	if strings.Contains(path, "../") {
		return false
	}

	// Reject absolute paths (for now, we'll allow them if they're within allowed dirs later)
	if filepath.IsAbs(path) {
		return false
	}

	return true
}

// isWriteAllowed checks if file write operations are allowed.
func isWriteAllowed() bool {
	// TODO: Implement config check for allowed directories
	// For now, return false to require approval
	return false
}

// RegisterFileTools adds all file tool handlers to the registry.
func RegisterFileTools(registry *ToolRegistry) {
	registry.Register("FileRead", FileRead, FileReadSchema)
	registry.Register("FileWrite", FileWrite, FileWriteSchema)
	registry.Register("FileSearch", FileSearch, FileSearchSchema)
}

// Tool schemas for file operations
var FileReadSchema = map[string]interface{}{
	"name":        "FileRead",
	"description": "Read the content of a file.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to read",
			},
		},
		"required": []string{"path"},
	},
}

var FileWriteSchema = map[string]interface{}{
	"name":        "FileWrite",
	"description": "Write content to a file. Requires approval.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	},
}

var FileSearchSchema = map[string]interface{}{
	"name":        "FileSearch",
	"description": "Search for files by name pattern.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "File name pattern to search for",
			},
			"directory": map[string]interface{}{
				"type":        "string",
				"description": "Directory to search in (defaults to current directory)",
			},
		},
		"required": []string{"pattern"},
	},
}
