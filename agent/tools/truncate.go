package tools

import "fmt"

// truncateOutput truncates long output while preserving beginning and end context
func truncateOutput(result string, maxChars int) string {
	if len(result) <= maxChars {
		return result
	}

	half := maxChars / 2
	quarter := maxChars / 4

	beginning := result[:half]
	end := result[len(result)-quarter:]

	return fmt.Sprintf("%s\n\n... (truncated, %d chars total, showing first %d and last %d)\n\n%s",
		beginning, len(result), half, quarter, end)
}
