// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import "fmt"

// ToolResult is a structured return type for tool handlers.
// It complements the existing (string, bool) pattern with machine-readable
// error codes that callers can switch on instead of string-matching.
type ToolResult struct {
	Output string
	Error  *ToolError // nil means success
}

// IsError returns true if this result represents an error.
func (r ToolResult) IsError() bool { return r.Error != nil }

// ToolError represents a structured tool error with a machine-readable code.
type ToolError struct {
	Code    string // e.g., "VALIDATION", "NOT_FOUND", "TIMEOUT"
	Message string // Human-readable message
	Cause   error  // Underlying error, may be nil
}

// Error satisfies the error interface.
func (e *ToolError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Predefined error codes for consistent error classification.
const (
	ErrCodeValidation        = "VALIDATION"
	ErrCodeNotFound          = "NOT_FOUND"
	ErrCodeTimeout           = "TIMEOUT"
	ErrCodeDaemonUnavailable = "DAEMON_UNAVAILABLE"
	ErrCodeRateLimited       = "RATE_LIMITED"
	ErrCodeInternal          = "INTERNAL"
)

// --- Conversion helpers ---

// ToResult converts a (string, bool) handler return to a ToolResult.
func ToResult(output string, isErr bool) ToolResult {
	if isErr {
		return ToolResult{
			Output: output,
			Error:  &ToolError{Code: ErrCodeInternal, Message: output},
		}
	}
	return ToolResult{Output: output}
}

// Success makes a successful result.
func Success(output string) (string, bool) {
	return output, false
}

// Failure makes an error result with the given message.
func Failure(message string) (string, bool) {
	return message, true
}
