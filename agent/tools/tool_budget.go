package tools

import "fmt"

// PerToolBudget defines the maximum number of calls per tool per turn.
// Tools not in this map have unlimited calls.
var PerToolBudget = map[string]int{
	"FileRead":    3,
	"FileSearch":  2,
	"Execute":     2,
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