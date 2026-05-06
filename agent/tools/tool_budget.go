// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import "fmt"

// PerToolBudget defines the maximum number of calls per tool per turn.
// Tools not in this map have unlimited calls.
// Execute raised from 2 to 10 now that we have proper dangerous-command
// validation (isDangerousCommand) and chain-splitting (splitChainedCommands).
//
// Deprecated: Use ToolBudgetConfig instead. This map remains for backward
// compatibility and will be removed in a future release.
var PerToolBudget = map[string]int{
	"FileRead":    5,  // raised from 3 — common to read several files in one turn
	"FileSearch":  3,  // raised from 2 — often need multiple searches
	"Execute":     10, // raised from 2 — chaining support + approval gate means 2 is too restrictive
}

// CheckBudget returns true if the tool has NOT exceeded its per-turn budget.
// callCount is how many times this tool has already been called this turn.
//
// Deprecated: Use ToolBudgetConfig.CheckBudget instead.
func CheckBudget(toolName string, callCount int) bool {
	return DefaultToolBudget().CheckBudget(toolName, callCount)
}

// BudgetExceededMessage returns a helpful error message for budget-exceeded tools.
//
// Deprecated: Use ToolBudgetConfig.BudgetExceededMessage instead.
func BudgetExceededMessage(toolName string, callCount int) string {
	return DefaultToolBudget().BudgetExceededMessage(toolName, callCount)
}

// FormatBudgetExceeded returns the standard budget-exceeded error message.
func FormatBudgetExceeded(toolName string, callCount int, max int) string {
	return fmt.Sprintf("Tool %s already called %d times this turn (maximum: %d). Use the results you already have instead of calling it again.", toolName, callCount, max)
}