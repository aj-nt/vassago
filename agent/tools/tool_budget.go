// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import "fmt"

// PerToolBudget defines the maximum number of calls per tool per turn.
// Tools not in this map have unlimited calls.
// Execute raised from 2 to 10 now that we have proper dangerous-command
// validation (isDangerousCommand) and chain-splitting (splitChainedCommands).
var PerToolBudget = map[string]int{
	"FileRead":    5,  // raised from 3 — common to read several files in one turn
	"FileSearch":  3,  // raised from 2 — often need multiple searches
	"Execute":     10, // raised from 2 — chaining support + approval gate means 2 is too restrictive
}

// CheckBudget returns true if the tool has NOT exceeded its per-turn budget.
// callCount is how many times this tool has already been called this turn.
func CheckBudget(toolName string, callCount int) bool {
	max, ok := PerToolBudget[toolName]
	if !ok {
		return true // unlimited
	}
	return callCount < max
}

// BudgetExceededMessage returns a helpful error message for budget-exceeded tools.
func BudgetExceededMessage(toolName string, callCount int) string {
	max, ok := PerToolBudget[toolName]
	if !ok {
		return ""
	}
	return fmt.Sprintf("Tool %s already called %d times this turn (maximum: %d). Use the results you already have instead of calling it again.", toolName, callCount, max)
}