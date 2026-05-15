# Mnemosyne — Shared Memory Daemon for AI Agents

> *In Greek mythology, Mnemosyne is the goddess of memory and the mother of the Muses.
> She is the source of all knowledge recalled — every insight, preference, and lesson
> that persists across the fleeting lives of those she watches over.*

## 1. Problem Statement

Hermes is a capable and feature-rich agent framework that we've used extensively.
Our experience with it taught us what a memory subsystem needs to do — and where
the architecture reaches its limits under new requirements it wasn't designed for.

Mnemosyne exists because we need capabilities that don't fit cleanly into Hermes'
existing architecture:

1. **Multi-agent memory sharing** — Hermes assumes a single agent owns the
   database. We need multiple agents to read and write the same memories and
   be aware of each other's changes in real-time.

2. **Hot block that adapts** — Hermes computes the memory context once at
   startup and freezes it for the session. We need it to update when other
   agents write, and to be configurable per-agent (a dev agent and a personal
   agent need different working context from the same data).

3. **Observation lifecycle** — Hermes sets expiry timestamps on observation
   entries but never enforces them. Over time, stale observations accumulate
   and consume space in the hot block.

4. **Configuration clarity** — Hermes has three places that define memory
   limits (config defaults, runtime fallbacks, tool constants) with different
   values, and a deep-merge that re-applies removed defaults. The system works,
   but the configuration path is hard to reason about when debugging.

5. **Memory budget pressure** — High-priority metadata about the memory system
   itself can consume most of the hot block budget, leaving less room for the
   actual content the agent needs in its working context.

These are growth constraints, not bugs. Hermes reliably serves a single agent.
Mnemosyne addresses the next scale: multiple agents, shared memory, and
per-agent context — requirements that call for a different architectural model.

## 2. Design Principles

| Principle | Decision |
|---|---|
| Greenfield | New project, new repo. Hermes' memory code is reference, not origin. |
| Language | **Go** — static typing, goroutines for concurrency, single binary, no runtime deps |
| Persistence | **SQLite** — proven, performant, serverless, concurrent-read capable |
| Concurrency | Multiple agents, one database — daemon serializes writes |
| Protocol | **gRPC** between agents and daemon (HTTP/gRPC gateway later) |
| Hot block | Computed **per-agent** — different agents have different working contexts |
| Schema | Absolutely new — old schema is example only |
| Deployment | Local machine, LAN, internet — all three |

## 3. Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Agent A    │     │  Agent B    │     │  Agent C    │
│  (local)    │     │  (LAN)      │     │  (internet) │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │ gRPC              │ gRPC              │ gRPC
       │                   │                   │
       └───────────────────┼───────────────────┘
                           │
                    ┌──────▼──────┐
                    │  Mnemosyne  │
                    │   Daemon    │
                    │             │
                    │  ┌───────┐  │
                    │  │SQLite │  │
                    │  └───────┘  │
                    │             │
                    │  ┌───────┐  │
                    │  │PubSub │  │
                    │  └───────┘  │
                    └─────────────┘
```

### 3.1 Daemon (mnemosyne-daemon)

A single long-running process that owns the SQLite database. It:

- Listens for gRPC connections (Unix socket for local, TCP for LAN/internet)
- Serializes all writes through a single goroutine (SQLite WAL mode allows
  concurrent reads)
- Publishes change events to subscribers (pub/sub for hot block invalidation)
- Computes hot blocks per-agent on request
- Manages schema migrations internally

### 3.2 Agent Client Library (mnemosyne-client)

A Go library (also with C bindings for potential Python/C# consumption) that:

- Connects to the daemon via gRPC
- Maintains a local hot block cache
- Subscribes to change events that affect its hot block
- Provides the memory tool interface (add, search, replace, remove, consolidate)

### 3.3 Telepathy

When Agent A writes a memory, the daemon publishes a change event. Any agent
subscribed to that memory scope receives the event and can invalidate/update its
hot block. This gives agents shared awareness — *telepathy* — without polling.

## 4. Schema Design

### 4.1 Core Tables

```sql
-- Schema versioning — one row, always present
CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Memories — the core data
CREATE TABLE memories (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    target      TEXT    NOT NULL,   -- 'memory' | 'user'
    category    TEXT    NOT NULL,   -- 'user' | 'environment' | 'quirk' | 'project' | 'observation'
    key         TEXT    NOT NULL,
    content     TEXT    NOT NULL,
    priority    INTEGER NOT NULL DEFAULT 3 CHECK(priority BETWEEN 1 AND 5),
    created_at  INTEGER NOT NULL,  -- Unix epoch seconds
    updated_at  INTEGER NOT NULL,
    last_accessed INTEGER NOT NULL,
    access_count  INTEGER NOT NULL DEFAULT 0,
    expires_at  INTEGER,            -- NULL = never expires; observation entries: now + 30 days
    source_agent TEXT NOT NULL,     -- Which agent wrote this
    source_session TEXT,            -- Session that wrote this
    UNIQUE(target, key)
);

-- FTS5 index for full-text search
CREATE VIRTUAL TABLE memories_fts USING fts5(
    content,
    content='memories',
    content_rowid='id'
);

-- Sessions — record of conversations
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    source      TEXT NOT NULL,      -- 'cli' | 'telegram' | 'discord' | 'cron' | 'tool'
    user_id     TEXT,
    model       TEXT,
    title       TEXT,
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER,
    message_count  INTEGER DEFAULT 0,
    tool_call_count INTEGER DEFAULT 0,
    input_tokens   INTEGER DEFAULT 0,
    output_tokens  INTEGER DEFAULT 0
);

-- Messages — record of conversation content
CREATE TABLE messages (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    role        TEXT NOT NULL,      -- 'user' | 'assistant' | 'tool'
    content     TEXT,
    timestamp   INTEGER NOT NULL,
    token_count INTEGER
);

-- Messages FTS
CREATE VIRTUAL TABLE messages_fts USING fts5(
    content,
    content='messages',
    content_rowid='id'
);

-- Agent registrations — who is connected
CREATE TABLE agents (
    id          TEXT PRIMARY KEY,   -- Agent-assigned unique ID
    name        TEXT,               -- Human-readable name
    hostname    TEXT,
    connected_at INTEGER NOT NULL,
    last_heartbeat INTEGER NOT NULL,
    config      TEXT                -- JSON blob of agent-specific config
);

-- Subscriptions — what each agent is watching (for pub/sub)
CREATE TABLE subscriptions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id    TEXT NOT NULL REFERENCES agents(id),
    target      TEXT,               -- NULL = all targets, or 'memory' | 'user'
    category    TEXT,               -- NULL = all categories
    key_prefix  TEXT,               -- NULL = all keys, or prefix match
    created_at  INTEGER NOT NULL
);
```

### 4.2 Indexes

```sql
CREATE INDEX idx_memories_target_cat ON memories(target, category);
CREATE INDEX idx_memories_priority ON memories(priority DESC);
CREATE INDEX idx_memories_last_accessed ON memories(last_accessed DESC);
CREATE INDEX idx_memories_expires ON memories(expires_at) WHERE expires_at IS NOT NULL);
CREATE INDEX idx_memories_source_agent ON memories(source_agent);

CREATE INDEX idx_sessions_source ON sessions(source);
CREATE INDEX idx_sessions_started ON sessions(started_at DESC);

CREATE INDEX idx_messages_session ON messages(session_id, timestamp);
```

### 4.3 Design Decisions vs Old Schema

| Aspect | Old (Hermes) | New (Mnemosyne) |
|---|---|---|
| Timestamps | `REAL` (float) | `INTEGER` (Unix seconds) — no floating point ambiguity |
| Agent tracking | None | `source_agent` on memories, `agents` table |
| Subscriptions | None | `subscriptions` table + gRPC streaming |
| Session schema | 21 columns incl. billing | 11 columns — data, not accounting |
| Messages | 13+ columns incl. reasoning | 7 columns — content is searchable, rest is metadata |
| Observation expiry | `expires_at` set but never checked | Daemon enforces on startup + periodic sweep |
| Config limits | 3 conflicting sources | Single source: daemon config |
| FTS wildcard | `*` query returns 0, no workaround | `ListMemories` RPC for "show everything" |

## 5. Hot Block Architecture

The hot block is the compressed context injected into an agent's system prompt.
It's the working set — what the agent "knows" before you even say hello.

### 5.1 Per-Agent Computation

Each agent can have different hot block parameters:

```protobuf
message HotBlockConfig {
    int32 memory_char_limit = 1;   // default: 4000
    int32 user_char_limit = 2;     // default: 2000
    int32 min_priority = 3;        // default: 2
    repeated string boost_categories = 4;  // categories to prioritize
    repeated string suppress_categories = 5; // categories to deprioritize
}
```

An agent working on a dev project might boost `project` and `environment`
categories. An agent doing personal assistant work might boost `user` and
`observation`. Same database, different view.

### 5.2 Rendering

```
[memories]
● Environment: dev-server (192.168.1.100) — always-on daemon host. Ollama on localhost.
● Project: Mnemosyne rewrite — Go daemon, gRPC, SQLite, pub/sub for telepathy.
● Project: Dogfood fork independent from upstream. 561 tests green.
● Quirk: FTS5 doesn't support * as wildcard. Use real search terms.

[user profile]
● Name: AJ. Prefers Go, codes in Python for years.
● AJ values being considerate to open source maintainers.
● AJ is pragmatic about upstream — dogfood fork is real production.
```

### 5.3 Invalidation via Pub/Sub

1. Agent A connects, requests hot block, subscribes to changes.
2. Agent B writes a memory.
3. Daemon publishes change event to all subscribers whose hot block
   parameters could be affected (same target, overlapping categories).
4. Agent A receives event, requests refreshed hot block.
5. Agent A now has Agent B's memory in its context — **telepathy**.

### 5.4 Budget Enforcement

The daemon computes the hot block by:

1. SELECT memories WHERE target= AND priority >= min_priority
2. ORDER BY priority DESC, last_accessed DESC
3. Accumulate content until char_limit reached
4. Truncate last entry with ellipsis if needed
5. Return rendered text block

This is identical in logic to the old `_build_hot_block()` but:
- Computed on demand, not frozen at init
- Per-agent parameters
- Automatically refreshed on change events

## 6. gRPC Service Definition

```protobuf
syntax = "proto3";

package mnemosyne;

service Mnemosyne {
    // --- Memory CRUD ---
    rpc AddMemory(AddMemoryRequest) returns (MemoryResponse);
    rpc ReplaceMemory(ReplaceMemoryRequest) returns (MemoryResponse);
    rpc RemoveMemory(RemoveMemoryRequest) returns (Empty);
    rpc GetMemory(GetMemoryRequest) returns (MemoryResponse);
    rpc SearchMemories(SearchRequest) returns (SearchResponse);
    rpc ListMemories(ListMemoriesRequest) returns (SearchResponse);

    // --- Hot Block ---
    rpc GetHotBlock(HotBlockRequest) returns (HotBlockResponse);
    rpc SubscribeHotBlock(HotBlockRequest) returns (stream HotBlockUpdate);

    // --- Consolidation ---
    rpc Consolidate(ConsolidateRequest) returns (ConsolidateResponse);

    // --- Session Lifecycle ---
    rpc CreateSession(CreateSessionRequest) returns (SessionResponse);
    rpc EndSession(EndSessionRequest) returns (Empty);
    rpc AddMessages(AddMessagesRequest) returns (Empty);
    rpc GetSession(GetSessionRequest) returns (SessionResponse);

    // --- Session Search ---
    rpc SearchSessions(SessionSearchRequest) returns (SessionSearchResponse);

    // --- Agent Management ---
    rpc RegisterAgent(RegisterAgentRequest) returns (RegisterAgentResponse);
    rpc Heartbeat(AgentId) returns (Empty);
}
```

### 6.1 Key Messages

```protobuf
message AddMemoryRequest {
    string target = 1;      // 'memory' | 'user'
    string category = 2;    // 'user' | 'environment' | 'quirk' | 'project' | 'observation'
    string key = 3;
    string content = 4;
    int32  priority = 5;    // 1-5, default 3
    string agent_id = 6;
    string session_id = 7;
}

message ReplaceMemoryRequest {
    // Match by (target, key) or by id
    string target = 1;
    string key = 2;
    string old_content = 3;  // Must match for optimistic concurrency
    string new_content = 4;
    int32  new_priority = 5;
    string agent_id = 6;
}

message HotBlockRequest {
    string agent_id = 1;
    HotBlockConfig config = 2;
}

message HotBlockUpdate {
    HotBlockResponse hot_block = 1;
    repeated int64 changed_memory_ids = 2;
    ChangeType change_type = 3;  // ADD | REPLACE | REMOVE | EXPIRE
}

message SearchRequest {
    string query = 1;
    string target = 2;      // optional filter: 'memory' | 'user'
    string category = 3;   // optional filter
    int32  limit = 4;       // default 20
}
```

## 7. Pub/Sub Design

### 7.1 Model

A **merge** model for hot block updates, not a cache-invalidation model. For
consolidation, there are two separate mechanisms (see §7.4).

**Hot block updates:**
- When a memory changes, subscribers receive the change event AND the new content
- The agent **merges** the update into its local hot block cache
- If the changed memory would appear in the subscriber's hot block (based on its
  priority, category, and budget), the subscriber rebuilds its hot block
- If the changed memory wouldn't make the cut (too low priority or won't fit),
  the subscriber can safely ignore it

This is more efficient than always re-requesting the full hot block, and more
correct than simple cache invalidation (which forces a round-trip even for
irrelevant changes).

### 7.2 Subscription Scoping

Agents can subscribe to specific scopes to reduce noise:

- All memories (target=null, category=null)
- Only user profile (target='user', category=null)
- Only project memories (target='memory', category='project')
- Only specific key prefix (key_prefix='kore-')

### 7.3 Event Flow

```
Agent B calls AddMemory("memory", "project", "mnemosyne-design", "...")
    ↓
Daemon writes to SQLite
    ↓
Daemon queries: which agents are subscribed to target=memory, category=project?
    ↓
Daemon pushes HotBlockUpdate to each matching subscriber
    ↓
Agent A receives update, merges into hot block cache
    ↓
If the new memory fits (priority >= min_priority, within char budget),
 rebuild hot block. Otherwise, ignore.
```

### 7.4 Consolidation: Auto vs Explicit

**Auto-consolidation** runs on the daemon as a background goroutine:
- Triggered on startup and on a configurable interval (default: 1 hour)
- Deletes expired observations: `DELETE FROM memories WHERE expires_at < now()`
- Deletes idle entries past their TTL (P1: 365 days, P2: 180 days, P3+: 60 days)
- Does NOT merge overlapping entries — too risky without LLM judgment

**Explicit consolidation** is triggered by the `Consolidate` RPC:
- The agent (or human via CLI) calls `Consolidate` with a scope
- The daemon returns a batch of candidate entries: overlapping keys, stale
  entries, near-duplicates
- The agent's LLM (or the human) decides what to merge, drop, or promote
- The agent calls `ReplaceMemory` or `RemoveMemory` for each decision

The `ConsolidateRequest` message should include:

```protobuf
message ConsolidateRequest {
    ConsolidateScope scope = 1;   // Which entries to consider
    int32 batch_size = 2;         // How many candidates to return (default: 10)
}

enum ConsolidateScope {
    ALL = 0;                  // All memories
    IDLE = 1;                 // Entries not accessed in N days
    OVERLAPPING = 2;          // Entries with similar keys or content
    OBSERVATIONS = 3;          // Observation entries only
    EXPIRED = 4;              // Entries past their expires_at
}

message ConsolidateResponse {
    repeated ConsolidateCandidate candidates = 1;
    int32 auto_deleted = 2;   // How many expired entries were auto-deleted
}

message ConsolidateCandidate {
    MemoryResponse memory = 1;
    string reason = 2;        // "idle_90d", "overlapping_key", "similar_content", "expired"
    float similarity = 3;     // 0-1 for overlapping candidates
}
```

## 8. Data Migration

### 8.1 Strategy

Support two paths:

1. **Fresh install** — empty database, start from scratch
2. **Import from Hermes** — point to existing `state.db`, read and transform

### 8.2 Import Process

```
mnemosyne import --source ~/.hermes/state.db
```

The import tool:
1. Opens the Hermes SQLite database (read-only)
2. Reads all memories, sessions, messages
3. Transforms schema (REAL timestamps → INTEGER, adds source_agent defaults)
4. Writes into Mnemosyne database
5. Validates row counts match
6. Rebuilds FTS5 indexes

### 8.3 No Running Migration

There is no in-place migration path. The Hermes database is treated as a
read-only source. Mnemosyne writes to its own database. Both can coexist
during evaluation.

## 9. Deployment Topology

### 9.1 Local Machine

- Daemon on Unix socket (`~/.mnemosyne/mnemosyne.sock`)
- Single agent on same machine
- Zero network overhead

### 9.2 LAN

- Daemon on TCP (`0.0.0.0:50051`)
- Multiple machines on same network connect via gRPC
- mDNS discovery optional (agents can also use explicit host:port)
- TLS optional on trusted networks

### 9.3 Internet

- Daemon behind reverse proxy / gRPC gateway
- TLS mandatory
- Authentication via API key or mTLS
- Rate limiting at gateway layer

### 9.4 Single Binary

Go compiles to a single static binary. No Python runtime, no venv, no pip.
Deploy by copying one file.

## 10. Development Process

### 10.1 Prototyping vs Production Code

Not everything needs tests first. The distinction matters:

| Mode | When | Testing | Check-in |
|---|---|---|---|
| Prototype | Exploring an API, testing an idea, first pass at wiring | Manual verification, ad-hoc | Do not check in |
| Production | Settled design, committed API, real logic | TDD — write test first | Only after QA pass |

A prototype proves something works. It lives in a branch or on scratch paper.
It does not go into `main`. Once the design is settled and the API won't change,
rewrite it as production code using TDD.

### 10.2 TDD for Production Code

Every production feature starts with a test. Write the test, watch it fail,
write the minimum code to pass, refactor. The Hermes memory system had 0 tests
when the bugs were found. Never again.

### 10.3 QA Gates

No code reaches `main` without passing two gates:

**QA Pass** — does it do what it says?
- All existing tests pass (`go test -race ./...`)
- New code has test coverage for the feature it adds
- Edge cases tested (empty input, max limits, concurrent access)
- No panics on malformed input
- gRPC returns proper status codes, not generic INTERNAL

**Red Team QA Pass** — what would break it?
- Try to crash it: malformed protobuf, oversized payloads, nil fields
- Try to corrupt it: concurrent writes to same key, write during schema migration
- Try to starve it: 10K memories, 50 concurrent agents, hot block with all P1 entries
- Try to lie to it: wrong agent_id, spoofed source_session, category injection
- Try to cheat expiry: set expires_at to 0, to max int64, to negative
- If it breaks, add a test, fix the code, re-run QA

### 10.4 Test Layers

| Layer | What | How |
|---|---|---|
| Unit | Schema ops, hot block computation, FTS5 query sanitization | Go `testing` package, in-memory SQLite |
| Integration | gRPC service end-to-end | Test server + client |
| Concurrency | Multiple agents writing simultaneously | goroutines + race detector (`-race`) |
| Migration | Import from Hermes state.db | Fixture: copy of real database |
| Performance | Hot block compute under 1K memories | Benchmarks (`_test.go` B funcs) |

### 10.5 Go Race Detector

`go test -race` is mandatory in CI. The single-writer, multi-reader model with
goroutines is exactly the kind of code that hides data races.

## 11. Project Structure

Mnemosyne is a daemon-only repository. Agent binaries live in separate repos
and connect to the daemon via gRPC.

```
mnemosyne/
├── cmd/mnemosyne/                # Unified CLI
│   ├── main.go                   # Entry point
│   ├── cmd_init.go               # First-run setup (dirs, config, launchd)
│   ├── cmd_start.go              # Start daemon + health check
│   ├── cmd_status.go             # Daemon health + database info
│   ├── cmd_memory.go             # Memory CRUD + search
│   ├── cmd_sessions.go           # Session listing + inspection
│   ├── cmd_config.go             # Config validation + display
│   ├── cmd_validate.go           # Deep config validation
│   ├── cmd_version.go            # Build info with commit hash
│   └── cmd_export.go             # Export memories to JSON
│
├── daemon/
│   ├── cmd/
│   │   ├── mnemosyne-daemon/     # Daemon binary entry point
│   │   ├── mnemosyne-cli/        # Admin/debug CLI client
│   │   └── mnemosyne-import/     # Hermes state.db import tool
│   └── internal/
│       ├── daemon/               # Core daemon: config, listeners, lifecycle
│       ├── store/                 # SQLite data access layer
│       │   ├── store.go          # Store initialization, schema, migrations
│       │   ├── add_memory.go     # Atomic upsert with validation
│       │   ├── hotblock.go       # Per-agent hot block computation
│       │   ├── sessions.go       # Session + message CRUD
│       │   ├── search.go         # FTS5 search with sanitization
│       │   ├── consolidate.go    # Expiry sweeps, idle cleanup
│       │   ├── schema.go         # Schema v9 + 35 migration statements
│       │   ├── skills.go         # Skills CRUD + FTS5
│       │   ├── saved_tools.go    # Agent-defined scripts
│       │   ├── cron.go           # Cron job storage
│       │   ├── todo.go           # Todo items
│       │   └── export.go         # Memory export to JSON
│       ├── grpc/                  # gRPC service implementation
│       │   ├── service.go        # Service struct + conversion helpers
│       │   ├── service_memory.go # Memory CRUD handlers
│       │   ├── service_hotblock.go
│       │   ├── service_session.go
│       │   ├── service_search.go
│       │   ├── service_agent.go
│       │   ├── service_pubsub.go
│       │   ├── service_consolidation.go
│       │   ├── service_health.go
│       │   └── interceptors.go  # Recovery + API key auth
│       ├── pubsub/               # In-process pub/sub hub
│       ├── cron/                 # 5-field cron expression runner
│       ├── messaging/            # Platform adapters (Discord, Telegram, iMessage, Email)
│       ├── extraction/           # LLM-powered memory extraction pipeline
│       ├── migration/            # Hermes state.db → Mnemosyne importer
│       └── github/               # GitHub webhook handler
│
├── client/                        # Public gRPC client (Apache 2.0)
│   ├── client.go                 # gRPC connection + health check
│   ├── mnemo_client.go           # MnemoClient interface (50+ methods)
│   ├── reconnect.go              # Auto-reconnection with backoff
│   ├── hotblock_cache.go         # Client-side hot block cache
│   ├── agent.go                  # Agent session lifecycle
│   └── null_client.go            # Null object pattern for testing
│
├── sdk/                           # Go SDK (batteries-included client)
│   ├── sdk.go                    # Connect(), MemoryStore, Config
│   ├── conversation.go           # Multi-turn conversation manager
│   ├── memstore.go               # MemoryStore interface
│   └── python/                   # Python SDK (async gRPC)
│       ├── pyproject.toml
│       └── src/mnemosyne/
│           └── client.py         # MnemosyneClient (async context manager)
│
├── proto/                         # gRPC protocol definitions (Apache 2.0)
│   ├── mnemosyne.proto           # Mnemosyne + AgentChat services
│   └── messaging.proto           # Messaging service
│
├── messaging/format/              # Shared message formatting (Apache 2.0)
│   └── format.go                 # Platform limits, chunking, context labels
│
├── configs/                       # Example configs
│   ├── mnemosyne.yaml            # Daemon configuration
│   └── com.mnemosyne.daemon.plist # macOS LaunchAgent plist
│
├── scripts/                       # CI and migration helpers
│
├── CHANGELOG.md
├── CONTRIBUTING.md
├── DESIGN.md                      # This document
├── README.md
├── Makefile
├── go.mod
├── Dockerfile
└── docker-compose.yml
```

## 12. Configuration

One file. One source of truth. No deep merge. No fallback chains.

```yaml
# mnemosyne.yaml
daemon:
  socket: ~/.mnemosyne/mnemosyne.sock   # Unix socket (local)
  tcp: localhost:50051                   # TCP listener (LAN/internet)
  tls:
    cert: ""                             # Empty = no TLS
    key: ""

database:
  path: ~/.mnemosyne/mnemosyne.db
  wal_mode: true                         # WAL for concurrent reads
  busy_timeout: 5000                     # ms to wait on lock

hot_block:
  memory_char_limit: 4000                # ~1140 tokens at 3.5 chars/token
  user_char_limit: 2000                  # ~570 tokens at 3.5 chars/token
  min_priority: 2                        # Include priority >= 2
  observation_ttl_days: 30               # Auto-expire observations

consolidation:
  max_idle_days: 60                      # Evict memories not accessed in N days
  max_idle_priority_1: 365               # P1 entries: generous TTL
  max_idle_priority_2: 180               # P2 entries: moderate TTL

logging:
  level: info
  format: json
```

## 13. Security Considerations

1. **Local**: Unix socket permissions (`0600`) — only owner can connect
2. **LAN**: TLS optional, API key header required
3. **Internet**: TLS mandatory, mTLS or API key, rate limiting
4. **Database**: File permissions `0600`, owned by daemon process
5. **Agent writes are tagged**: Every memory records `source_agent` — who wrote it.
   All agents can read and write all memories. `source_agent` enables auditing
   and filtering ("what did the dev agent learn?") but does not restrict access.
   Future: ACL system for restricting write access per-agent if needed.

## 14. From Hermes — What We Learned, What We're Building Differently

| What Hermes Does | What We Learned | How Mnemosyne Approaches It |
|---|---|---|
| Three config sources for memory limits | Multiple sources makes debugging hard | One config file, one loader, one code constant |
| Deep merge re-applies removed defaults | Removing a config key should work | No deep merge. Absent keys use the code constant |
| Hot block computed once at startup | Agents work long sessions; context goes stale | On-demand computation + pub/sub invalidation |
| Single agent owns the database | Multiple agents need shared state | Multi-agent by design, pub/sub for telepathy |
| Priority system doesn't distinguish metadata from content | System metadata can starve useful content | Per-agent hot block config, category boost/suppress |
| Observations have expires_at but never checked | Stale data accumulates silently | Daemon enforces TTL on startup + periodic sweep |
| FTS5 `*` query returns zero results | Wildcard search should just work | `ListMemories` RPC for "show everything" |
| Schema migration can silently fail | ALTER errors should halt startup | Schema version checked; ALTER errors halt startup |
| No tests for memory subsystem | Untested code breeds bugs | TDD from day one |