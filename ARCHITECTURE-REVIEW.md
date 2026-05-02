# Mnemosyne Architectural Review

**Date:** 2026-05-01  
**Reviewer:** AJ + Hermes  
**Commit:** 8d478ed  
**Tests:** 449 passing with -race

---

## 1. Package Dependency Graph

### Structure
```
proto (leaf — no internal deps)
  ↑
client (depends on proto only)
  ↑
agent/tools, agent/scheduler, agent/interface (depend on client + proto)
  ↑
agent/context, agent/eventbus (utility packages, no upstream deps)
  ↑
agent/agent, agent/pipeline, agent/provider (core logic)
  ↑
agent/cmd/mnemosyne-agent (binary — ties everything together)

daemon/internal/store (leaf — SQLite only)
  ↑
daemon/internal/pubsub (depends on proto)
  ↑
daemon/internal/grpc (depends on store + pubsub + proto)
  ↑
daemon/internal/daemon (ties gRPC + store together)
  ↑
daemon/cmd/mnemosyne-daemon (binary)
```

### Findings

**CRITICAL: Circular dependency via agent ↔ interface**
`agent/interface` imports `agent` (for the Agent struct), while `agent` depends on `agent/interface` (for CLI). This creates a tight coupling that makes testing difficult.

**MEDIUM: agent/tools imports client directly**
The tools package imports `client` — this means any tool that needs the Mnemosyne daemon (memory tools, getHotBlock) has a hard dependency on the gRPC client. This makes unit testing tools in isolation require a mock client, which isn't formalized as an interface.

**INFO: 7 interfaces defined** — `Provider`, `AgentAPI`, `SubAgentRunner`, `HTTPClient`, `MnemosyneClient`, `MnemosyneServer`, `UnsafeMnemosyneServer`. The gap: the `client.Client` concrete type is used directly in `Agent` instead of through an interface, making it hard to mock for the hot block cache and other tests.

**INFO: No import cycles** — `go build ./...` produces no cycle warnings.

---

## 2. Code Organization & Responsibility

| Package | Lines | Responsibility | Assessment |
|---------|-------|---------------|------------|
| agent/ | 593 | Agent orchestrator, Run loop | **Too large** — RunStreaming is 155 lines |
| agent/cmd/ | 279 | CLI entry point, wiring | Good — thin main, config loading |
| agent/interface/ | 820 | CLI + TUI + compact.go | **Mixed** — TUI formatting and context management mixed |
| agent/pipeline/ | ~700 | 5-stage pipeline | Good — each stage is a separate file |
| agent/provider/ | ~770 | Provider registry + routing | Good — clear separation |
| agent/scheduler/ | ~150 | Periodic task runner | Good — small, focused |
| agent/tools/ | ~830 | Tool handlers + validation | Good — each tool is a file |
| client/ | 530 | gRPC client + reconnect + cache | Good — well-separated |
| daemon/internal/store/ | ~800 | SQLite CRUD | Good — clean data layer |
| daemon/internal/grpc/ | 601 | gRPC service | **Large** — service.go could be split |
| daemon/internal/pubsub/ | ~150 | Pub/sub hub | Good — small, focused |

### Findings

**HIGH: agent/interface mixes 3 concerns** — TUI formatting (tui.go), context compaction (compact.go), and CLI loop (cli.go). These should be separate packages or at least separate files with clear boundaries.

**MEDIUM: daemon/internal/grpc/service.go is 601 lines** — It handles all RPCs in one file. Should be split into separate files per RPC group (memory, session, subscription, agent).

**MEDIUM: agent/agent.go RunStreaming is 155 lines** — The streaming loop is complex with tool dispatch, context management, and provider selection all interleaved. Extract into smaller methods.

---

## 3. Interface Design & Testability

### Current Interfaces
| Interface | Purpose | Used For |
|-----------|---------|----------|
| `Provider` | LLM provider abstraction | Registry, fallback, routing |
| `AgentAPI` | Mock agent session for tests | client/agent_test.go |
| `SubAgentRunner` | Delegate tool mock | agent/tools/delegate_test.go |
| `HTTPClient` | Mock HTTP for web tools | agent/tools/web.go |

### Findings

**HIGH: client.Client used as concrete type everywhere**
`Agent.Mnemo` is `*memoclient.Client`, not an interface. This means:
- Can't mock the daemon client in Agent tests without creating a real gRPC connection
- Hot block cache tests require workarounds (the SetHotBlockCache pattern)
- Tool handlers create their own `context.Background()` contexts instead of using the agent's context

**Recommendation:** Extract a `MnemoClient` interface from `client.Client` that covers `GetHotBlock`, `AddMemory`, `SearchMemories`, etc. This is the single highest-impact change for testability.

**MEDIUM: ToolHandler is a function type, not an interface**
```go
type ToolHandler func(args map[string]interface{}) (string, bool)
```
This is fine for simple tools but makes it hard to add middleware (logging, metrics, retry) without wrapping each handler.

**INFO: Pipeline stages use concrete types, not interfaces**
Each stage is a concrete struct. This is fine for the current architecture but makes it harder to compose pipelines differently (e.g., for test mode vs production mode).

---

## 4. Error Handling

### Patterns Found

**GOOD: Errors are generally well-wrapped**
- Store errors use `fmt.Errorf("...: %w", err)` consistently
- gRPC errors return proper status codes
- Tool errors return structured `ValidationError`

**MEDIUM: log.Fatal only in main packages**
All `log.Fatalf` calls are in `main.go` and `mnemosyne-import/main.go` — acceptable for CLI entry points.

**MEDIUM: Some tool error messages are opaque**
```go
// memory.go line ~160
return "key and content are required", true
```
This doesn't tell the LLM which field is missing. The validation layer fixes this for schema-validated tools, but the direct error return bypasses it.

**LOW: No structured error types in the agent**
The agent uses plain `error` throughout. There's no `AgentError` type that wraps error codes for programmatic handling. This makes it hard for callers to distinguish "daemon unreachable" from "validation failed" from "LLM API error".

---

## 5. Configuration Architecture

### Current Design
```go
type Config struct {
    Agent          AgentSection
    Provider       ProviderSection
    HotBlock       HotBlockConfig
    Tools          ToolsSection
    Delegation     DelegationSection  // Phase 14C
    Scheduler      SchedulerSection   // Phase 14D
}
```

### Findings

**MEDIUM: Config is a flat struct, not validated**
`LoadConfig` reads YAML and applies defaults but doesn't validate the result. Invalid values (e.g., `max_turns: 0`) silently use defaults. This led to the Phase 14 features being disabled (delegation.enabled defaulted to false).

**Recommendation:** Add a `Validate() error` method to Config that checks:
- `agent.max_turns > 0`
- `provider.name` is non-empty
- `hot_block.memory_char_limit > 0`
- `delegation.max_depth >= 1`
- `scheduler.interval >= 60`

**LOW: Config is passed by value everywhere**
Config structs (200+ bytes) are copied on every function call. Should be passed by pointer.

---

## 6. Concurrency Model

### Goroutine Inventory
| Goroutine | Package | Controlled By | Leak Risk |
|-----------|---------|---------------|-----------|
| heartbeatLoop | client/agent.go:180 | context.WithCancel + `<-done` | ✅ Clean |
| subscription reader | client/agent.go:115 | context.WithCancel | ✅ Clean |
| scheduler tasks | agent/scheduler:28 | context.WithCancel per task | ✅ Clean |

### Mutex Usage
| Location | Type | Protecting | Assessment |
|----------|------|-----------|------------|
| `eventbus.Bus.mu` | RWMutex | Subscribers list | ✅ Good |
| `provider.Router.mu` | RWMutex | model map + costs | ✅ Good |
| `provider.Router.costMu` | Mutex | session cost tracking | ✅ Good |
| `scheduler.Scheduler.mu` | Mutex | task cancellers map | ✅ Good |
| `client.AgentSession.mu` | Mutex | registered, streamCancel | ✅ Good |
| `client.ReconnectClient.mu` | Mutex | connection state | ✅ Good |
| `store.Store.writeMu` | Mutex | serialized writes | ✅ Good (SQLite BUSY) |
| `agent.hotBlockCache.mu` | RWMutex | cached content | ✅ Good |
| `agent.hotBlockStale` | atomic int32 | cache stale flag | ✅ Good |
| `agent.context.ConversationContext.set` | atomic Bool | stream done flag | ✅ Good |

### Findings

**MEDIUM: SetHotBlockCache is not thread-safe**
`Agent.hotBlockCache` is set directly without synchronization. In practice it's called once at startup before `Run()`, so there's no data race. But it violates Go's memory model rules. The doc comment notes this precondition.

**Recommendation:** Change `hotBlockCache` to `atomic.Value` or add a mutex. Low-risk since it's set-once, but would make the API safe-by-design.

**MEDIUM: context.Background() in subscription goroutine**
`client/agent.go:92` creates `context.Background()` for the subscription goroutine, separate from the heartbeat context. If the process crashes without `Close()`, the subscription goroutine leaks until the gRPC stream dies. The `Close()` method does properly cancel both contexts.

**LOW: No graceful shutdown of scheduler on SIGINT**
The scheduler stops on `defer scheduler.Stop()` in main, but SIGINT handling is not shown. If the process is killed hard, scheduled tasks are abandoned mid-execution without completion.

---

## 7. gRPC API Design

### Current RPCs
| RPC | Request | Response | Assessment |
|-----|---------|----------|------------|
| AddMemory | AddMemoryRequest | AddMemoryResponse | ✅ Good |
| GetHotBlock | HotBlockRequest | HotBlockResponse | ✅ Good |
| SearchMemories | SearchRequest | SearchResponse | ✅ Good |
| Consolidate | ConsolidateRequest | ConsolidateResponse | ✅ Good |
| CreateSession | CreateSessionRequest | CreateSessionResponse | ✅ Good |
| RegisterAgent | RegisterAgentRequest | RegisterAgentResponse | ⚠️ No auth check |
| Heartbeat | HeartbeatRequest | HeartbeatResponse | ⚠️ No auth check |
| Subscribe | SubscribeRequest | stream UpdateEvent | ⚠️ No auth check |
| RemoveMemory | RemoveRequest | RemoveResponse | ✅ Good |
| ReplaceMemory | ReplaceRequest | ReplaceResponse | ✅ Good |
| ListMemories | ListRequest | ListResponse | ✅ Good |
| SearchSessions | SearchSessionRequest | SearchSessionResponse | ✅ Good |

### Findings

**HIGH: No authentication on RegisterAgent, Heartbeat, Subscribe**
The `mnemosyne-cli` tool currently has no auth, and the gRPC interceptor checks API keys. But the agent-to-daemon RPCs (RegisterAgent, Heartbeat, Subscribe) don't verify the agent's identity beyond checking it exists. Any client that can reach the gRPC port can subscribe to all events.

**Recommendation:** Add agent authentication to RegisterAgent (API key or token), and have Heartbeat/Subscribe verify the calling agent is registered.

**LOW: Missing RPCs for common operations**
- `DeleteSession` — no way to clean up sessions
- `GetSession` — no way to retrieve a specific session's messages
- `UpdateMemory` — no partial update (only ReplaceMemory which is upsert by key)

These are minor gaps that can be added later.

---

## 8. Test Coverage

### Coverage by Package (lowest to highest)
| Package | Coverage | Lines of Code | Assessment |
|---------|----------|---------------|------------|
| agent/cmd/mnemosyne-agent | 0% | ~279 | ❌ No tests for main wiring |
| daemon/cmd/mnemosyne-import | 0% | ~57 | ❌ One-time migration, acceptable |
| proto | 0% | ~2800 (generated) | ⚪ Generate-only, no tests needed |
| daemon/internal/daemon | 25.2% | ~249 | ⚠️ Low — daemon startup/shutdown |
| client | 37.0% | ~318 | ⚠️ Low — reconnection logic untested |
| agent | 38.1% | ~593 | ⚠️ Low — RunStreaming untested |
| agent/interface | 45.6% | ~820 | ⚠️ Low — TUI and context management |
| daemon/internal/migration | 42.6% | ~277 | ⚠️ Medium — Hermes import paths |
| daemon/internal/grpc | 46.8% | ~601 | ⚠️ Medium — RPC handlers |
| daemon/internal/pubsub | 58.8% | ~150 | ✅ Decent |
| agent/context | 62.2% | — | ✅ Good |
| agent/pipeline | 68.0% | — | ✅ Good |
| agent/scheduler | 74.0% | ~150 | ✅ Good |
| daemon/cmd/mnemosyne-cli | 86.5% | ~312 | ✅ Good |
| daemon/internal/store | 96.0% | ~800 | ✅ Excellent |

### Findings

**HIGH: agent/ and client/ packages have <40% coverage**
These are the core agent loop and client reconnection logic — the most critical runtime paths. A regression in streaming, reconnection, or tool dispatch would go undetected.

**Recommendation:** Priority areas for increased coverage:
1. `agent/agent.go` — RunStreaming (155 lines, 0% coverage)
2. `client/reconnect.go` — reconnection backoff logic
3. `daemon/internal/daemon` — daemon startup/shutdown

**MEDIUM: Integration tests only cover subscription path**
The daemon `integration_test.go` tests subscription flow but doesn't test:
- Agent registration + heartbeat lifecycle
- Multi-agent subscription interference
- Stress: 1000 rapid AddMemory operations
- gRPC auth enforcement

---

## Summary of Findings

### CRITICAL (0)
None found.

### HIGH (3)
1. **Circular dependency via agent ↔ agent/interface** — The `Agent` struct is imported by the interface package it's used from. This creates tight coupling and makes testing the full agent loop in isolation nearly impossible.

2. **No authentication on RegisterAgent/Heartbeat/Subscribe** — Any client that can reach the gRPC port can register as an agent and subscribe to all Telepathy events.

3. **Low test coverage on critical paths** — agent/ (38%) and client/ (37%) contain the runtime agent loop and reconnection logic but are undertested. RunStreaming has 0% coverage.

### MEDIUM (7)
1. **agent/interface mixes 3 concerns** — TUI, context compaction, and CLI loop should be separate packages.

2. **daemon/internal/grpc/service.go is 601 lines** — Split per RPC group.

3. **agent/agent.go RunStreaming is 155 lines** — Extract into smaller methods.

4. **client.Client used as concrete type** — No `MnemoClient` interface makes unit testing tools in isolation require gRPC mock infrastructure.

5. **SetHotBlockCache not thread-safe** — Set-once-before-run pattern works but isn't safe-by-design. Use atomic.Value.

6. **Config is not validated** — Invalid config values silently use defaults, which caused Phase 14 features to be silently disabled.

7. **context.Background() in subscription goroutine** — Subscription uses a separate context from heartbeat. If Close() isn't called, goroutine leaks.

### LOW (5)
1. **ToolHandler is a function type** — Can't add middleware without wrapping.
2. **Config passed by value** — 200+ byte struct copied on every call.
3. **No graceful scheduler shutdown on SIGINT** — Tasks abandoned mid-execution on hard kill.
4. **Missing gRPC RPCs** — DeleteSession, GetSession, UpdateMemory.
5. **Opaque tool error messages** — Some tools return plain strings instead of structured errors.

### INFO (4)
1. **7 Go interfaces** for testability — Provider, AgentAPI, SubAgentRunner, HTTPClient, MnemosyneClient, MnemosyneServer.
2. **No import cycles** confirmed.
3. **No panics in production code**.
4. **log.Fatalf only in main packages**.

---

## 9. Phase 15 Updates (2026-05-01)

**Commits:** d645500, 8d478ed
**Tests:** 449 passing with -race (up from 414)

### 9.1 New Components

| Component | Package | Lines | Purpose |
|-----------|---------|-------|---------|
| MessagingAdapter | daemon/internal/messaging | ~200 | Platform-agnostic messaging interface (Connect, Send, Listen, ListChannels, Close) |
| DiscordAdapter | daemon/internal/messaging | ~300 | Discord implementation with 2000-char split, bot self-filter, channel filtering |
| MessagingService (gRPC) | daemon/internal/grpc | ~150 | SendMessage, ListChannels, SubscribeToMessages RPCs |
| SendMessage tool | agent/tools | ~80 | Agent tool to send messages via daemon, 10s timeout |
| isConsecutiveToolIterationLoop | agent/pipeline | ~35 | Cross-tool repetition loop detector |

### 9.2 Bug Fixes

**Cross-tool repetition loop (HIGH → FIXED)**
The `isRepetitiveToolLoop` detector only caught same-tool repetition (3+ consecutive calls to the same tool). Production agents were looping across *different* tools (FileSearch → Execute → FileRead → repeat), which never triggered the threshold. Added `isConsecutiveToolIterationLoop` which detects 5+ consecutive iterations of ANY tool calls with no text-only yield between them. A text-only assistant response resets both counters.

**Streaming error kills session (HIGH → FIXED)**
`RunStreaming` returned immediately on `StreamError`, ending the entire session. The non-streaming fallback path (`runTurnNonStreaming`) already had proper primary-fallback logic. Now: when a non-primary streaming provider fails, the agent falls back to the primary provider (non-streaming) for that turn. Only returns error if the primary also fails or if the failing provider IS the primary.

**Ctrl+C unresponsive during tool loops (MEDIUM → FIXED)**
The interrupt signal (`InterruptSignal`) was only checked at the top of each loop iteration. During a streaming HTTP call or tool execution, Ctrl+C had no effect. Two fixes:
1. Added `convCtx.Interrupt.IsSet()` check inside the streaming event loop, so Ctrl+C breaks out mid-stream.
2. CLI now tracks signal count per turn: first Ctrl+C sends interrupt (graceful stop between iterations), second Ctrl+C within the same turn calls `os.Exit(130)` for immediate force quit.

**Tool input validation (MEDIUM → FIXED)**
Added `ValidateArgs` and `ApplyDefaults` in the tool dispatch pipeline. Tools now validate required fields, types, enums, and min/max values against their JSON Schema *before* the handler runs. `ValidationError` provides structured error messages (Tool, Field, Issue). The `Execute` tool now supports context cancellation via `_context` injection.

### 9.3 Architecture Changes

**Messaging system (proto + adapter + gRPC)**
The messaging subsystem follows the adapter pattern established by the Provider interface:
- `messaging.proto`: SendMessage, ListChannels, SubscribeToMessages RPCs
- `MessagingAdapter`: platform-agnostic interface for any chat platform
- `DiscordAdapter`: first concrete implementation, session management, rate limiting
- Daemon wiring: `NewService()` takes `*MessagingService`, which takes `MessagingAdapter` + `MessagingDB`

**Model routing (smart strategy)**
The `selectSmart` heuristic routes turns based on content:
- Tool-result turns → primary model (needs good reasoning)
- Complex reasoning keywords → best model
- Short/simple prompts → cheapest model
- Everything else → primary

Router tracks per-model failure counts and auto-demotes after 1 failure (5-minute recovery). Session cost tracking via `TurnCost` records.

**Agent context compaction**
`compactMessagesForNextTurn` in cli.go strips old tool call/result pairs and truncates old assistant text to 300 chars / 5 lines. This prevents the model from re-reading verbose prior responses and regurgitating them. The Stage 5 `ContextManagementStage` also compacts when context exceeds token limits.

**Telepathy hot block cache refresh**
`Agent.hotBlockCache` + `hotBlockStale` (atomic int32) implement a stale-then-revalidate pattern. `InvalidateHotBlock()` is called by Telepathy event callbacks, causing the next `GetHotBlock()` to re-fetch from the daemon.

### 9.4 Updates to Original Findings

| # | Original Finding | Status |
|---|-----------------|--------|
| HIGH-1 | Circular dependency agent ↔ agent/interface | **Open** — not addressed |
| HIGH-2 | No auth on RegisterAgent/Heartbeat/Subscribe | **Open** — not addressed |
| HIGH-3 | Low test coverage on agent/ and client/ | **Improved** — agent/ from 38% to ~50% (5 new streaming/loop tests) |
| MED-1 | agent/interface mixes 3 concerns | **Worsened** — compact.go now added to the mix |
| MED-4 | client.Client used as concrete type | **Open** — same |
| MED-5 | SetHotBlockCache not thread-safe | **Unchanged** — doc'd precondition, still not atomic |
| MED-6 | Config not validated | **Partially addressed** — MessagingSection added but Validate() still doesn't check Phase 14/15 features |
| LOW-3 | No graceful scheduler shutdown on SIGINT | **Partially addressed** — Ctrl+C now propagates to agent loop, but scheduler.Stop() still only called on clean exit |
| LOW-5 | Opaque tool error messages | **Improved** — ValidationError provides structured Tool/Field/Issue |

### 9.5 New Findings

**MEDIUM: Streaming fallback bypasses interrupt checks**
When a streaming provider fails and the fallback `runTurnNonStreaming` executes, it doesn't check `InterruptSignal` between stages. If the user presses Ctrl+C during fallback, the agent won't respond until the fallback turn completes.

**MEDIUM: RunStreaming is now 200+ lines**
The addition of streaming error fallback logic has made RunStreaming even longer. It now handles: streaming accumulation, tool calls, interrupt checks, provider fallback, and cost tracking in one function. This should be extracted into smaller methods.

**LOW: Execute tool context injection is unused**
The `_context` and `_deadline` parameter injection in Execute is wired but not called from the dispatch stage, which passes `map[string]interface{}` args without any context. The infrastructure is there but the pipeline doesn't use it yet.

**LOW: MessagingDB not yet implemented**
The MessagingService gRPC handler references a `MessagingDB` for message persistence but it's not implemented yet. Messages flow through but aren't stored.

**MEDIUM: Per-tool call budgets enforce limits**
FileRead (3/turn), FileSearch (2/turn), Execute (2/turn) are now enforced at the ToolDispatchStage level. Budget-exceeded results return a helpful error message telling the agent to use results it already has. This prevents the model from re-reading the same file 10 times.

**MEDIUM: Stuck detection breaks failed-search loops**
If the last 2 tool results are both errors or both contain "not found", the ContextManagementStage yields with a message: "I wasn't able to find what I was looking for." This prevents the common pattern of FileSearch(fail) → FileSearch(fail) → Execute(error) loops.

**MEDIUM: Aggressive compaction after iteration 5**
When iteration >= 5, `compactContext` Phase 0 strips tool results to summaries: `[tool result: FileRead (4523 chars)]`. Forces the model to rely on accumulated understanding rather than re-reading detailed outputs.

**MEDIUM: Tool result size limits prevent context blowup**
- FileRead: max 8000 chars (preserves beginning and end)
- FileSearch: max 4000 chars
- Execute: max 4000 chars
- ContextManagementStage tool results: max 50 lines / 4000 chars
Oversized results are truncated with a summary line showing total size.

**LOW: Smart routing now detects technical prompts**
`isTechnicalPrompt()` detects file paths, function calls, and code keywords in short prompts. Queries like "fix the bug in context_management.go:30" are now routed to the primary model instead of the cheap model.

**LOW: System prompt restructured into 4 sections**
The system prompt is now: Role + Personality → Tool Usage Guide → Anti-loop Rules → Hot Block. Memory instructions were consolidated into 1 proactive-memory rule. Tool usage guide tells the model when to use each tool and not to re-call tools it already has results from.

**LOW: Execute cwd parameter and path validation**
Execute tool now supports an optional `cwd` parameter with `~` expansion and allowed_dirs validation. Allows the agent to run commands in the repo directory.

**LOW: `~` expansion in allowed_dirs**
SetFileConfig now expands `~/...` paths in allowed_dirs to the user's home directory, so the agent can access `~/Documents/repos` properly.