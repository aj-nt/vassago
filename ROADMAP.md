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

### Phase 26: Reasoning Model Support (COMPLETE)

**Commit:** 9ced143

- Added `StreamReasoning` event type and `Reasoning` field to `StreamEvent` (agent/context/stream.go)
- Extended OpenAI SSE parser to recognize `reasoning_content` (DeepSeek) and `reasoning` (OpenAI o-series) delta fields
- Accumulated reasoning text in `streamingCaller` into `ParsedResponse.Reasoning`
- CLI: reasoning tokens displayed with `💭` dim prefix, clean newline transition to content
- TUI: reasoning accumulated in chat history with live updates, reset on content start

---

## Planned

### Phase 25: Saved Tools — Runtime Tool Creation and Sharing

**Problem:** Tools are compiled Go functions. The set of tools is fixed at build time. An agent cannot create a new tool during a session, and it cannot share a procedure it discovers with other agents or future sessions. Skills (procedural knowledge) are prompt-injected — the agent reads them but doesn't "call" them. What's missing is **executable procedural memory**: procedures the agent creates, saves to the DB, and calls as first-class tools in future sessions.

**⚠️ Review — Strengths and Gaps:**

The existing spec is thorough on the happy path (create, store, load, call). But there are four gaps that will surface immediately in real use:

**Gap 1: No versioning or mutation tracking.** The schema has `updated_at` but no `version` column, no `previous_script` audit field, and the `UNIQUE` constraint on `name` means `UpdateTool` silently overwrites. If an agent updates `deploy_service` and breaks it, there's no rollback and no way to know what changed. Fix: add a `version INTEGER NOT NULL DEFAULT 1` column and keep the last N versions in a `saved_tool_versions` table (or just store the diff). `UpdateTool` increments version. `GetTool` returns the current version; add `GetToolVersion(id, version)` for rollback.

**Gap 2: Execution has no sandbox boundary.** Script-type tools "run through the existing Execute handler" — but Execute already has `NeedsApproval()`, path validation, and command validation. The spec says "same validation as Execute," which is correct but insufficient. A saved script is a *persistent* code execution surface. If the agent creates a script with `rm -rf $1` and the user approved it once, it can be called again in any future session without re-approval unless we add a flag. Fix: add a `requires_approval_every_call BOOLEAN NOT NULL DEFAULT 1` column. Default is 1 (always ask). The user can set it to 0 for trusted, idempotent tools like `calculate_natal_chart`. This is a security boundary, not a convenience toggle.

**Gap 3: Trigger-based loading will be brittle.** The spec says "Tools whose triggers match the user's first message are loaded" with a priority fallback. But trigger matching against a single message is fragile — "deploy to staging" matches `["deploy", "staging"]`, but "can you push the latest build to the test environment?" matches nothing. The priority fallback (top 10 by priority) saves this, but it means trigger matching is effectively just a ranking mechanism, not a filtering one. This is fine, but the spec should be honest about it. Fix: rename "trigger-based loading" to "trigger-ranked loading." Always load tools up to a configurable cap (default: 10), prioritized by (trigger match × priority), with exact trigger matches getting a ×3 boost.

**Gap 4: The `prompt_template` type is redundant with Skills.** Look at the capability table:

| Feature | Persistent? | Callable by LLM? | Executable? |
|---------|-------------|-------------------|-------------|
| Skills | DB + FTS5 | No (system prompt) | No |
| Saved Tools (prompt_template) | DB + FTS5 | Yes (as tool def) | No |

A `prompt_template` saved tool and a skill are the same thing — a stored text procedure — with one difference: the LLM calls one as a tool and reads the other from the system prompt. But the LLM doesn't *need* to call a prompt template as a tool. It can already follow the same instructions from the skill block. Adding `prompt_template` as a saved tool type creates a parallel category that agents and users have to reason about separately. Fix: drop `prompt_template` from v1. Saved Tools are exclusively `script` type. If prompt templates prove useful later, add them then — but start with the unambiguous case.

**Updated recommendation:**

| Tool Type | Supported in v1? | Why |
|-----------|-------------------|-----|
| `script` (bash/python) | ✅ Yes | Clear value — reusable executable procedures |
| `prompt_template` | ❌ No | Redundant with Skills; revisit if use case emerges |
| `http_request` | 🔮 Future | Call external APIs with parameterized URLs/headers — natural extension, not v1 |

**Example use cases:**

| Tool Type | Example | What it does |
|-----------|---------|--------------|
| `script` | `deploy_service` | Takes `{env: staging\|prod, service: string}`, substitutes into a bash template, runs via Execute with approval |

**Relationship to existing features:**

| Feature | What it is | Persistent? | Callable by LLM? | Executable? |
|---------|-----------|-------------|-------------------|-------------|
| **Memories** | Factual knowledge | DB + FTS5 | No (hot block) | No |
| **Skills** | Procedural knowledge (how-to) | DB + FTS5 | No (system prompt) | No |
| **Saved Tools** | Callable executable procedures | DB + FTS5 | **Yes** (as tool defs) | **Yes** |
| **Compiled Tools** | Go functions | Binary | Yes | Yes |

#### Architecture

**Database schema (v11 migration):**

```sql
CREATE TABLE saved_tools (
    id          TEXT PRIMARY KEY,              -- UUID
    name        TEXT NOT NULL UNIQUE,           -- e.g., "calculate_natal_chart"
    description TEXT NOT NULL,                  -- LLM-readable description for tool schema
    type        TEXT NOT NULL CHECK(type IN ('script')),  -- 'script' only in v1
    parameters  TEXT NOT NULL,                  -- JSON schema for inputs
    script      TEXT NOT NULL DEFAULT '',        -- executable content (bash/python)
    category    TEXT NOT NULL DEFAULT '',        -- e.g., "astrology", "devops"
    triggers    TEXT NOT NULL DEFAULT '[]',      -- JSON array of trigger keywords for discovery
    author      TEXT NOT NULL,                   -- agent ID that created it
    priority    INTEGER NOT NULL DEFAULT 3,      -- 1-5
    version     INTEGER NOT NULL DEFAULT 1,      -- incremented on each UpdateTool
    requires_approval_every_call INTEGER NOT NULL DEFAULT 1,  -- 1 = ask every time, 0 = ask once
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE VIRTUAL TABLE saved_tools_fts USING fts5(
    name, description, triggers, category, content=saved_tools
);
```

**Security model:**

- **CreateTool requires approval** — persistent tools are a security boundary. The agent must explain what the tool does, and the user confirms.
- **Script tools inherit all Execute security** — `NeedsApproval()`, path validation, command validation. A saved script doesn't bypass any checks.
- **`requires_approval_every_call` defaults to 1** — every invocation of a saved tool requires user approval, even if the same tool was approved before. This is the safe default. The user can set it to 0 for trusted, idempotent, side-effect-free tools.
- **UpdateTool/RemoveTool require approval** — same as CreateTool.
- **No new code execution surface** — scripts run through the existing Execute handler. No eval, no arbitrary code execution beyond what Execute already allows.
- **Audit trail** — `author` field tracks which agent created each tool. `version` increments on updates. `created_at`/`updated_at` for traceability.
- **Trigger-ranked loading** — all active tools are loaded (up to a configurable cap, default 10), ranked by (trigger_match_boost × priority). Exact trigger matches get a ×3 boost. This keeps the tool list manageable while ensuring the right tools surface.

**Implementation phases:**

**25A: Storage layer** (daemon/internal/store/saved_tools.go)
- `saved_tools` table + FTS5, schema migration v10→v11
- Store CRUD: AddSavedTool, GetSavedTool, ListSavedTools, SearchSavedTools, UpdateSavedTool (increments version), RemoveSavedTool
- gRPC service methods
- gRPC handler tests
- Estimated: ~350 lines store, ~150 lines gRPC, ~100 lines tests

**25B: Agent tools** (agent/tools/saved_tools.go)
- 6 tool handlers + JSON schemas: CreateTool, GetTool, ListTools, SearchTools, UpdateTool, RemoveTool
- Tool approval for CreateTool/UpdateTool/RemoveTool (leverages Phase 3b approval system)
- Agent tool tests
- Estimated: ~400 lines + ~150 lines tests

**25C: Runtime tool loading and execution** (agent/agent.go)
- `GetSavedToolDefinitions()` — fetch and convert to ToolDefinitions with trigger-ranked loading
- `RegisterSavedToolHandlers()` — create dynamic handlers
- Script handler: parameter substitution → Execute with security + `requires_approval_every_call` check
- Integration test: create tool, verify it appears in tool list, call it
- Estimated: ~250 lines + ~100 lines tests

**25D: TUI discovery** (agent/interface/tea_model.go)
- `/tools` command — list saved tools with name, type, description, version
- One-shot: `mnemosyne chat --message "create a tool that..."` works naturally
- Estimated: ~100 lines

**Total estimated effort:** 4-5 sessions for 25A-D.

---

### Phase 27: GitHub Integration (Monitoring)

**Problem:** When Mnemosyne goes public, we need to monitor the GitHub repo for issues, discussions, PRs, and community submissions.

**Planned components:**
1. GitHub webhook listener
2. Issue-to-memory pipeline
3. Agent notification via event bus
4. GitHub Auth (OAuth / PAT)

---

## Architectural Review — GoF Design Patterns

A review of the current codebase through the lens of the Gang of Four catalog. Patterns already in use, patterns partially applied, and patterns that should be introduced.

### ✅ Patterns Already in Use

| Pattern | Where | How |
|---------|-------|-----|
| **Template Method** | `pipeline/loop.go` → `RunPipelineLoop()` | The 5-stage loop skeleton (lines 73–185) calls `caller.Call()` as the variable step. `RunWithContext` provides `BlockingCaller`, `RunStreaming` provides `StreamingCaller`. Same skeleton, different provider-call strategy. |
| **Strategy** | `pipeline/loop.go` → `ProviderCaller` interface | `BlockingCaller` and `StreamingCaller` are interchangeable strategies for Stages 2–3. The loop doesn't know which it has. |
| **Chain of Responsibility** | `pipeline/context_management.go` → `HandlerChain` | `OverflowHandler` → `StuckHandler` → `ProgressDetectionHandler` → `AppendAndContinueHandler`. Each handler decides independently whether to handle the request. First handler returning `handled=true` wins. |
| **Builder** | `agent/builder.go` → `AgentBuilder` | Fluent API for constructing an `Agent` with optional router, event bus, and stage overrides. Eliminates constructor proliferation (`NewAgent`, `NewAgentWithRouter`, `SubAgentRunnerImpl`). |
| **Factory Method** | `agent/delegate.go` → `RunSubAgent()` | Creates sub-agents with restricted tool registries. The factory decides which tools the sub-agent gets (memory only, no delegation). |
| **Observer** | `agent/eventbus/bus.go` → `EventBus` | Pub/sub for pipeline observability. `Emit()` on `tool_call`, `tool_result`, `iteration_start`, `iteration_end`. Display/logging/metrics subscribe. **Currently underutilized — see Phase 31.** |
| **Proxy** | `client/null_client.go` → `NullMnemo` | Stands in for the real gRPC client when the daemon is unavailable. Same interface, no-op behavior. |
| **Adapter** | `agent/provider/openai_stream.go` | Translates OpenAI's SSE wire format into the canonical `StreamEvent` type that the pipeline loop consumes. Anthropic format → same canonical type. |

### ⚠️ Patterns Partially Applied (Gaps to Close)

| Pattern | Where | Gap |
|---------|-------|-----|
| **Observer** | `EventBus` | The bus fires events, but no subscriber handles `conversation_complete` for memory extraction. The pipeline still calls `extractAndPersistMemories` directly. **Fix: Phase 31.** |
| **Decorator** | Provider interface | `Prepare → Execute → Parse` is a 3-step protocol, but there's no way to add cross-cutting behavior (retry, caching, metrics) without modifying each provider. **Fix: Phase 37.** |
| **Mediator** | Pipeline stages | Stages talk directly to each other via `ConversationContext` mutation. Stage 4 appends tool results, Stage 5 reads them. This works but makes the data flow implicit — you have to read each stage to know what it writes. **Fix: Phase 38.** |

### 🆕 Patterns to Introduce

| Pattern | Phase | Rationale |
|---------|-------|-----------|
| **Observer** (complete) | 31 | Decouple memory extraction from the pipeline loop via `conversation_complete` event |
| **Decorator** | 37 | Add retry, caching, and metrics as composable wrappers around the Provider interface |
| **State** | 38 | Make pipeline phases explicit — replace `phaseReady/phaseThinking/phaseStreaming/phaseToolCall/phaseApproval` with a proper State pattern in the TUI |
| **Mediator** | 39 | Introduce a `PipelineBus` that stages write results to and read results from, making data flow explicit |

---

### Phase 28: Namespace Isolation — Foundation for Multi-Tenancy

**Pattern:** **Proxy (GoF) + Namespace** — Adds an `owner` column to every table and scopes all queries through it, without building full auth. The existing `APIKeyInterceptor` maps API key → owner identity. This is the minimum viable step that unblocks all future multi-user work.

**Problem:** The current schema has no concept of "who owns this data." `memories` has `source_agent`, but the `UNIQUE INDEX idx_memories_target_key ON memories(target, key)` means two users can't have different memories with the same key. Skills, sessions, todos, and cron jobs are similarly flat. There's no way to scope data to a user or organization.

The current entry says "Deferred. Requires design for API keys, agent isolation, per-user memory namespaces." This is too vague — it implies a monolithic auth project that will block everything else. The right approach is to split it: **namespace isolation first, auth later.**

**Why not build full auth now?** OAuth flows, JWT tokens, session management, password reset — these are well-solved problems but complex to build and test. They're also entirely unused until there's a second user. Building them now means maintaining untested auth code for months. Worse, they'd couple the daemon to a specific auth model before we know what the actual multi-user pattern will be (shared household? development team? SaaS platform?).

**Why namespace isolation now?** It's a pure schema refactor with no new infrastructure. The interceptor chain already exists (auth.go + interceptors.go). Adding `owner` to the schema means:
- All queries gain `WHERE owner = ?` — zero new code paths, just broader WHERE clauses
- The existing `APIKeyInterceptor` maps key → owner — one lookup, already wired
- Single-user deployments are unchanged — `owner = ""` is the default namespace
- When real auth arrives, the namespace is already partitioned — no data migration

**Recommended approach:**

1. Add `owner TEXT NOT NULL DEFAULT ''` column to: `memories`, `skills`, `sessions`, `todos`, `cron_jobs`.
2. Update `memories_fts` and `skills_fts` to include `owner` in their content tables (or add `owner` as a filter column).
3. Update the store layer: every query method gains an `owner` parameter. Methods that already accept a struct (AddMemory, CreateSession) gain the field.
4. Wire the interceptor: `APIKeyInterceptor` extracts the API key, looks up the associated owner in a new `api_keys` table (or config), and injects it into the gRPC context via metadata.
5. Update gRPC handlers to extract `owner` from context and pass to store methods.
6. Single-user deployments: `owner = ""` (default). No API key required. No behavior change.

**What this explicitly does NOT do:**
- No OAuth. No JWT. No password hashing. No user registration flow.
- No per-user permissions or RBAC.
- No isolation between agents owned by the same user.
- No rate limiting per user.

**What this enables:**
- Multiple users sharing a daemon without data collision
- API keys per user (via `api_keys` table mapping key → owner)
- Future auth can be added behind the same `owner` column — just change how `owner` is resolved
- Safe open-source deployment: contributors can run their own namespace

**Estimated effort:** 2 sessions. Touches every store method and gRPC handler, but each change is mechanical (`owner` added to WHERE clauses).

---

### Phase 29: Accurate Token Counting

**Problem:** The current token estimation (`chars/4` with a 10% safety margin) is crude. Actual tokenizers (tiktoken, BPE) can produce 2–3× more tokens than `chars/4` for code, non-English text, emoji, and special characters. The safety margin catches some of this, but context overflow detection is unreliable — we either terminate too early (wasting context window) or too late (hitting hard API errors).

**Proposal:**
1. Add a `TokenCounter` interface in `agent/context/`:
   ```go
   type TokenCounter interface {
       CountTokens(text string) int
       CountMessages(msgs []Message) int  // accounts for role overhead
   }
   ```
2. Implement a tiktoken-backed counter (Go port or subprocess) for OpenAI models, and a character-based fallback with model-specific ratios for Anthropic/Ollama.
3. Use model-specific counters per the routing config — each `ModelConfig` already has a `Provider` field, which determines the tokenizer.
4. Replace `TokenCount()` calls throughout the pipeline with the accurate counter.

**Estimated effort:** 1–2 sessions.

---

### Phase 30: Parallel Tool Dispatch

**Problem:** Stage 4 (`ToolDispatchStage`) dispatches tool calls sequentially. Modern LLMs (GPT-4o, Claude Sonnet 4, etc.) routinely emit multiple independent tool calls per turn. Running them sequentially wastes latency waiting for I/O-bound tools (web fetch, file read, memory search) that don't depend on each other.

**Proposal:**
1. Analyze tool call dependencies in `ToolDispatchStage.Process()`: tools whose inputs don't reference prior tool outputs can run concurrently.
2. Dispatch independent tools via goroutines with a `sync.WaitGroup` + error collection.
3. Maintain ordering for dependent tool chains (tool B's input references tool A's output ID).
4. Add a `MaxParallelTools` config (default: 5) to bound concurrency.

**Estimated effort:** 2 sessions.

---

### Phase 31: Observer — Memory Extraction via Event Bus

**Pattern:** **Observer (GoF)** — Decouples the pipeline loop from memory persistence by emitting a domain event that subscribers handle independently.

**Problem:** `ExtractMemories` (agent/memory_extract.go) runs synchronously after every session via `FinallyFunc`. If the LLM provider is slow or down, it blocks the session close path (though `FinallyFunc` is in a defer). More importantly, there's no retry, no queuing, and the extraction uses the same primary provider even if it's expensive. A long conversation could have 50+ messages to extract from, making the extraction call itself costly.

The deeper issue is structural: the pipeline loop is tightly coupled to memory persistence. The `EventBus` already exists and already fires events at every checkpoint (`iteration_start`, `tool_call`, `tool_result`, `response`, `iteration_end`), but nothing listens for memory-relevant events. This is Observer without subscribers — the infrastructure is in place but unused.

**Current coupling:**
```go
// agent.go — FinallyFunc calls extractAndPersistMemories directly
FinallyFunc: func(sid string, result *pipeline.LoopResult) {
    a.extractAndPersistMemories(ctx, sid, convCtx, result)
},
```

**Observer refactor:**
```go
// Instead of calling extraction directly, emit an event:
FinallyFunc: func(sid string, result *pipeline.LoopResult) {
    a.EventBus.Emit(eventbus.Event{
        Kind: "conversation_complete",
        Data: ConversationCompletePayload{
            SessionID:  sid,
            Messages:   convCtx.Messages,
            AgentName:  a.Config.AgentName,
        },
    })
},
```

Then in `setup.go`, a subscriber handles it asynchronously:
```go
bus.Subscribe("conversation_complete", func(e Event) {
    go extractMemoriesAsync(e.Data) // own goroutine, own timeout, own model selection
})
```

This adds no new infrastructure — the `EventBus` is already wired through every stage. Additional subscribers could listen for analytics, cost tracking, or session summarization without touching the pipeline.

**Proposal:**
1. Emit `conversation_complete` event from `FinallyFunc` instead of calling extraction directly.
2. Create `MemoryExtractionSubscriber` that listens on `conversation_complete`, runs extraction in its own goroutine with timeout + 3 retries with backoff.
3. Use the cheapest available model for extraction (structured JSON task, not complex reasoning).
4. Store extraction status in the session row (`extraction_status`: pending/complete/failed) so the daemon can retry failed extractions on its own schedule.
5. Add a gRPC endpoint `TriggerMemoryExtraction(sessionID)` for manual retry.

**Estimated effort:** 1–2 sessions.

---

### Phase 32: Prompt Caching for System Context

**Problem:** The system prompt, hot block, and skill block are sent on every turn but rarely change within a session. Anthropic and OpenAI both support prompt caching that can reduce input costs by up to 90% for repeated prefix content. Currently `SupportsPromptCaching` is hardcoded to `false` in the provider interface.

**Proposal:**
1. In `RequestPrepStage`, mark the system prompt + hot block + skill block segment with a cache breakpoint marker.
2. Extend the OpenAI and Anthropic providers to set the appropriate cache-control headers/ephemeral markers.
3. Track cache hit rates in `TurnCost` entries to measure savings.
4. Disable caching when the hot block changes mid-session (Telepathy invalidation).

**Estimated effort:** 2 sessions. Potential cost savings: 50–70% on input tokens for long sessions.

---

### Phase 33: Observability & Structured Logging

**Problem:** The agent uses `log.Printf` throughout for all diagnostics. There's no structured logging, no OpenTelemetry traces, no metrics export. For a system that runs multi-turn tool-use loops with model routing, debugging production issues (why did the router pick model X? why did loop detection fire? how long did tool Y take?) requires grep'ing logs.

**Proposal:**
1. Adopt `log/slog` (stdlib since Go 1.21) with structured key-value pairs. Replace all `log.Printf` calls.
2. Add a `slog.Handler` that writes JSON to stderr (or a file) for tool consumption.
3. Instrument the pipeline loop with spans:
   - `pipeline.iteration` span with attributes: iteration, model, provider, tool_count, action
   - `tool.dispatch` span per tool call with: tool_name, duration, is_error
   - `router.select` span with: strategy, selected_model, reason
4. Emit key metrics as counters/gauges (even if just logged): `turns_total`, `tools_called_total`, `loop_detections_total`, `routing_fallbacks_total`, `cost_accumulated_usd`.
5. Add a `/health` endpoint that returns JSON with daemon status + last N error summaries.

**Estimated effort:** 2–3 sessions.

---

### Phase 34: Tool Execution Interruptibility

**Problem:** `InterruptSignal` is checked between iterations in the pipeline loop, but not during tool execution. If a user presses Ctrl+C while the agent is running a 30-second `Execute` command or a slow web fetch, they have to wait for the tool to finish before the interrupt takes effect.

**Proposal:**
1. Pass `ctx context.Context` to `ToolHandler` (currently it's `func(args map[string]interface{}) (string, bool)` — change signature to include `ctx`).
2. Tools that support cancellation (terminal commands, HTTP fetches) check `ctx.Done()` periodically.
3. Wire the `InterruptSignal` to cancel the context passed to tool dispatch.
4. Tools that can't be interrupted (e.g., atomic file writes) complete normally; the interrupt still stops the next iteration.

**Estimated effort:** 2 sessions. Touches every tool handler, so needs careful rollout.

---

### Phase 35: Evaluation Framework

**Problem:** There's no systematic way to measure whether changes to routing, loop detection, or tool definitions improve agent behavior. Every change is tested manually or with narrow unit tests.

**Proposal:**
1. Create an `eval/` package with:
   - **Scenario runner**: loads a YAML/JSON file describing a conversation (user messages, expected tool calls, expected outcomes).
   - **Judgment model**: a separate LLM call that scores the agent's response against expected behavior.
   - **Regression suite**: a set of scenarios that must pass before merging.
2. Key scenarios to cover:
   - Simple Q&A (no tools needed)
   - Tool-use chains (search → read → execute)
   - Loop detection (intentional loops that should YIELD)
   - Routing decisions (cheap model for simple, premium for complex)
   - Context overflow (long conversations that should compact gracefully)
3. Run in CI with a `make eval` target. Store results as JSON for trend analysis.

**Estimated effort:** 3–4 sessions. High leverage — pays for itself as complexity grows.

---

### Phase 36: SQLite Hardening & Migration Readiness

**Problem:** The current SQLite setup works for single-user but has no explicit WAL mode, busy timeout, or connection pooling configuration visible. When multi-user auth arrives (Phase 28), SQLite's single-writer limitation will become a bottleneck. Planning ahead avoids a rushed migration later.

**Proposal:**
1. Enable WAL mode and set `busy_timeout=5000` in the daemon's store initialization.
2. Add connection pooling with `SetMaxOpenConns(1)` (SQLite serializes writes) but allow multiple readers.
3. Document the scaling ceiling: ~100 concurrent read QPS, ~20 write QPS with WAL.
4. Design the store interface such that a Postgres backend could be swapped in behind the same `store.Store` interface. The gRPC handlers already talk through this — verify the abstraction is clean.
5. Add a `store.Metrics()` method that returns: `db_size_bytes`, `wal_size_bytes`, `active_connections`, `query_count`.

**Estimated effort:** 2 sessions.

---

### Phase 37: Decorator — Composable Provider Wrappers

**Pattern:** **Decorator (GoF)** — Wraps the Provider interface with cross-cutting behaviors (retry, caching, metrics) without modifying existing providers.

**Problem:** The `Provider` interface is `Prepare → Execute/Stream → Parse`, but cross-cutting concerns are handled ad-hoc. The streaming caller has manual fallback logic (`callNonStreaming` in streaming_caller.go). The router has manual retry/demotion. Prompt caching headers would need to be injected into every provider. Each of these is a Decorator waiting to be composed.

**Current state (ad-hoc cross-cutting in streaming_caller.go):**
```go
// streaming_caller.go — fallback logic is hardcoded in the caller
func (c *streamingCaller) callNonStreaming(...) {
    // manual fallback: try primary, then fallback chain
    if p.Name() != c.agent.Registry.Primary() {
        if primary, err := c.agent.Registry.GetPrimary(); err == nil {
            // ...
        }
    }
}
```

**Decorator refactor:**
```go
type RetryProvider struct {
    inner    Provider
    maxRetry int
    backoff  time.Duration
}

func (r *RetryProvider) Execute(ctx context.Context, req interface{}) (ProviderResult, error) {
    for attempt := 0; attempt <= r.maxRetry; attempt++ {
        result, err := r.inner.Execute(ctx, req)
        if err == nil && result.Error == nil {
            return result, nil
        }
        time.Sleep(r.backoff * time.Duration(1<<attempt))
    }
    return r.inner.Execute(ctx, req) // last attempt, let caller handle error
}

// Compose: metrics → cache → retry → actual provider
p := NewMetricsProvider(
    NewCacheProvider(
        NewRetryProvider(openaiProvider, 3, 2*time.Second),
        cache,
    ),
    metrics,
)
```

**Proposal:**
1. Define standard Decorators:
   - `RetryProvider` — retries with exponential backoff
   - `CacheProvider` — caches `Execute` results by request hash (read-only calls)
   - `MetricsProvider` — records latency, token usage, error rates per provider
   - `FallbackProvider` — delegates to a chain of providers on failure
2. Wire in `setup.go` Providers() function — compose decorators based on config.
3. Remove ad-hoc fallback logic from `streamingCaller` — the Decorator chain handles it.
4. Phase 32 (Prompt Caching) becomes a `CacheHeadersProvider` decorator.

**Estimated effort:** 2–3 sessions.

---

### Phase 38: State — TUI Phase State Machine

**Pattern:** **State (GoF)** — Replaces the current `phase` enum + switch-based dispatch with polymorphic state objects, each encapsulating its own input handling and rendering.

**Problem:** The TUI model (`tea_model.go`) uses a `phase` enum with 5 values (`phaseReady`, `phaseThinking`, `phaseStreaming`, `phaseToolCall`, `phaseApproval`). The `Update()` method is a 475-line switch statement with phase checks scattered throughout:

```go
// Current: phase checks in Update()
if m.phase == phaseApproval {
    // approval logic
}
if m.phase == phaseReady || m.phase == phaseApproval {
    // input handling
}
```

The View() method has similar branching. Adding a new phase (e.g., `phaseError`, `phaseConfirmQuit`) means touching the switch in 4+ places. This is the classic State pattern smell: conditional logic dispersed across methods based on a state variable.

**State pattern refactor:**
```go
type TUIState interface {
    Update(msg tea.Msg, m *teaModel) (TUIState, tea.Cmd)
    View(m *teaModel) string
    Enter(m *teaModel) tea.Cmd  // called on transition into this state
    Exit(m *teaModel)            // called on transition out
}

type ReadyState struct{}
func (s *ReadyState) Update(msg tea.Msg, m *teaModel) (TUIState, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.Type {
        case tea.KeyEnter:
            return &ThinkingState{}, m.startAgentTurn(...)
        }
    }
    return s, nil
}

type ThinkingState struct{ spinner int }
func (s *ThinkingState) Update(msg tea.Msg, m *teaModel) (TUIState, tea.Cmd) {
    // only handles thinking-related messages
}
```

Each state object is small (~30–50 lines), testable in isolation, and adding a new state doesn't touch existing ones. The `teaModel` holds a `state TUIState` field and delegates `Update()`/`View()` to it.

**Proposal:**
1. Define `TUIState` interface with `Update`, `View`, `Enter`, `Exit`.
2. Create state objects: `ReadyState`, `ThinkingState`, `StreamingState`, `ToolCallState`, `ApprovalState`, `ErrorState`.
3. Move `teaModel` fields that belong to specific states into their state objects (e.g., `spinner` lives in `ThinkingState`).
4. `teaModel.Update()` delegates to `m.state.Update()`, which returns the next state.
5. Add `StateTransition` event to the EventBus for observability.

**Estimated effort:** 2 sessions. Clean refactor, no behavior change.

---

### Phase 39: Mediator — Explicit Pipeline Data Bus

**Pattern:** **Mediator (GoF)** — Introduces a `PipelineBus` that stages write results to and read results from, replacing implicit mutation of `ConversationContext`.

**Problem:** Pipeline stages communicate by mutating `ConversationContext` directly. Stage 4 appends tool results (`convCtx.AppendToolResults`). Stage 5 reads them. Stage 1 reads `ctx.HotBlockContent`. This implicit data flow means:
- You can't tell what a stage reads without reading its source
- You can't tell what a stage writes without reading its source
- Testing a stage in isolation requires constructing the full `ConversationContext` with the exact fields that stage will read, even if those fields aren't "its concern"
- Adding a new data dependency between stages is invisible — just access another field on `convCtx`

**Current state (implicit data flow):**
```go
// Stage 4 mutates convCtx:
convCtx.AppendToolResults(toolResults)

// Stage 5 reads convCtx (from Stage 4's mutation):
func (h *ProgressDetectionHandler) Handle(convCtx *ConversationContext, 
    toolResults []ToolResult, parsed ParsedResponse) { ... }
```

Note: Stage 5 receives `toolResults` as a parameter (explicit) but also reads `convCtx.ProgressHistory`, `convCtx.CompactedThisTurn`, `convCtx.Messages` (implicit). Which data comes from where is arbitrary.

**Mediator refactor:**
```go
type PipelineBus struct {
    PreparedRequest   PreparedRequest
    ParsedResponse    ParsedResponse
    ToolResults       []ToolResult
    StageAction       StageAction
    // No ConversationContext — stages read/write the bus, not the context
}

// Stage 1 writes:
func (s *RequestPrepStage) Process(ctx *ConversationContext, bus *PipelineBus) {
    bus.PreparedRequest = s.buildRequest(ctx)
}

// Stage 4 writes:
func (s *ToolDispatchStage) Process(ctx *ConversationContext, bus *PipelineBus) {
    bus.ToolResults = s.dispatch(bus.ParsedResponse.ToolCalls)
}

// Stage 5 reads from bus:
func (s *ContextManagementStage) Process(ctx *ConversationContext, bus *PipelineBus) {
    bus.StageAction = s.chain.Handle(ctx, bus.ToolResults, bus.ParsedResponse)
}
```

`ConversationContext` remains as the persistent state (messages, session ID), but stages communicate through the bus. Each stage's inputs and outputs are explicit in its function signature.

**Proposal:**
1. Define `PipelineBus` struct with fields for each inter-stage data dependency.
2. Refactor each stage's `Process()` to read from bus and write to bus.
3. `ConversationContext` becomes read-only during a single iteration (stages can read it but not write inter-stage data to it).
4. The loop skeleton (`RunPipelineLoop`) owns the bus, clears it each iteration.
5. Dramatically simplifies testing: mock a bus with known inputs, call one stage, assert bus outputs.

**Estimated effort:** 3 sessions. Touches every pipeline stage and the loop skeleton.

---

## Suggested Priority Order

| Priority | Phase | Rationale |
|----------|-------|-----------|
| 1 | **Phase 29** (Token Counting) | Correctness bug — affects context overflow, which is a hard error for users |
| 2 | **Phase 31** (Observer — Memory Extraction) | Reliability — current sync extraction fails silently; event bus already exists, just needs a subscriber |
| 3 | **Phase 25** (Saved Tools) | Core feature already spec'd out — major capability unlock |
| 4 | **Phase 37** (Decorator — Provider Wrappers) | Simplifies Phase 32 (caching becomes a decorator), removes ad-hoc fallback logic, enables metrics |
| 5 | **Phase 32** (Prompt Caching) | Cost — could cut LLM bills by 50%+; made simpler by Phase 37 decorator chain |
| 6 | **Phase 33** (Observability) | Debuggability — needed before the system grows more complex |
| 7 | **Phase 30** (Parallel Tool Dispatch) | Latency — makes multi-tool turns feel snappier |
| 8 | **Phase 38** (State — TUI State Machine) | Maintainability — 475-line Update() switch becomes 6 small, testable state objects |
| 9 | **Phase 34** (Tool Interruptibility) | UX polish — responsiveness under Ctrl+C |
| 10 | **Phase 39** (Mediator — Pipeline Data Bus) | Clarity — makes inter-stage data flow explicit; enables cleaner tests |
| 11 | **Phase 35** (Eval Framework) | Quality — pays for itself as change velocity increases |
| 12 | **Phase 36** (SQLite Hardening) | Infrastructure — enables multi-user; WAL mode, connection pooling, Postgres readiness |
| 13 | **Phase 28** (Namespace Isolation) | Multi-tenancy foundation — `owner` column scoping; blocked on Phase 36 for schema migration tooling |
| 14 | **Phase 27** (GitHub Integration) | Community — needed for open-source launch |
