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

	// RepetitiveLoopThreshold is the number of consecutive same-tool calls
	// that triggers a repetitive loop detection YIELD. Default: 3
	// (allows 2 retries, breaks on the 3rd).
	RepetitiveLoopThreshold int

	// ProgressYieldThreshold is the number of consecutive no-progress
	// iterations (same tool names, same result pattern) before yielding.
	// A "no-progress" iteration is one where the progress signature
	// duplicates a recent one in the history. Default: 3.
	// This replaces ConsecutiveLoopThreshold and NarrationNudgeThreshold
	// with a smarter detection that distinguishes legitimate multi-step
	// tool use (different tools, different results) from actual spirals
	// (same tool, same result, no forward progress).
	ProgressYieldThreshold int

	// ConsecutiveLoopThreshold is the LEGACY threshold for consecutive
	// tool-only iterations (any tools) that triggers a YIELD.
	// DEPRECATED: superseded by ProgressYieldThreshold. Set to 0 to disable.
	ConsecutiveLoopThreshold int

	// NarrationNudgeThreshold is the LEGACY threshold for injecting a
	// system nudge after N consecutive tool-only iterations.
	// DEPRECATED: superseded by ProgressDetectionHandler. Set to 0 to disable.
	NarrationNudgeThreshold int

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
		RepetitiveLoopThreshold:    3,
		ProgressYieldThreshold:     3,
		ConsecutiveLoopThreshold:   0, // disabled — ProgressDetection replaces it
		NarrationNudgeThreshold:    0, // disabled — ProgressDetection replaces it
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