# Mnemosyne Roadmap

## Development Process

These rules apply across all phases:

### Prototyping vs Production

- **Prototype code** proves an idea works. It lives on a branch, not `main`. No TDD required. Manual verification is fine.
- **Production code** is committed API, real logic. TDD — write test first. Must pass both QA gates before merge to `main`.

### QA Gates (Required Before Every Merge to `main`)

**1. QA Pass** — does it do what it says?
- All tests pass: `go test -race ./...`
- New code has test coverage for the feature
- Edge cases tested (empty input, max limits, concurrent access)
- No panics on malformed input
- gRPC returns proper status codes

**2. Red Team QA Pass** — what would break it?
- Crash it: bad payloads, nil fields, oversized input
- Corrupt it: concurrent writes to same key, mid-migration writes
- Starve it: large data, many agents, full hot block budgets
- Lie to it: wrong agent IDs, spoofed sessions, category injection
- Cheat it: boundary timestamps, expiry gaming, priority overflow
- **If it breaks, add a test, fix the code, re-run both QA gates**

---

## Host Assignment

| Host | Role | Mnemosyne Daemon | Agent |
|---|---|---|---|
| QMM (AJ's personal M4 Mini) | Development, personal machine | No (connects to Studio) | CLI for ad-hoc dev |
| Studio (AJ's Mac Studio, always-on) | Testing + always-on daemon | Yes (Unix socket + TCP :50051) | CLI for testing, cron agents |

QMM is AJ's personal computer. Studio is the always-on test bed.

---

## Key Dependency Graph

```
Phase 1 (Daemon CRUD)
    ↓
Phase 2 (Agent Core) — needs daemon for GetHotBlock, AddMemory, SearchMemories
    ↓           ↘
Phase 3 (Telepathy)   Phase 4 (Search & Consolidation)
    ↓                    ↓
Phase 5 (Agent Tools) ← both
    ↓
Phase 6 (Data Migration) — can start once daemon is stable
    ↓
Phase 7 (Client Polish & Validation)
    ↓
Phase 8 (Hardening)
```

Phases 3 and 4 run in parallel — different code paths. Phase 6 can start
as soon as the daemon schema is stable (after Phase 1).

---

## Phase 1: Daemon Foundation (Week 1-2)

**Goal:** Bootable daemon with SQLite, gRPC, and basic CRUD. This is the
foundation everything else depends on.

### 1.1 Project Scaffold
- [ ] `go mod init github.com/aj-nt/mnemosyne`
- [ ] Directory structure (monorepo: daemon/ and agent/ subdirs)
- [ ] `proto/mnemosyne.proto` — full service definition (memory CRUD + hot block + session lifecycle)
- [ ] Generate Go code from proto (`buf generate` or `protoc`)
- [ ] `Makefile` — build, test, lint, generate targets
- [ ] CI: `go test -race ./...` on push

### 1.2 Store Layer
- [ ] `daemon/internal/store/schema.go` — CREATE TABLE statements, version tracking
- [ ] `daemon/internal/store/store.go` — Open/Close, WAL mode, busy timeout
- [ ] `daemon/internal/store/memories.go` — Add, Replace, Remove, Get, List
- [ ] `daemon/internal/store/search.go` — FTS5 query sanitization, search
- [ ] `daemon/internal/store/hotblock.go` — Compute hot block from parameters
- [ ] `daemon/internal/store/sessions.go` — CreateSession, AddMessages, GetSession
- [ ] Unit tests for every method (TDD: write test first)

### 1.3 gRPC Service
- [ ] `daemon/internal/grpc/service.go` — Implement Mnemosyne service
- [ ] `daemon/internal/grpc/session.go` — Session lifecycle RPCs
- [ ] `daemon/internal/daemon/listener.go` — Unix socket + TCP listeners
- [ ] `daemon/cmd/mnemosyne-daemon/main.go` — Start daemon, serve gRPC

**Deliverable:** `mnemosyne-daemon` starts, accepts gRPC connections, performs
CRUD on memories, computes hot blocks, manages sessions.

**QA Pass:** All tests green (`go test -race ./...`), edge cases covered, no panics on bad input.

**Red Team QA Pass:** Malformed gRPC requests return status codes, not panics. Concurrent writes don't deadlock. Empty DB returns empty hot block without error.

**Test:** Integration test: start daemon, connect client, AddMemory, GetHotBlock,
verify content. Race detector clean.

---

## Phase 2: Agent Core (Week 3-4)

**Goal:** Working CLI agent that talks to an LLM and uses Mnemosyne for memory.
This is the first thing we can actually USE.

**Why moved up from Phase 4:** We need feedback early. A daemon without an agent
is a database without a client. Building the agent right after basic CRUD lets
us validate the entire system end-to-end while we build the fancier features.

### 2.1 Agent Scaffold
- [ ] `agent/cmd/mnemosyne-agent/main.go` — CLI entry point
- [ ] `agent/agent.go` — Orchestrator: drives Stage 1→2→3→4→5 loop (see AGENT.md §3.3)
- [ ] `agent/config.go` — Load agent config from YAML
- [ ] `agent/context/types.go` — PreparedRequest, ParsedResponse, ProviderResult, ToolDispatchResult, StageAction
- [ ] `agent/context/context.go` — ConversationContext, SessionState
- [ ] `agent/context/capabilities.go` — ProviderCapabilities

### 2.2 Client Library (basic)
- [ ] `client/client.go` — Connect, AddMemory, SearchMemories, ReplaceMemory, RemoveMemory, GetHotBlock, CreateSession, AddMessages
- [ ] `client/hotblock_cache.go` — Local cache (no subscription yet — that's Phase 3)

### 2.3 Pipeline Stages
- [ ] `agent/pipeline/request_prep.go` — Stage 1: Build system prompt, inject hot block, prepare API kwargs
- [ ] `agent/pipeline/provider_call.go` — Stage 2: Call LLM via ProviderRegistry, emit stream deltas
- [ ] `agent/pipeline/response_processing.go` — Stage 3: Parse into canonical ParsedResponse
- [ ] `agent/pipeline/tool_dispatch.go` — Stage 4: Route tool calls to handlers, collect results
- [ ] `agent/pipeline/context_management.go` — Stage 5: Append results, check limits, persist session

### 2.4 Provider (OpenAI-compatible only for now)
- [ ] `agent/provider/registry.go` — ProviderRegistry: name → Provider
- [ ] `agent/provider/provider.go` — Provider interface (Prepare, Execute, Parse, Capabilities)
- [ ] `agent/provider/openai.go` — OpenAI-compatible API (covers Ollama, OpenRouter)
- [ ] `agent/provider/mock.go` — MockProvider for testing (canned responses)

### 2.5 Memory Tools
- [ ] `agent/tools/registry.go` — ToolRegistry: name → ToolHandler
- [ ] `agent/tools/memory.go` — AddMemory, SearchMemories, ReplaceMemory, RemoveMemory, Consolidate
- [ ] Connects to daemon via client library (gRPC)

### 2.6 CLI Interface
- [ ] `agent/interface/cli.go` — stdin/stdout terminal interface
- [ ] Streaming output (token-by-token display via EventBus)
- [ ] Signal handling (Ctrl+C to interrupt)
- [ ] `agent/eventbus/bus.go` — EventBus for observability

**Deliverable:** `mnemosyne-agent chat "What do you know about me?"` works.
The agent reads hot block from daemon, calls LLM, executes memory tools, loops
until done. Basic but functional.

**QA Pass:** Agent can add a memory, search for it, and read it back. Hot block
injected into system prompt. Agent loops correctly (tool call → result → continue).
Mock provider tests pass — full loop tested without real LLM calls.

**Red Team QA Pass:** LLM returns malformed tool call JSON — agent handles gracefully. LLM never yields (always wants another tool) — max_turns stops it. Daemon not running — clear error, no hang.

**Open items to resolve before Phase 2 implementation:**
- Error handling: retries, fallback, daemon-down behavior (AGENT.md §3.6)
- Streaming: Ctrl+C, stale detection, timeout params (AGENT.md §3.7)

---

## Phase 3: Telepathy (Week 5-6)

**Goal:** Multiple agents share memory in real-time. Pub/sub working.

### 3.1 Pub/Sub Hub
- [ ] `daemon/internal/pubsub/hub.go` — In-process pub/sub hub
- [ ] `daemon/internal/pubsub/subscription.go` — Scope matching (target, category, key_prefix)
- [ ] `SubscribeHotBlock` gRPC stream — push updates to connected agents
- [ ] `client/hotblock_cache.go` — Update: merge incoming updates into hot block (replaces Phase 2's static cache)

### 3.2 Agent Registration
- [ ] `RegisterAgent` RPC — agent identifies itself, gets agent_id
- [ ] `Heartbeat` RPC — keep-alive, detect disconnected agents
- [ ] `agents` table — track connected agents
- [ ] Clean up subscriptions on disconnect
- [ ] Agent sends agent_id and role on connect; daemon records in agents table

### 3.3 Agent Hot Block Subscription
- [ ] Agent subscribes to hot block updates at session start
- [ ] Hot block refreshes mid-session when telepathy event arrives
- [ ] Unsubscribe on session end

**Deliverable:** Two agent processes connected to same daemon. Agent A writes a
memory. Agent B receives the update in its hot block stream within milliseconds.

**QA Pass:** Stream delivers correct updates. Heartbeat detects disconnect. Subscriptions cleaned up on agent exit.

**Red Team QA Pass:** Agent crashes mid-stream — daemon cleans up, other agents unaffected. 1000 rapid writes — no lost updates in stream. Spoofed agent_id on subscription — rejected.

**Test:** Integration: two goroutine-clients, one writes, one receives stream update.
Verify latency < 100ms. Race detector clean.

---

## Phase 4: Search & Consolidation (Week 5-6, parallel with Phase 3)

**Goal:** Full search, session search, and consolidation working. Different code
path from Phase 3 — can be developed in parallel.

### 4.1 Full-Text Search
- [ ] `daemon/internal/store/search.go` — FTS5 sanitization (learn from Hermes bugs)
- [ ] `SearchMemories` RPC — FTS5 + LIKE fallback
- [ ] `ListMemories` RPC — paginated list (replaces broken `*` query)
- [ ] `SearchSessions` RPC — search session history

### 4.2 Consolidation & Expiry
- [ ] `Consolidate` RPC — trigger eviction/consolidation with scope and batch_size
- [ ] `ConsolidateRequest` with `ConsolidateScope` enum (ALL, IDLE, OVERLAPPING, OBSERVATIONS, EXPIRED)
- [ ] Auto-consolidation: background goroutine, startup sweep + hourly interval
- [ ] Eviction rules: idle > N days by priority, observations with expired TTL
- [ ] Startup sweep: `DELETE FROM memories WHERE expires_at < now()`

**Deliverable:** Search works reliably. Stale memories get cleaned up. No more
observation entries piling up forever.

**QA Pass:** Fuzz FTS5 queries with special characters. Verify expiry on startup.
Verify periodic sweep removes stale entries.

**Red Team QA Pass:** Queries with `*`, `OR AND NOT NEAR` edge cases, colons, newlines, null bytes. Expiry with timestamps at 0, max int64, negative. Consolidation with all-priority-1 memory (no eviction).

---

## Phase 5: Agent Tools (Week 7-8)

**Goal:** Agent has the tools it needs to be useful beyond memory. Also adds
Anthropic provider support.

### 5.1 Anthropic Provider
- [ ] `agent/provider/anthropic.go` — Anthropic Messages API + prompt caching
- [ ] ProviderCapabilities dispatches on capabilities, not provider name
- [ ] Fallback chain: primary → fallback1 → fallback2 (config-driven)
- [ ] Error handling for retries, rate limits, daemon-down (resolves AGENT.md §3.6)

### 5.2 File Tools
- [ ] `agent/tools/file.go` — FileRead, FileWrite, FileSearch
- [ ] Read-only by default, write requires approval in config

### 5.3 Terminal Tool
- [ ] `agent/tools/terminal.go` — Execute shell commands
- [ ] Approval gate: dangerous commands require user confirmation

### 5.4 More Tools (Priority-Sorted)
- [ ] `agent/tools/browser.go` — Web browsing (if needed)
- [ ] `agent/tools/delegate.go` — Sub-agent delegation (future)
- [ ] Tool registry: pluggable, enable/disable per agent config

**Deliverable:** Agent can read/write files, run shell commands, and search
memories. Anthropic provider works. Fallback chain works.

**QA Pass:** File operations work. Terminal executes commands. Memory tools still work after adding new tools. Anthropic provider streams correctly with prompt caching.

**Red Team QA Pass:** File path traversal (../../etc/passwd) — blocked. Shell injection (rm -rf /) — approval gate catches it. Concurrent file writes — no corruption. Provider fallback: primary returns 429 → fallback chain tries next.

---

## Phase 6: Data Migration (Week 7, parallel with Phase 5)

**Goal:** Import Hermes database for evaluation. No data loss.

### 6.1 Hermes Import Tool
- [ ] `daemon/cmd/mnemosyne-import/main.go` — read Hermes state.db
- [ ] `daemon/internal/migration/hermes.go` — schema transformation
- [ ] Timestamp conversion: REAL → INTEGER (Unix seconds)
- [ ] Add default `source_agent` for imported memories (use "hermes-import")
- [ ] Session/message import
- [ ] Validation: compare row counts, spot-check content
- [ ] Rebuild FTS5 indexes after import

### 6.2 Validation
- [ ] Import AJ's real state.db (~34 memories, ~824 sessions)
- [ ] Compare hot block rendering character-by-character against Hermes
- [ ] Verify all memories searchable via Mnemosyne
- [ ] Verify session search works

**Deliverable:** `mnemosyne import --source ~/.hermes/state.db` produces a
Mnemosyne database with all Hermes memories intact.

**QA Pass:** Import AJ's real state.db. Verify 34 memories, 824 sessions present
and searchable. Zero data loss.

**Red Team QA Pass:** Import a corrupt state.db (missing table, wrong schema version, NULL in NOT NULL column). Verify graceful error, no partial import left behind. Import twice — second run rejected with clear message (or idempotent with no duplicates).

---

## Phase 7: Client Polish & Validation (Week 9)

**Goal:** Polished client library. Admin CLI. End-to-end validation against Hermes.

### 7.1 Client Package Polish
- [ ] `client/client.go` — Automatic reconnection on disconnect
- [ ] `client/hotblock_cache.go` — Graceful shutdown: flush pending writes
- [ ] Exponential backoff on connection failure

### 7.2 Admin CLI
- [ ] `daemon/cmd/mnemosyne-cli/main.go` — admin/debug CLI
- [ ] Commands: add, search, get, remove, hot-block, consolidate, stats, session
- [ ] `stats` — memory count, agent count, hot block size, uptime

### 7.3 End-to-End Validation
- [ ] Run Mnemosyne daemon alongside Hermes (different port/socket)
- [ ] Import all Hermes data
- [ ] Compare hot block output character-by-character
- [ ] Compare search results for 10 known queries
- [ ] Verify memory CRUD operations produce identical results

**Deliverable:** Mnemosyne is a validated replacement for Hermes' memory subsystem.

**QA Pass:** End-to-end: daemon + CLI client, add/search/hot-block workflow. All commands return expected output.

**Red Team QA Pass:** Client connects to wrong port — clear error. Daemon not running — client fails gracefully, doesn't hang. Network partition mid-request — timeout, not goroutine leak.

---

## Phase 8: Hardening & Polish (Week 10+)

**Goal:** Production-ready. Secure, observable, documented.

- [ ] TLS support for LAN/internet deployment
- [ ] API key authentication (shared secret in gRPC metadata)
- [ ] Rate limiting (token bucket per agent)
- [ ] Structured logging (JSON, levels, request IDs)
- [ ] Metrics endpoint (Prometheus): memory count, hot block size, latency histograms
- [ ] Graceful restart: SIGTERM → drain connections → exit
- [ ] Documentation: README, API reference, deployment guide
- [ ] launchd unit file for Studio (always-on daemon)

---

## Success Criteria

| Metric | Target |
|---|---|
| AddMemory latency (local) | < 5ms |
| GetHotBlock latency (local) | < 10ms |
| SearchMemories latency (local) | < 20ms |
| Telepathy latency (LAN) | < 100ms |
| Race detector | Clean on all tests |
| Data migration | Zero data loss from Hermes |
| Hot block correctness | Character-by-character match against Hermes |
| Observation expiry | Stale entries removed within 1 hour |
| Agent loop | Completes multi-tool conversations without panic |
| Mock provider tests | Full loop tested without real LLM calls |
| Provider fallback | 429 on primary → next provider within 2 seconds |

---

## Risk Register

| Risk | Mitigation |
|---|---|
| SQLite write contention under multi-agent load | WAL mode + busy_timeout. Goroutine serializes writes. Benchmark early. |
| gRPC stream reconnection complexity | Start with simple reconnect. Exponential backoff later. |
| Proto backward compatibility | Version the proto. Breaking changes get new major version. |
| Hot block rendering differs from Hermes | Phase 7: character-by-character diff test. |
| LLM provider API drift | Provider interface abstracts away differences. OpenAI-compatible covers most. |
| Agent loop infinite cycle | max_turns config. Hard stop. |
| Tool execution safety | Approval gates for dangerous tools (Terminal, FileWrite). |
| Anthropic API complexity | Deferred to Phase 5. Start with OpenAI-compatible (covers Ollama + OpenRouter). |
| Agent starts too late → no early feedback | Moved Agent Core to Phase 2. Usable end of Week 4. |
| Session lifecycle RPCs forgotten | Added to Phase 1 scaffold and schema. |
| Client library needed before Phase 7 | Basic client written in Phase 2 as part of agent. Phase 7 is polish. |