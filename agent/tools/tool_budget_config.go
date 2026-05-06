// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

// ToolBudgetConfig holds per-tool call limits for a single agent turn.
// Using a named struct instead of a package-level map makes the limits
// discoverable, testable, and overridable per-agent.
//
// Use DefaultToolBudget() for production defaults.
type ToolBudgetConfig struct {
	// FileRead is the maximum number of FileRead calls per turn (default: 5).
	FileRead int

	// FileSearch is the maximum number of FileSearch calls per turn (default: 3).
	FileSearch int

	// Execute is the maximum number of Execute calls per turn (default: 10).
	Execute int
}

// DefaultToolBudget returns the production defaults for tool budgets.
func DefaultToolBudget() ToolBudgetConfig {
	return ToolBudgetConfig{
		FileRead:    5,
		FileSearch: 3,
		Execute:     10,
	}
}

// CheckBudget returns true if the tool has NOT exceeded its per-turn budget.
// Tools not in the config have unlimited calls.
func (b ToolBudgetConfig) CheckBudget(toolName string, callCount int) bool {
	limit := b.LimitFor(toolName)
	if limit == 0 {
		return true // unlimited
	}
	return callCount < limit
}

// BudgetExceededMessage returns a helpful error message for budget-exceeded tools.
// Returns empty string if the tool has unlimited budget.
func (b ToolBudgetConfig) BudgetExceededMessage(toolName string, callCount int) string {
	limit := b.LimitFor(toolName)
	if limit == 0 {
		return ""
	}
	return FormatBudgetExceeded(toolName, callCount, limit)
}

// LimitFor returns the per-turn budget for a tool, or 0 for unlimited.
func (b ToolBudgetConfig) LimitFor(toolName string) int {
	switch toolName {
	case "FileRead":
		return b.FileRead
	case "FileSearch":
		return b.FileSearch
	case "Execute":
		return b.Execute
	default:
		return 0
	}
}