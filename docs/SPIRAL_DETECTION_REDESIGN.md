# Spiral Detection Redesign Proposal

## Problem Statement

The current spiral detection uses `ConsecutiveLoopThreshold=3` (YIELD after 3 consecutive
tool-only iterations) and `NarrationNudgeThreshold=2` (inject nudge after 2). This approach:

1. **Too restrictive for legitimate tasks**: Code review, research, and file analysis
   naturally involve 4-8 sequential tool calls. The model gets interrupted mid-task.

2. **Too lenuent for actual spiraling**: The model can make 2 calls, produce one line of
   text (resetting the counter), then make 2 more calls, repeating indefinitely. The
   nudge doesn't help because GLM-5.1 often ignores it.

3. **Can't distinguish progress from repetition**: Reading file A, then file B, then
   running a test is *progress*. Calling SearchMemories("foo") three times is *repetition*.

## Root Cause Analysis

The current `countConsecutiveToolIterations()` function (line 282-306 in context_management.go)
walks backwards through messages counting consecutive assistant messages with empty Content
and non-empty ToolCalls. It cannot tell the difference between:

- **Progress**: `SearchMemories → GetSession → FileRead` (different tools, different args)
- **Repetition**: `SearchMemories("foo") → SearchMemories("foo") → SearchMemories("foo")`
- **Spiral**: `SearchMemories → FileRead → SearchMemories → FileRead` (oscillating)

All three patterns are treated identically — as "consecutive tool iterations."

## Proposed Redesign: Three-Zone Detection

Zone 1: **Repetition Detection** (same tool, similar args, same result)
  - Calls to the *same tool* with *similar arguments* that produce *similar or empty results*
  - Threshold: 3 consecutive same-tool repetitions (configurable: `RepetitiveLoopThreshold`)
  - Action: YIELD immediately — no nudge, no second chance
  - This is the existing `isRepetitiveToolLoop()` — already correct

Zone 2: **Progress Tracking** (are we making forward progress?)
  - Track a "progress signature" per iteration: hash of (tool_name, key_args, result_summary)
  - If the progress signature is *novel* (new tool, new args, or new result), reset the counter
  - If the progress signature *duplicates* a recent one, increment the counter
  - Threshold: 2 duplicate progress signatures → NUDGE, 3 → YIELD
  - This replaces `ConsecutiveLoopThreshold` and `NarrationNudgeThreshold`

Zone 3: **Iteration Cap** (safety valve)
  - Absolute maximum iterations per turn regardless of progress
  - Configurable: `MaxIterationsPerTurn` (default: 15)
  - Action: YIELD with final response text
  - This is already handled by `AppendAndContinueHandler` checking `convCtx.Iteration >= convCtx.MaxIterations`

## Implementation Plan

### 1. Add ProgressTracker to ConversationContext

```go
// ProgressSignature is a compact hash of what happened in an iteration.
type ProgressSignature struct {
    ToolNames  string   // sorted, joined tool names: "FileRead,SearchMemories"
    KeyArgs    string   // first 100 chars of key arguments, hashed
    ResultTag  string   // "success", "error", "not_found", "empty"
}

// Add to ConversationContext:
type ConversationContext struct {
    // ... existing fields ...
    ProgressHistory []ProgressSignature  // last N progress signatures
    DuplicateCount  int                   // consecutive duplicate signatures
}
```

### 2. Add ProgressDetectionHandler to the Chain

Place it between RepetitiveLoopHandler and NarrationNudgeHandler:

```
OverflowHandler → StuckHandler → RepetitiveLoopHandler → ProgressDetectionHandler → AppendAndContinueHandler
```

Remove: NarrationNudgeHandler, ConsecutiveLoopHandler (replaced by ProgressDetectionHandler).

```go
type ProgressDetectionHandler struct {
    Thresholds PipelineThresholds
    EventBus   *eventbus.EventBus
}

func (h *ProgressDetectionHandler) Handle(convCtx, toolResults, parsed) (action, handled) {
    // Skip after compaction
    if convCtx.CompactedThisTurn { return CONTINUE, false }
    
    // Compute progress signature for this iteration
    sig := computeProgressSignature(parsed.ToolCalls, toolResults)
    
    // Check against recent history
    if isDuplicateProgress(sig, convCtx.ProgressHistory) {
        convCtx.DuplicateCount++
    } else {
        convCtx.DuplicateCount = 0
    }
    convCtx.ProgressHistory = append(convCtx.ProgressHistory, sig)
    // Keep only last 10 signatures
    if len(convCtx.ProgressHistory) > 10 {
        convCtx.ProgressHistory = convCtx.ProgressHistory[len(convCtx.ProgressHistory)-10:]
    }
    
    // Nudge zone: 1-2 duplicates (not yet spiraling, but not making progress)
    if convCtx.DuplicateCount >= 1 && convCtx.DuplicateCount < h.Thresholds.ProgressYieldThreshold {
        // Only nudge if no text was produced
        if parsed.Text == "" {
            injectNarrationNudge(convCtx, convCtx.DuplicateCount)
            return CONTINUE, false  // Continue, but the nudge is in context
        }
    }
    
    // Yield zone: 3+ duplicates without progress
    if convCtx.DuplicateCount >= h.Thresholds.ProgressYieldThreshold {
        emitEvent(h.EventBus, LoopDetectionEvent{
            Kind: "progress_stall",
            Message: fmt.Sprintf("no progress after %d duplicate iterations", convCtx.DuplicateCount),
        })
        return YIELD, true
    }
    
    return CONTINUE, false
}
```

### 3. isDuplicateProgress — the key function

```go
// isDuplicateProgress checks if the current signature duplicates any of the
// last 3 signatures. "Duplicate" means:
// - Same tool names (regardless of order)
// - Same result tag (success/error/not_found)
// OR
// - Exact same tool names AND same key arguments (different results, same query)
func isDuplicateProgress(sig ProgressSignature, history []ProgressSignature) bool {
    // Check last 3 signatures
    start := len(history) - 3
    if start < 0 { start = 0 }
    for i := start; i < len(history); i++ {
        if sig.ToolNames == history[i].ToolNames && sig.ResultTag == history[i].ResultTag {
            return true
        }
    }
    return false
}
```

### 4. Updated Thresholds

```go
type PipelineThresholds struct {
    // ... existing fields ...
    
    // REMOVED: ConsecutiveLoopThreshold (replaced by ProgressYieldThreshold)
    // REMOVED: NarrationNudgeThreshold (replaced by progress-based nudging)
    
    // ProgressYieldThreshold is the number of duplicate progress signatures
    // before yielding. Default: 3 (allows 2 duplicate iterations, yields on 3rd).
    ProgressYieldThreshold int
}

func DefaultThresholds() PipelineThresholds {
    return PipelineThresholds{
        AggressiveSummaryThreshold: 500,
        RepetitiveLoopThreshold:     3,
        ProgressYieldThreshold:      3,  // yield after 3 no-progress iterations
        StuckMinFailures:            2,
        StuckHistoryWindow:         3,
        MaxConsecutiveErrors:        3,
    }
}
```

### 5. Impact on Existing Tests

- Remove NarrationNudgeHandler from the chain
- Remove ConsecutiveLoopHandler from the chain
- Add ProgressDetectionHandler
- Update tests that assert on NudgeThreshold/ConsecutiveLoop
- Add new tests for progress detection

### 6. Config Overrides

Add to config.yaml under pipeline:

```yaml
pipeline:
  progress_yield_threshold: 3    # yield after N no-progress iterations
  repetitive_loop_threshold: 3   # yield after N same-tool calls
  max_iterations: 20              # safety valve cap
```

## Expected Behavior Changes

| Scenario | Current Behavior | Proposed Behavior |
|----------|-----------------|-------------------|
| Search → Get → FileRead | NUDGE after 2nd, YIELD after 3rd | All different tools, all novel results → CONTINUE |
| Search("foo") → Search("foo") → Search("foo") | NUDGE after 2nd, YIELD after 3rd | Same tool, same args → RepetitiveLoopHandler YIELDs at 3 |
| Search → FileRead → Search → FileRead (oscillating) | YIELD after 3rd call | Duplicate progress signatures → ProgressYieldThreshold YIELDs at 3 |
| 15 sequential file reads in code review | YIELD after 3rd call | All novel signatures → CONTINUE (up to MaxIterations) |
| Search → Search (no results) → Search (no results) | YIELD after 3rd | Same tool, same result_tag=not_found → RepetitiveLoopHandler YIELDs at 3 |
| GLM-5.1 tool spiral (no text) | NUDGE at 2, YIELD at 3 | No progress (duplicates) → YIELD at 3 |

## Migration Path

1. Add ProgressDetectionHandler (with its own threshold)
2. Keep NarrationNudgeHandler and ConsecutiveLoopHandler but set their thresholds to 0 (disabled)
3. Verify tests pass with ProgressDetectionHandler active
4. Remove old handlers in follow-up commit

This gives us a clean rollback path — if ProgressDetection causes issues, set its threshold to 0 and re-enable the old handlers.