# Mnemosyne Roadmap

## Completed

### Phase 24: Bubble Tea TUI (COMPLETE)
Branch: `feat/bubbletea-tui`, merged to main (commit 665c88b)

- Phase 1: Program loop, streaming SSE → tea.Msg, slash commands, --tui flag
- Phase 2: Viewport, chat history, turn state machine, lipgloss panels
- Phase 3: textarea multi-line input, input history, tool approval scaffolding
- Phase 3b: Interactive tool approval via tea.Program.Send() + approvalCh (thread-safe)
- Phase 4: Mouse support (WithMouseCellMotion), one-shot TUI mode (RunOneShotBubble)
- Phase 5: Assessment only — tea_model.go uses lipgloss exclusively, no raw ANSI
- **Postscript (May 2026):** Converged on Bubble Tea TUI — removed cli.go entirely. Renamed tui.go → format.go. Split tea_model.go (937 lines) into tea_model.go + tea_handlers.go + tea_renderer.go.

### Phase 25: Saved Tools — Runtime Tool Creation and Sharing (COMPLETE)

**Files:** `daemon/internal/store/saved_tools.go`, `agent/tools/saved_tools.go`, `daemon/internal/store/schema.go` (migration v4)

- `saved_tools` table with FTS5: script-type tools with name, description, parameters (JSON schema), triggers, priority, versioning, `requires_approval_every_call`
- 6 tool handlers: CreateTool, GetTool, ListTools, SearchTools, UpdateTool, RemoveTool
- Trigger-ranked loading — all active tools loaded (up to cap), ranked by (trigger_match × priority)
- Security: CreateTool/UpdateTool/RemoveTool require approval. Script tools inherit Execute security. `requires_approval_every_call` defaults to 1.
- `/tools` command in TUI for discovery
- Full test suite including red-team tests (SQL injection, nil args, etc.)

### Phase 26: Reasoning Model Support (COMPLETE)

**Commit:** 9ced143

- Added `StreamReasoning` event type and `Reasoning` field to `StreamEvent`
- Extended OpenAI SSE parser for `reasoning_content` (DeepSeek) and `reasoning` (OpenAI o-series)
- CLI: reasoning tokens displayed with `💭` dim prefix
- TUI: reasoning accumulated in chat history with live updates

### Phase 29: Accurate Token Counting (COMPLETE)

**Files:** `agent/context/token_counter.go`

- `TokenCounter` interface with `CountTokens(text)` and `CountMessages(msgs)`
- Tiktoken-backed counter for OpenAI models via `CounterForModel(modelName)`
- Character-based fallback with model-specific ratios for Anthropic/Ollama
- Integrated into `ConversationContext.TokenCount()` and context overflow detection

### Phase 28: Namespace / Multi-Tenancy (COMPLETE)

**Files:** `daemon/internal/store/namespace.go`, `daemon/internal/store/schema.go`

- `owner` column on all store tables (memories, sessions, tools, skills)
- API key → owner mapping with gRPC context extraction
- Store-layer query scoping: all queries filtered by owner
- Isolation tests verify cross-tenant data leakage is impossible

### Phase 30: Parallel Tool Dispatch (COMPLETE)

**Files:** `agent/pipeline/parallel_dispatch.go`

- Independent tool calls run concurrently via goroutines with WaitGroup + error collection
- Dependent tool chains (input references prior output ID) run sequentially
- `MaxParallelTools` config bounds concurrency (default: 5)
- Preserves result ordering; budget still enforced
- 8 tests covering: concurrency, dependency ordering, error isolation, bounds, ordering, empty/single cases

### Phase 31: Observer — Memory Extraction via Event Bus (COMPLETE)

**Files:** `agent/memory_extraction_subscriber.go`

- `ConversationCompletePayload` emitted via EventBus on session end
- `MemoryExtractionSubscriber` listens for `conversation_complete` events
- Extraction runs in own goroutine with timeout + 3 retries with backoff
- Uses cheapest available model for extraction (JSON structured task)
- Full test suite: subscribe, receive, skip-on-empty, nil-mnemo, cheapest-model selection

### Phase 32: Prompt Caching (COMPLETE)

**Files:** `agent/pipeline/cache_prep_test.go`, `agent/provider/anthropic.go`

- `RequestPrepStage` calculates `CacheBreakpoint` based on system prompt length
- `CacheDisabled` flag on `ConversationContext` when hot block changes (invalidation)
- Anthropic provider injects `cache_control: ephemeral` on content blocks up to breakpoint
- Metrics track cache hit rate
- 9 tests all passing

### Phase 33: Observability & Structured Logging (COMPLETE)

**Files:** `agent/log/logger.go`, `agent/log/spans.go`, `agent/log/metrics.go`, `agent/log/health.go`

- `log/slog` structured logging with JSON handler to stderr
- Pipeline instrumentation spans: `pipeline.iteration`, `tool.dispatch`, `router.select`
- Key metrics: `turns_total`, `tools_called_total`, `loop_detections_total`, `routing_fallbacks_total`, `cost_accumulated_usd`
- `/health` endpoint with daemon status + last N error summaries

### Phase 35: Evaluation Framework (COMPLETE)

**Files:** `eval/eval.go`, `eval/eval_test.go`

- Scenario runner + judgment model for scoring agent responses
- JSON result storage for trend analysis
- See `eval/` package for details

### Phase 36: SQLite Hardening & Migration Readiness (COMPLETE)

**Files:** `daemon/internal/store/store.go`

- WAL mode enabled (`PRAGMA journal_mode=WAL`)
- `busy_timeout=5000` configured
- Config validation: busy_timeout range check (1000–30000 ms)
- Store interface abstraction verified clean for Postgres swap

### Phase 37: Decorator — Composable Provider Wrappers (COMPLETE)

**Files:** `agent/provider/decorator.go`, `agent/provider/decorator_test.go`

- `ProviderDecorator` interface for composable middleware around LLM providers
- Built-in decorators: `RetryDecorator`, `LoggingDecorator`, `MetricsDecorator`
- Decorators chain via `Wrap(provider, decorators...)`
- Full test suite with mock provider verifying retry, logging, and metrics behavior

### Phase 40: Branch Cleanup & Bug Fixes (COMPLETE)

- Fixed oscillating progress detection: `:keyArg` suffix was not stripped before counting distinct tools, causing false negatives when same tool called with different args (branch `fix/oscillating-key-args`, merged)
- Merged 4 stale branches into main: `feature/auto-memory`, `feature/skills-index`, `fix/sanitize-fts5`, `fix/hot-block-config`
- Confirmed Phase 33 (Observability) and refactor-reduce-debt work already on main
- Deleted 13+ stale local and remote branches — only `main` remains
- Removed dead `tea_states.go` (uncommitted orphan from incomplete Phase 38 attempt)
- All tests passing with -race (23 packages)

---

## Remaining

### Phase 27: GitHub Integration (Monitoring)

**Problem:** When Mnemosyne goes public, we need to monitor the GitHub repo for issues, discussions, PRs, and community submissions.

**Planned components:**
1. GitHub webhook listener
2. Issue-to-memory pipeline
3. Agent notification via event bus
4. GitHub Auth (OAuth / PAT)

---

### Phase 38: State — TUI Phase State Machine

**Pattern:** **State (GoF)** — Replaces the current `phase` enum + switch-based dispatch with polymorphic state objects.

**Problem:** The TUI uses a `phase` enum with 5 values. The `Update()` method (now in `tea_handlers.go`, 554 lines post-split) still has phase checks scattered throughout. Adding a new phase means touching 3+ files. This is the classic State pattern smell.

**Proposal:**
1. Define `TUIState` interface with `Update`, `View`, `Enter`, `Exit`.
2. Create state objects: `ReadyState`, `ThinkingState`, `StreamingState`, `ToolCallState`, `ApprovalState`, `ErrorState`.
3. Move phase-specific fields into their state objects (e.g., `spinner` lives in `ThinkingState`).
4. `teaModel.Update()` delegates to `m.state.Update()`, which returns the next state.

**Estimated effort:** 2 sessions.

---

### Phase 39: Mediator — Explicit Pipeline Data Bus

**Pattern:** **Mediator (GoF)** — Introduces a `PipelineBus` that stages write results to and read results from, replacing implicit mutation of `ConversationContext`.

**Problem:** Pipeline stages communicate by mutating `ConversationContext` directly. Stage 4 appends tool results; Stage 5 reads them. This implicit data flow means you can't tell what a stage reads/writes without reading its source. Testing a stage in isolation requires constructing the full `ConversationContext`.

**Proposal:**
1. Define `PipelineBus` struct with fields for each inter-stage data dependency.
2. Refactor each stage's `Process()` to read from bus and write to bus.
3. `ConversationContext` becomes read-only during a single iteration.
4. Dramatically simplifies testing: mock a bus with known inputs, call one stage, assert bus outputs.

**Estimated effort:** 3 sessions. Touches every pipeline stage and the loop skeleton.

---

## Suggested Priority Order

| Priority | Phase | Rationale |
|----------|-------|-----------|
| 1 | **Phase 38** (State — TUI) | Maintainability — phase checks scattered across handlers; partial implementation already on main |
| 2 | **Phase 39** (Mediator — Pipeline Bus) | Clarity — explicit data flow; enables cleaner isolated tests |
| 3 | **Phase 27** (GitHub Integration) | Community — needed for open-source launch |