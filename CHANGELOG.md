# Changelog

All notable changes to Vassago.

## [Unreleased]

## [0.1.0] - 2026-05-21

First public release. Vassago is a shared memory daemon for AI agents — one daemon, many agents, zero echo loops.

### Core — Daemon

- **SQLite-backed memory store** with WAL mode, concurrent reads, serialized writes
- **Hot block** — per-agent priority-sorted context window for prompt injection. Supports category boost/suppress, char limits, last-session context
- **FTS5 full-text search** with OR semantics, operator preservation, and column filter sanitization
- **Skills** — shared procedural memory for reusable workflows
- **Saved tools** — user-defined scripts stored in the daemon
- **Todo management** — per-agent task tracking
- **Sessions & messages** — durable session lifecycle with message persistence
- **Consolidation** — auto-expiry of stale observations (startup sweep + periodic hourly). Scoped idle/expired/category deletion
- **Schema migrations** — versioned schema (v0→v9) with idempotent re-apply, applied sequentially on startup
- **Namespace isolation** — multi-tenant via owner column on all tables

### Real-time Events

- **Pub/sub hub** — in-process event bus with self-exclusion (agents never hear their own writes)
- **Telepathy** — agent A writes memory → agent B's hot block invalidates instantly
- **Cron runner** — built-in 5-field cron parser with step values, ranges, and comma-separated lists. Jobs fire via pub/sub events

### Platform Integration

- **Discord adapter** — bot with guild/channel allowlisting
- **Telegram adapter** — bot with chat allowlisting
- **iMessage adapter** — macOS-only, DB-polling with recipient allowlisting
- **Email adapter** — IMAP polling + SMTP sending with folder/filter support
- **GitHub webhook** — issue and PR events ingested as observation memories

### CLI

- **Unified binary** — `vassago init|start|stop|status|memory|hot-block|sessions|config|version`
- **Interactive first-run setup** — `vassago init` walks through configuration
- **Import from Hermes** — `vassago-import --source state.db` migrates Hermes state.db (memories, sessions, messages) with UTF-8 sanitization

### Client Libraries

- **Go client** — full gRPC client with reconnection, heartbeat, hot block caching (Apache 2.0)
- **Go SDK** — batteries-included connection lifecycle with MemoryStore interface
- **Python SDK** — async gRPC client with MemoryEntry, HotBlock, SessionInfo, TodoItem, Skill, CronJob types (Apache 2.0)

### Development

- **918 tests** — all passing with `-race` (race detector)
- **3 CI workflows** — test+vet+lint+vulncheck, QA gate (adds proto diff), release (goreleaser)
- **GoReleaser** — 4 binaries (darwin+linux, arm64+amd64), Homebrew formula
- **Docker** — single Dockerfile + docker-compose, starts daemon by default
- **Dual-licensed** — AGPLv3 core, Apache 2.0 protocol/client SDK
