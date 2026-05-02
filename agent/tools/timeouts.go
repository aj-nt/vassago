package tools

import (
	"context"
	"time"
)

// Timeout constants for tool operations (Fowler: Replace Magic Number with Named Constant)
const (
	// DefaultTimeout is the standard timeout for daemon calls (memory, session, todo).
	DefaultTimeout = 10 * time.Second

	// LongTimeout is for operations that may take longer (consolidation, web fetch).
	LongTimeout = 30 * time.Second
)

// withTimeout returns a context and cancel function with the given timeout.
// This is an Extract Function refactoring to remove the repeated
// context.WithTimeout(context.Background(), ...) + defer cancel() pattern.
func withTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}