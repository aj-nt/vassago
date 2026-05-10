// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// keyArgFields maps tool names to the JSON field that distinguishes different
// calls to that tool. For example, FileRead's "path" field differentiates
// reading different files, while Execute's "command" field differentiates
// running different commands.
var keyArgFields = map[string]string{
	"FileRead":       "path",
	"FileWrite":      "path",
	"FileEdit":       "path",
	"Execute":        "command",
	"SearchMemories": "query",
	"SearchWeb":      "query",
	"FetchURL":       "url",
	"SearchSessions": "query",
	"FileSearch":     "pattern",
}

// extractKeyArg extracts a named field from a JSON RawMessage of tool arguments.
// Returns the string value truncated to 64 chars, or empty string on failure.
func extractKeyArg(args json.RawMessage, field string) string {
	var m map[string]interface{}
	if err := json.Unmarshal(args, &m); err != nil {
		return ""
	}
	if v, ok := m[field]; ok {
		return truncateStr(fmt.Sprintf("%v", v), 64)
	}
	return ""
}

// ComputeProgressSignature creates a progress signature for the current iteration
// based on the tool calls made, their results, and the LLM's stated intent.
// For tool calls with key argument fields defined in keyArgFields, the value
// of that field is appended to the tool name to distinguish different operations.
// For example, FileRead with path="/a/foo.go" becomes "FileRead:/a/foo.go",
// and Execute with command="git log" becomes "Execute:git log".
func ComputeProgressSignature(toolCalls []ToolCall, results []ToolResult, intentText string) ProgressSignature {
	// Build tool names, including key argument for differentiation
	names := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		name := tc.Name
		if keyField, ok := keyArgFields[tc.Name]; ok {
			argVal := extractKeyArg(tc.Arguments, keyField)
			if argVal != "" {
				name = tc.Name + ":" + argVal
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	toolNames := strings.Join(names, ",")

	// Determine result tag
	resultTag := "success" // default
	if len(results) == 0 {
		resultTag = "empty"
	} else {
		hasError := false
		hasNotFound := false
		hasSuccess := false
		for _, r := range results {
			if r.IsError {
				hasError = true
			} else if IsResultNotFound(r.Result) {
				hasNotFound = true
			} else {
				hasSuccess = true
			}
		}
		if hasError && !hasSuccess && !hasNotFound {
			resultTag = "error"
		} else if hasNotFound && !hasSuccess && !hasError {
			resultTag = "not_found"
		} else if hasError || hasNotFound {
			resultTag = "mixed"
		}
	}

	return ProgressSignature{
		ToolNames: toolNames,
		ResultTag: resultTag,
		IntentTag: ComputeIntentTag(toolNames, intentText),
	}
}

// truncateStr truncates s to maxLen runes, appending "…" if truncated.
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// IsDuplicateProgress checks if a signature matches any of the last 3
// history entries by ToolNames and ResultTag. Two iterations that call
// the same tools with the same key arguments and get the same result
// classification are considered duplicates.
func IsDuplicateProgress(sig ProgressSignature, history []ProgressSignature) bool {
	start := len(history) - 3
	if start < 0 {
		start = 0
	}
	for i := start; i < len(history); i++ {
		if sig.ToolNames == history[i].ToolNames && sig.ResultTag == history[i].ResultTag {
			return true
		}
	}
	return false
}