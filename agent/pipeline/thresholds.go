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

	// ConsecutiveLoopThreshold is the number of consecutive tool-call
	// iterations (any tools) that triggers a cross-tool loop detection
	// YIELD. Default: 2 (breaks quickly to avoid silent tool spirals).
	ConsecutiveLoopThreshold int

	// NarrationNudgeThreshold is the number of consecutive tool-only
	// iterations after which a system nudge is injected to encourage
	// the model to narrate its actions. Must be < ConsecutiveLoopThreshold.
	// Set to 0 to disable nudging. Default: 1 (nudge after first tool-only
	// iteration, giving the model one chance to self-correct before YIELD).
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
		ConsecutiveLoopThreshold:   3,
		NarrationNudgeThreshold:    2,
		StuckMinFailures:           2,
		StuckHistoryWindow:        3,
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