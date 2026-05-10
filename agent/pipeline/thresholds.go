// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

// PipelineThresholds holds all configurable thresholds for the pipeline's
// context management and loop detection. Using a single struct makes the
// magic numbers discoverable, testable, and overridable.
//
// Defaults are defined by DefaultThresholds(). Override individual fields
// in tests or config-driven construction.
type PipelineThresholds struct {
	// AggressiveSummaryThreshold is the character count above which tool
	// results are aggressively summarized to "[tool result: Name (N chars)]"
	// during overflow compaction. Used by OverflowHandler and
	// AppendAndContinueHandler.
	AggressiveSummaryThreshold int

	// ProgressYieldThreshold is the number of consecutive no-progress
	// iterations (same tool names, same result pattern) before yielding.
	// Default: 5. Reading 3 files in sequence produces 3 identical
	// FileRead/success signatures, but that's legitimate work. Only
	// yield after 5+ identical signatures — a genuine spiral like
	// SearchMemories/not_found repeated 5 times.
	ProgressYieldThreshold int

	// IntentLoopThreshold is the number of times the same intent bucket
	// (e.g., "search", "read", "execute") must appear in recent progress
	// history to trigger an intent nudge. This catches cross-tool loops
	// where the agent switches tools but pursues the same goal.
	// Default: 5 (5 iterations of "search" even if using FileSearch,
	// SearchMemories, and Execute).
	IntentLoopThreshold int

	// StuckMinFailures is the minimum number of consecutive tool failures
	// (errors or "not found") required for stuck detection. Default: 2.
	StuckMinFailures int

	// StuckHistoryWindow is how many recent tool results to inspect from
	// conversation history when checking for stuck patterns. Default: 3.
	StuckHistoryWindow int

	// MaxConsecutiveErrors is the maximum number of consecutive provider
	// errors before the pipeline gives up. Default: 3 for streaming mode,
	// 1 for blocking mode.
	MaxConsecutiveErrors int
}

// DefaultThresholds returns the production defaults for pipeline thresholds.
func DefaultThresholds() PipelineThresholds {
	return PipelineThresholds{
		AggressiveSummaryThreshold: 500,
		ProgressYieldThreshold:     5,
		IntentLoopThreshold:        5,
		StuckMinFailures:           2,
		StuckHistoryWindow:         3,
		MaxConsecutiveErrors:       3,
	}
}

// BlockingThresholds returns thresholds suitable for blocking mode
// (single provider error is fatal).
func BlockingThresholds() PipelineThresholds {
	t := DefaultThresholds()
	t.MaxConsecutiveErrors = 1
	return t
}
