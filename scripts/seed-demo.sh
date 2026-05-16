#!/bin/bash
# Vassago Demo Seed — pre-loads the daemon with realistic, interesting memories
# Usage: ./scripts/seed-demo.sh [--addr localhost:50051]
#
# Generates ~40 memories across 6 categories that look like they accumulated
# naturally over weeks of development. Makes the hot block and search demos
# actually show something useful.

set -euo pipefail

ADDR="${1:-localhost:50051}"
CLI="vassago-cli --addr $ADDR"

banner() { echo ""; echo "  $*"; echo "  ─────────────────────────"; echo ""; }
add()  { $CLI add "$@" > /dev/null; }

echo "╭──────────────────────────────────────────────────╮"
echo "│  Vassago Demo Seed — Populating with memories   │"
echo "╰──────────────────────────────────────────────────╯"

# --------------------------------------------------
banner "🖥  Development Environment"
# --------------------------------------------------

add memory env go-install        "Go 1.25.0 installed at /opt/homebrew/bin/go via Homebrew"
add memory env python-install    "Python 3.12.2 at /opt/homebrew/bin/python3, managed by pyenv"
add memory env editor-path       "Neovim 0.11 at /opt/homebrew/bin/nvim with LazyVim config"
add memory env docker-path       "Docker Desktop 4.45 running on macOS 15, socket at ~/.docker/run/docker.sock"
add memory env node-install      "Node 22 LTS at /opt/homebrew/bin/node via nvm, npm 10.x"
add memory env rust-install      "Rust 1.86 at ~/.cargo/bin/rustc, installed via rustup"
add memory env postgres-local    "PostgreSQL 17 running on localhost:5432 via Homebrew services"
add memory env redis-local       "Redis 7.4 at localhost:6379 for caching and pub/sub testing"
add memory env db-browser        "TablePlus at /Applications/TablePlus.app for SQLite inspection"
add memory env git-config        "Git configured with name 'AJ', email via GitHub no-reply, signing SSH key at ~/.ssh/id_ed25519"

# --------------------------------------------------
banner "🧠  Architecture & Design Decisions"
# --------------------------------------------------

add memory arch sqlite-choice    "Chose SQLite over Postgres: zero-dependency install, no daemon to manage. WAL mode gives enough concurrency for 2-3 agents. FTS5 handles full-text search without Elasticsearch."
add memory arch grpc-choice      "gRPC over REST: bidirectional streaming for pub/sub and chat, protobuf schema as single source of truth, auto-generated clients for Go and Python"
add memory arch namespace-design "Owner field doubles as namespace — each owner gets isolated memory space. Multi-tenancy without separate databases. API keys map to owners for auth."
add memory arch hotblock-design  "Hot blocks are priority-ranked context injection: ORDER BY priority DESC, updated_at DESC, clamped to char limits. Agents get exactly what matters, nothing they don't need."
add memory arch replication-lww  "Peer replication uses last-write-wins: UpdatedAt comparison at the entry level. No vector clocks needed — memories are eventually consistent, not transactional."
add memory arch extraction-flow  "Memory extraction is debounced: 30s cooldown, 15min hard cap. Prevents hammering the LLM API when an agent is mid-conversation. Pipeline reads session messages, extracts {key, content, category, priority} JSON."

# --------------------------------------------------
banner "📦  Project Status & Milestones"
# --------------------------------------------------

add memory project vassago-status   "Vassago is a gRPC memory daemon for AI agents. 14 packages, 664 tests, all green. 42 RPCs including CRUD, FTS5 search, hot blocks, sessions, pub/sub, cron, extraction, replication, messaging."
add memory project sdk-version      "vassago-sdk at v0.0.3 — includes proto definitions, Go/Python clients, messaging adapters (Discord, Telegram, iMessage, Email), and format utilities."
add memory project phase-replication "Peer replication shipped May 2025: delta sync engine with Engine + Peer types, SyncChanges RPC, vassago_replication state tracking table. Last-write-wins conflict resolution."
add memory project phase-extraction  "Autonomous memory extraction shipped: LLM pipeline reads conversation transcripts, extracts structured memories. Debounced with 30s/15min intervals to protect API budget."
add memory project phase-refactor    "Adapter refactor complete: moved Discord/Telegram/Email/iMessage adapters from daemon into vassago-sdk. Daemon is now pure memory infrastructure — SDK owns communication."
add memory project show-hn-planned   "Show HN launch planned for May 2025. README updated with full feature map, architecture diagram, configuration reference. Terminal demo script in progress."
add memory project go-report-card    "Go Report Card: A+ rating. golangci-lint passes clean. CI runs license header checks, race detector, and full test suite."

# --------------------------------------------------
banner "🔧  Bugs Found & Fixed (War Stories)"
# --------------------------------------------------

add memory bug fts5-colon-crash     "FTS5 MATCH queries with colons crashed as column filter syntax. Fix: double-quote all query tokens, fall back to LIKE when FTS5 syntax can't handle it."
add memory bug hotblock-empty       "Hot blocks were 78% empty because HOT_MIN_PRIORITY was 4 but all migrated content had priority 2. Fix: lowered to 2. Char limits handle truncation naturally."
add memory bug context-overflow     "Agent hit context overflow infinite loop: RunStreaming kept retrying when LLM API rejected requests at 200K+ tokens. Fix: EffectiveMaxTokens() with 10% safety margin, consecutive error tracking."
add memory bug silent-agent         "GLM-5.1 produced tool calls with no text between them — 4 iterations of empty content → YIELD. User saw tool indicators then silence. Fix: inject explanatory status on loop-detected YIELD."
add memory bug session-truncation   "Session IDs truncated to 8 chars in ListSessions — UUID prefix collision made GetSession lookups fail. Fix: show full 36-char UUIDs."
add memory bug streaming-timeout    "HTTP client 30s timeout killed SSE streams mid-response. http.Client.Timeout is a hard wall that overrides context deadlines. Fix: separate streamClient with 5-minute timeout, dynamically extended."

# --------------------------------------------------
banner "👤  User Preferences & Context"
# --------------------------------------------------

add user pref editor            "Uses Neovim with LazyVim distribution. Prefers terminal-first workflow. tmux with custom keybindings."
add user pref language          "Primary: Go for infrastructure. Python for AI/ML pipelines. Rust for performance-sensitive CLI tools. TypeScript when forced."
add user pref communication     "Direct, no fluff. Prefers architectural reasoning over hand-holding. Values: correctness > speed, explicitness > magic, local-first > cloud-dependent."
add user pref dev-philosophy    "TDD absolutist — red-green-refactor on every feature. White-box and black-box QA after every phase. Red-team adversarial testing before merge."
add user pref deploy-philosophy "Prefer single binary deployment. Docker only when necessary (multi-service orchestration). Cross-compile with CGO_ENABLED=0 for static binaries."
add user pref music-coding      "Coding soundtrack: lo-fi hip hop, synthwave, or complete silence. No podcasts while writing code — too distracting for architectural thinking."

# --------------------------------------------------
banner "📊  Metrics & Performance Notes"
# --------------------------------------------------

add memory perf sqlite-wal       "SQLite WAL mode: 3-5 concurrent readers with no contention. Write throughput ~200 ops/sec with busy_timeout=5000. Database at 194MB after importing 77K messages from Hermes."
add memory perf grpc-throughput  "gRPC on Unix socket: <1ms latency per call. TCP localhost: ~2ms. Streaming pub/sub handles 1000 events/sec with 100-subscriber fan-out before dropping."
add memory perf hotblock-latency "Hot block computation: <5ms for 10K memories with priority index. FTS5 search: <10ms for 50K documents. Both well within agent turn budget."
add memory perf migration-time   "Hermes → Vassago migration: 53 memories, 1,349 sessions, 76,923 messages imported in ~30 seconds. All FTS5 indexes rebuilt automatically."

echo ""
echo "  ╭──────────────────────────────────────────────────╮"
echo "  │  Seed complete — 40+ memories ready for demo   │"
echo "  ╰──────────────────────────────────────────────────╯"
echo ""
echo "  Try:"
echo "    vassago-cli --addr $ADDR search 'sqlite'"
echo "    vassago-cli --addr $ADDR search 'bug'"
echo "    vassago-cli --addr $ADDR hot-block memory"
echo "    vassago-cli --addr $ADDR list memory"
echo ""
