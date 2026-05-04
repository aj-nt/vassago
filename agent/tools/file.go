// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// allowedDirs is the list of directories the agent is allowed to read/write.
// Set by RegisterFileToolsWithConfig. If empty, all absolute paths are allowed
// (dev mode). If non-empty, only paths under these directories are allowed.
var allowedDirs []string

// approvalRequired is the list of tools that require user approval before execution.
// Used by the CLI to prompt the user before executing tools like FileWrite and Execute.
var approvalRequired []string

// IsApprovalRequired returns whether a tool requires human approval before execution.
func IsApprovalRequired(toolName string) bool {
	for _, name := range approvalRequired {
		if name == toolName {
			return true
		}
	}
	return false
}

// SetFileConfig configures file tools with security constraints.
func SetFileConfig(dirs []string, approval []string) {
	allowedDirs = make([]string, 0, len(dirs))
	for _, d := range dirs {
		// Expand ~ to home directory
		if strings.HasPrefix(d, "~/") || d == "~" {
			if home, err := os.UserHomeDir(); err == nil {
				d = filepath.Join(home, d[1:])
			}
		}
		abs, err := filepath.Abs(d)
		if err == nil {
			// Resolve symlinks in allowlist entries too (e.g., /tmp → /private/tmp on macOS)
			resolved, resolveErr := filepath.EvalSymlinks(abs)
			if resolveErr == nil {
				allowedDirs = append(allowedDirs, resolved)
			} else {
				// Directory doesn't exist yet — use the absolute path as-is
				allowedDirs = append(allowedDirs, abs)
			}
		}
	}
	approvalRequired = approval
}

// isPathSafe checks if a path is safe to access.
// It resolves symlinks, checks for traversal, and validates against allowed directories.
func isPathSafe(path string) bool {
	// Reject obvious traversal
	if strings.Contains(path, "../") {
		return false
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Resolve symlinks — walk up the path until we find an existing component
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// File or parent directories don't exist yet — walk up to find
		// the nearest existing ancestor, then append the remaining path
		dir := filepath.Dir(absPath)
		base := filepath.Base(absPath)
		for {
			dirResolved, dirErr := filepath.EvalSymlinks(dir)
			if dirErr == nil {
				resolved = filepath.Join(dirResolved, base)
				break
			}
			// Parent doesn't exist either — go up one level
			base = filepath.Join(filepath.Base(dir), base)
			dir = filepath.Dir(dir)
			if dir == "/" {
				// Reached root without finding an existing directory
				resolved = absPath
				break
			}
		}
	}

	// Block device files and virtual filesystems
	if isDevicePath(resolved) {
		return false
	}

	// If allowedDirs is configured, enforce it
	if len(allowedDirs) > 0 {
		return IsUnderAllowedDir(resolved)
	}

	// No allowlist configured — allow all (dev mode)
	return true
}

// isWriteAllowed checks if file write operations are allowed for the given path.
func isWriteAllowed(path string) bool {
	// isPathSafe already checks allowlist, symlink resolution, device paths, etc.
	// For write, we use the same logic — if it's safe to read, it's safe to write
	// (within the allowlist boundaries).
	return isPathSafe(path)
}

// isDevicePath returns true for paths under /dev, /proc, /sys, /sys/fs.
func isDevicePath(path string) bool {
	devPrefixes := []string{"/dev/", "/proc/", "/sys/"}
	for _, prefix := range devPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// IsUnderAllowedDir checks if the path falls under any of the configured allowed directories.
// Resolves symlinks on the input path to handle macOS /tmp → /private/tmp, /var → /private/var.
func IsUnderAllowedDir(path string) bool {
	// Try to resolve symlinks on the input path
	resolvedPath := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		resolvedPath = r
	} else {
		// Walk up to find existing ancestor, same as isPathSafe
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		for {
			if dirResolved, dirErr := filepath.EvalSymlinks(dir); dirErr == nil {
				resolvedPath = filepath.Join(dirResolved, base)
				break
			}
			base = filepath.Join(filepath.Base(dir), base)
			dir = filepath.Dir(dir)
			if dir == "/" {
				break
			}
		}
	}

	for _, dir := range allowedDirs {
		// Check both resolved and raw paths against resolved allowlist
		for _, p := range []string{resolvedPath, path} {
			if strings.HasPrefix(p+"/", dir+"/") || p == dir {
				return true
			}
		}
	}
	return false
}

// expandPath expands ~ to the home directory and makes the path absolute.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// FileRead reads a file and returns its content.
func FileRead(args map[string]interface{}) (string, bool) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "FileRead requires a 'path' argument", true
	}

	// Reject path traversal before expanding
	if strings.Contains(path, "../") {
		return fmt.Sprintf("Access denied: path traversal not allowed in path %q", path), true
	}

	// Expand ~ and make absolute before safety checks and file access
	path = expandPath(path)

	if !isPathSafe(path) {
		return fmt.Sprintf("Access denied: path %q is not in an allowed directory", path), true
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err), true
	}

	// Truncate output for large files
	const maxResultChars = 20000
	return truncateOutput(string(content), maxResultChars), false
}

// FileWrite writes content to a file.
func FileWrite(args map[string]interface{}) (string, bool) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "FileWrite requires a 'path' argument", true
	}

	// Reject path traversal before expanding
	if strings.Contains(path, "../") {
		return fmt.Sprintf("Access denied: path traversal not allowed in path %q", path), true
	}

	// Expand ~ and make absolute before safety checks and file access
	path = expandPath(path)

	content, ok := args["content"].(string)
	if !ok {
		return "FileWrite requires a 'content' argument", true
	}

	if !isWriteAllowed(path) {
		return fmt.Sprintf("Write denied: path %q is not in a writable directory", path), true
	}

	err := os.WriteFile(path, []byte(content), fs.FileMode(0644))
	if err != nil {
		return fmt.Sprintf("Error writing file: %v", err), true
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), false
}

// FileSearch searches for files matching a pattern.
func FileSearch(args map[string]interface{}) (string, bool) {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return "FileSearch requires a 'pattern' argument", true
	}

	directory := "."
	if dir, ok := args["directory"].(string); ok && dir != "" {
		// Check for path traversal before expanding
		if strings.Contains(dir, "../") {
			return fmt.Sprintf("Access denied: path traversal not allowed in directory %q", dir), true
		}
		dir = expandPath(dir)
		if !isPathSafe(dir) {
			return fmt.Sprintf("Access denied: directory %q is not in an allowed directory", dir), true
		}
		directory = dir
	}

	matches, err := filepath.Glob(filepath.Join(directory, pattern))
	if err != nil {
		return fmt.Sprintf("Error searching files: %v", err), true
	}

	if len(matches) == 0 {
		return "No files found matching pattern", false
	}

	// Truncate output for file listings
	const maxResultChars = 4000
	return truncateOutput(strings.Join(matches, "\n"), maxResultChars), false
}

// RegisterFileTools adds all file tool handlers to the registry.
func RegisterFileTools(registry *ToolRegistry) {
	RegisterFileToolsWithConfig(registry, nil, nil)
}

// RegisterFileToolsWithConfig adds file tools with security configuration.
func RegisterFileToolsWithConfig(registry *ToolRegistry, dirs []string, approval []string) {
	SetFileConfig(dirs, approval)
	registry.Register("FileRead", FileRead, FileReadSchema)
	registry.Register("FileWrite", FileWrite, FileWriteSchema)
	registry.Register("FileSearch", FileSearch, FileSearchSchema)
	registry.Register("FileEdit", FileEdit, FileEditSchema)
}

// Tool schemas

var FileReadSchema = map[string]interface{}{
	"name":        "FileRead",
	"description": "Read the contents of a file. Only files in allowed directories can be read.",
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
	"description": "Write content to a file. Only files in writable directories can be written. Requires approval in production.",
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
	"description": "Search for files matching a glob pattern in a directory.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern to match (e.g., '*.txt', 'test*.go')",
			},
			"directory": map[string]interface{}{
				"type":        "string",
				"description": "Directory to search in (default: current directory)",
			},
		},
		"required": []string{"pattern"},
	},
}