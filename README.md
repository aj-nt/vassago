# Vassago — AI Agent Memory Daemon

[![Go Report Card](https://goreportcard.com/badge/github.com/aj-nt/vassago)](https://goreportcard.com/report/github.com/aj-nt/vassago)

**Named after the Goetic demon Vassago**, who "declares things past, present, and future, and discovers things hidden and lost." A shared memory daemon for AI agents — local-first, multi-agent, language-agnostic.

## What It Is

Vassago is a gRPC memory daemon written in Go. It gives AI agents durable shared memory: persistent storage, full-text search, real-time event subscriptions, session management, scheduled jobs, and autonomous memory extraction from conversations. All backed by SQLite with FTS5, no external dependencies at runtime.

## Quick Start

```bash
# Build
make build

# Run (defaults: unix socket + localhost:50051)
./bin/vassago-daemon

# Or with a config file
./bin/vassago-daemon --config config.yaml
```

## Architecture

```
┌──────────────┐    gRPC    ┌──────────────────────────────┐
│  AI Agents   │◄──────────►│       Vassago Daemon         │
│  (Go, Py,    │            │                              │
│   any lang)  │            │  ┌────────┐  ┌────────────┐  │
└──────────────┘            │  │ Store  │  │ Pub/Sub    │  │
                            │  │(SQLite)│  │   Hub      │  │
┌──────────────┐            │  └────────┘  └────────────┘  │
│ Other Vassago│◄──────────►│                              │
│  Instances   │  Replicat. │  ┌────────┐  ┌────────────┐  │
│  (peers)     │            │  │ Cron   │  │ Extraction │  │
└──────────────┘            │  │ Runner │  │  Pipeline  │  │
                            │  └────────┘  └────────────┘  │
                            └──────────────────────────────┘
```

## API Surface (42 RPCs)

| Category | RPCs |
|----------|------|
| **Memory CRUD** | AddMemory, GetMemory, UpdateMemory, RemoveMemory, ListMemories |
| **Search** | Search, SearchSessions, ListSessions |
| **Hot Block** | GetHotBlock (priority-ranked context injection) |
| **Sessions** | CreateSession, AddMessages, EndSession, GetSession |
| **Consolidation** | Consolidate, GetConsolidationCandidates |
| **Pub/Sub** | Subscribe (streaming — real-time memory change events) |
| **Agent Mgmt** | RegisterAgent, Heartbeat |
| **Chat** | ChatStream (bidirectional streaming for agent conversations) |
| **Todos** | AddTodo, ListTodos, CompleteTodo, RemoveTodo |
| **Skills** | AddSkill, GetSkill, ListSkills, RemoveSkill, SearchSkills |
| **Saved Tools** | AddSavedTool, GetSavedTool, ListSavedTools, SearchSavedTools, UpdateSavedTool, RemoveSavedTool |
| **Cron** | CreateCronJob, ListCronJobs, UpdateCronJob, DeleteCronJob |
| **Replication** | SyncChanges (peer delta sync) |

## Features

- **Durable Memory** — SQLite with WAL mode. Memories scoped by owner (namespace) with target/category/key indexing. Automatic expiry and idle cleanup.

- **FTS5 Full-Text Search** — Fast keyword search across memory content with Unicode tokenization.

- **Hot Block Computation** — On-demand, priority-ranked context injection for agent system prompts. Configurable per-target char limits, minimum priority filter, and observation TTL.

- **Agent Sessions** — Full session lifecycle with message storage. Sessions track agent, source platform, duration, and message count.

- **Real-Time Pub/Sub** — Agents subscribe to memory changes (target, category, key-prefix filters). Events stream over gRPC with drop-on-slow-subscriber backpressure.

- **Cron Jobs** — Durable scheduled jobs with cron expressions. Jobs fire on interval, publish events via pub/sub, and track last/next fire times. Auto-resumes across daemon restarts.

- **Autonomous Memory Extraction** — LLM-powered pipeline that extracts structured memories from conversation transcripts. Debounced scheduler prevents duplicate processing; hard-cap interval ensures eventual processing. No agent involvement needed.

- **Peer Replication** — Delta sync between Vassago instances. Pulls changes since last-synced timestamp per namespace. Last-write-wins conflict resolution with event publication so local agents pick up remote changes.

- **Multi-Platform Messaging** — Platform adapters for Discord, Telegram, iMessage (macOS only), and Email (IMAP/SMTP). Each adapter polls or listens via webhooks, processes inbound messages through the daemon, and sends responses back.

- **GitHub Webhooks** — Ingest issue and PR events as memory observations. Configurable webhook secret for payload verification.

- **gRPC Hardening** — Panic recovery interceptor, structured request logging, configurable API key auth, message size limits, and concurrent stream caps.

- **Multi-Transport** — Unix domain sockets, TCP with optional TLS, and HTTP (for webhooks). gRPC reflection enabled for tooling.

- **Config Merge** — Defaults with YAML overlay. No deep merge, no fallback chains — explicit and auditable.

## Internal Packages

| Package | Purpose | Tests |
|---------|---------|-------|
| `store` | SQLite CRUD, FTS5 search, hot blocks, sessions, consolidation, replication state | 80.9% coverage |
| `grpc` | Service handlers, interceptors, auth, namespace isolation | 82.3% coverage |
| `pubsub` | In-process event hub with subscription matching | 100% coverage |
| `cron` | Cron expression parsing, job scheduling, fire callback | 89.1% coverage |
| `replication` | Peer gRPC client, sync engine, delta application | ✅ |
| `daemon` | Wiring, config, listeners, lifecycle, shutdown | 54.5% coverage |
| `messaging` | Platform adapters (Discord, Telegram, iMessage, Email) | ✅ |
| `extraction` | LLM pipeline, debounced scheduler, memory formatting | ✅ |
| `github` | Webhook parsing (issues, PRs) | ✅ |
| `migration` | Hermes → Vassago data migration tooling | ✅ |

**Total: ~33K lines of Go, 664 tests, 15 packages, all green.**

## Configuration

```yaml
daemon:
  socket: "~/.vassago/vassago.sock"
  tcp: "localhost:50051"
  http: ":8080"
  api_key: ""         # optional

database:
  path: "~/.vassago/vassago.db"
  wal_mode: true
  busy_timeout: 5000

hot_block:
  memory_char_limit: 4000
  user_char_limit: 2000
  min_priority: 2
  observation_ttl_days: 30

cron:
  enabled: false
  interval: 60        # check interval in seconds
  max_jobs: 100

extraction:
  enabled: false
  model: "claude-sonnet-4-5"
  base_url: "https://api.anthropic.com/v1"
  api_key: ""
  debounce_interval: 30
  hard_cap_interval: 900
  max_messages_per_extraction: 50
  min_messages_for_extraction: 4

replication:
  enabled: false
  poll_interval_sec: 10
  peers:
    - id: "studio"
      addr: "studio.local:50051"
      namespaces: ["default"]

messaging:
  discord:
    token: ""
    guilds: []
    allowed_channels: []
  telegram:
    token: ""
    allowed_chats: []
  imessage:
    enabled: false
    allowed_recipients: []
  email:
    enabled: false
    # ... IMAP/SMTP credentials

github:
  enabled: false
  webhook_secret: ""
```

## Python SDK

```python
from vassago import VassagoClient

client = VassagoClient()
client.add_memory(target="memory", category="fact", key="python-version",
                  content="Python 3.12.2", priority=3)
results = client.search("python", target="memory")
```

## License

AGPLv3 — see [LICENSE-AGPLv3](LICENSE-AGPLv3).

The `vassago-sdk` (shared proto, client, and messaging packages) is dual-licensed under Apache 2.0.
