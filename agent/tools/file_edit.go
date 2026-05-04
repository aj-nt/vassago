// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"fmt"
	"os"
	"strings"
)

// FileEdit performs a targeted find-and-replace in a file.
// It finds old_string and replaces it with new_string.
// If old_string appears multiple times, it errors unless replace_all is true.
// Supports fuzzy matching: minor whitespace differences (tabs vs spaces,
// trailing whitespace) are tolerated so the LLM doesn't need exact copies.
func FileEdit(args map[string]interface{}) (string, bool) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "FileEdit requires a 'path' argument", true
	}

	oldString, ok := args["old_string"].(string)
	if !ok {
		return "FileEdit requires an 'old_string' argument", true
	}

	// new_string defaults to empty (deletion)
	newString := ""
	if ns, ok := args["new_string"].(string); ok {
		newString = ns
	}

	replaceAll := false
	if ra, ok := args["replace_all"].(bool); ok {
		replaceAll = ra
	}

	// Path safety checks
	if strings.Contains(path, "../") {
		return fmt.Sprintf("Access denied: path traversal not allowed in path %q", path), true
	}
	path = expandPath(path)

	if !isWriteAllowed(path) {
		return fmt.Sprintf("Write denied: path %q is not in a writable directory", path), true
	}

	// Read the file
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err), true
	}

	// Reject oversized old_string (prevents abuse)
	const maxOldStringLen = 50000
	if len(oldString) > maxOldStringLen {
		return fmt.Sprintf("old_string too large (%d chars, max %d)", len(oldString), maxOldStringLen), true
	}

	// Try exact match first
	count := strings.Count(string(content), oldString)

	if count == 0 {
		// Try fuzzy match — normalize whitespace and retry
		normalizedContent := normalizeWhitespace(string(content))
		normalizedOld := normalizeWhitespace(oldString)
		fuzzyCount := strings.Count(normalizedContent, normalizedOld)

		if fuzzyCount == 0 {
			return fmt.Sprintf("old_string not found in file. No exact or fuzzy match for %d-char snippet.", len(oldString)), true
		}

		if fuzzyCount > 1 && !replaceAll {
			return fmt.Sprintf("old_string matches %d locations (fuzzy). Set replace_all=true to replace all, or provide more context to make the match unique.", fuzzyCount), true
		}

		// Find the matches in normalized space and map back to original positions
		var result strings.Builder
		lastEnd := 0
		normalizedIdx := 0
		contentRunes := []rune(string(content))
		normalizedRunes := []rune(normalizedContent)
		oldRunes := []rune(normalizedOld)

		matchesFound := 0
		for normalizedIdx <= len(normalizedRunes)-len(oldRunes) {
			match := true
			for i := 0; i < len(oldRunes); i++ {
				if normalizedRunes[normalizedIdx+i] != oldRunes[i] {
					match = false
					break
				}
			}

			if match {
				// Found a fuzzy match at normalizedIdx
				// We need to find the corresponding position in the original content
				// by counting non-whitespace chars up to this point
				origStart := findOriginalPosition(contentRunes, normalizedRunes, normalizedIdx)
				origEnd := findOriginalPosition(contentRunes, normalizedRunes, normalizedIdx+len(oldRunes))

				// Append the original content before this match, then new_string
				result.WriteString(string(contentRunes[lastEnd:origStart]))
				result.WriteString(newString)
				lastEnd = origEnd

				matchesFound++
				normalizedIdx += len(oldRunes)

				if !replaceAll {
					break
				}
			} else {
				normalizedIdx++
			}
		}

		// Append remaining content
		result.WriteString(string(contentRunes[lastEnd:]))

		if err := os.WriteFile(path, []byte(result.String()), 0644); err != nil {
			return fmt.Sprintf("Error writing file: %v", err), true
		}

		return fmt.Sprintf("Replaced %d occurrence(s) (fuzzy match)", matchesFound), false
	}

	if count > 1 && !replaceAll {
		return fmt.Sprintf("old_string matches %d locations. Set replace_all=true to replace all, or provide more context to make the match unique.", count), true
	}

	// Exact replacement
	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(string(content), oldString, newString)
	} else {
		newContent = strings.Replace(string(content), oldString, newString, 1)
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Sprintf("Error writing file: %v", err), true
	}

	label := "occurrence"
	if count > 1 {
		label = fmt.Sprintf("%d occurrences", count)
	}
	return fmt.Sprintf("Replaced %s in %s", label, path), false
}

// findOriginalPosition maps a position in normalized (whitespace-collapsed)
// content back to the original content's position.
// Strategy: walk both strings in parallel, advancing through the original
// for each character that survives normalization.
func findOriginalPosition(original, normalized []rune, normPos int) int {
	origIdx := 0
	normIdx := 0

	for normIdx < normPos && origIdx < len(original) {
		origChar := original[origIdx]
		normChar := normalized[normIdx]

		if origChar == normChar {
			// Same character after normalization — advance both
			origIdx++
			normIdx++
		} else {
			// Different — this is whitespace that was normalized away
			// Skip it in the original
			origIdx++
		}
	}

	return origIdx
}

// normalizeWhitespace collapses whitespace for fuzzy matching:
// - Tabs → 4 spaces
// - \r\n → \n
// - Leading/trailing whitespace per line stripped
// - Multiple spaces collapsed to single
func normalizeWhitespace(s string) string {
	// CRLF → LF
	s = strings.ReplaceAll(s, "\r\n", "\n")

	// Tabs → 4 spaces
	s = strings.ReplaceAll(s, "\t", "    ")

	// Split into lines, strip per-line whitespace
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		// Trim leading and trailing whitespace per line
		result = append(result, strings.TrimSpace(line))
	}

	// Rejoin with newlines
	// If original ended with newline, preserve that
	joined := strings.Join(result, "\n")
	if strings.HasSuffix(s, "\n") && !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}

	return joined
}

// fuzzyMatch checks if two strings match after whitespace normalization.
func fuzzyMatch(content, search string) bool {
	return normalizeWhitespace(content) == normalizeWhitespace(search)
}

// FileEditSchema is the LLM tool definition for FileEdit.
var FileEditSchema = map[string]interface{}{
	"name":        "FileEdit",
	"description": "Targeted find-and-replace in a file. Finds old_string and replaces with new_string. Supports fuzzy matching for whitespace differences. Use replace_all=true to replace all occurrences.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to edit",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "The text to find in the file (must be unique unless replace_all is true)",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "The replacement text (empty string to delete)",
				"default":     "",
			},
			"replace_all": map[string]interface{}{
				"type":        "boolean",
				"description": "Replace all occurrences instead of requiring a unique match",
				"default":     false,
			},
		},
		"required": []string{"path", "old_string"},
	},
}