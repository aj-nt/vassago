# Mnemosyne

**Shared memory for AI agents.** One daemon, many agents, zero echo loops.

[![License: AGPLv3](https://img.shields.io/badge/Core-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![License: Apache 2.0](https://img.shields.io/badge/Protocol-Apache%202.0-green.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report](https://goreportcard.com/badge/github.com/aj-nt/mnemosyne)](https://goreportcard.com/report/github.com/aj-nt/mnemosyne)
[![Tests](https://img.shields.io/badge/tests-1236%20%2B-race-brightgreen)]()

> **v0.1.0** — Public release. 1236+ tests, race-clean.

Mnemosyne is a memory daemon that gives AI agents persistent, searchable, ***shared*** memory. Every agent sees the same hot blocks, the same facts, the same context — and when one agent learns something, the others know immediately.

**The problem it solves:** Agents without shared memory are amnesiacs. They forget everything between sessions, can't coordinate, and echo each other's outputs in feedback loops. Mnemosyne fixes all three.

**How:** A Go daemon backed by SQLite with a gRPC API. Agents connect, get a priority-sorted "hot block" of relevant context, search memories, and subscribe to real-time updates when other agents write. Self-exclusion prevents echo loops — the core innovation behind Mnemosyne's telepathy system.

---

## Try it in 5 minutes

**Prerequisites:** Go 1.21+, an LLM API key (Anthropic or OpenAI), and `make`.

```bash
git clone https://github.com/aj-nt/mnemosyne.git && cd mnemosyne
make install              # build + install to ~/bin
export PATH="$HOME/bin:$PATH"
mnemosyne init            # interactive setup (configs, API key, tools)
mnemosyne start           # start daemon + agent (with health verification)
mnemosyne chat            # talk to your agent
```

That's it. You're now running an agent with persistent memory. Try:

```
> Remember that the deploy server is at node-3.internal

> What's the deploy server address?     ← it remembers across sessions

> /quit                                 ← exit and come back later
```

Outside the chat, you can inspect memory directly:

```bash
mnemosyne hot-block        # show computed context window
mnemosyne memory list      # show all memories
mnemosyne memory search "deploy"   # full-text search
mnemosyne health           # daemon health check
```

The agent automatically saves important facts to memory and retrieves them in future sessions. No prompt engineering required.

## What you get

| Feature | What it does |
|---------|-------------|
| **Hot Block** | Per-agent priority-sorted context window — the agent always starts a session knowing what matters |
| **Skills** | Shared procedural memory — reusable workflows, debug procedures, and project conventions injected into agent prompts |
| **Telepathy** | Real-time pub/sub: agent A writes memory → agent B gets notified instantly |
| **Self-exclusion** | Agents never see their own writes echo back — no feedback loops |
| **Consolidation** | Auto-expiry sweeps idle observations after 30 days. Manual consolidation for spring cleaning |
| **FTS5 Search** | Full-text search across all memories, sessions, and skills |
| **gRPC API** | Connect from Go, Python, any language. Protocol is Apache 2.0 |

## Architecture

```
┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│  Agent A     │   │  Agent B     │   │  Agent C     │
│  (Go SDK)    │   │  (gRPC)      │   │  (Python)    │
└──────┬───────┘   └──────┬──────┘   └──────┬───────┘
       │                  │                  │
       └──────────────────┼──────────────────┘
                          │ gRPC (TCP/Unix socket)
                  ┌───────┴───────┐
                  │   Mnemosyne   │
                  │   Daemon      │
                  │               │
                  │  ┌─────────┐ │
                  │  │ SQLite  │ │
                  │  │ (WAL)   │ │
                  │  └─────────┘ │
                  │               │
                  │  Hot Block    │  ← per-agent context window
                  │  Skills       │  ← shared procedural memory
                  │  Telepathy    │  ← pub/sub with self-exclusion
                  │  Consolidation│  ← auto-expiry + idle sweep
                  └───────────────┘
```

## Unified CLI

Everything through one binary:

| Command | What it does |
|---------|-------------|
| `mnemosyne init` | First-run setup (dirs, configs, launchd) |
| `mnemosyne start` | Start daemon + agent (with health verification) |
| `mnemosyne stop` | Stop services |
| `mnemosyne chat` | Interactive chat (type `/quit` to exit, `/hot` for context) |
| `mnemosyne status` | Service health + database info |
| `mnemosyne memory` | Memory CRUD + search + stats |
| `mnemosyne hot-block` | Show computed context window |
| `mnemosyne sessions` | List, resume, and inspect past sessions |
| `mnemosyne export` | Export all memories to JSON |
| `mnemosyne import` | Import memories from JSON (with backup) |
| `mnemosyne health` | Daemon health check |
| `mnemosyne config` | Validate / show / edit config |
| `mnemosyne version` | Build info with commit hash |

Legacy binaries (`mnemosyne-daemon`, `mnemosyne-agent`, `mnemosyne-cli`) also work.

## Configuration

Daemon config (`~/.mnemosyne/config.yaml`):

```yaml
daemon:
  socket: ~/.mnemosyne/mnemosyne.sock
  tcp: localhost:50051

database:
  path: ~/.mnemosyne/mnemosyne.db
  wal_mode: true

hot_block:
  memory_char_limit: 8000
  user_char_limit: 3000
  min_priority: 2
  boost_categories: [project, environment, quirk]
  suppress_categories: [observation]
```

Agent config (`~/.mnemosyne/agent.yaml`):

```yaml
agent:
  id: mnemosyne
  name: mnemosyne
  personality: |-
    You are a direct, dry-witted collaborator. You know a lot but don't lecture. You prefer short, concise answers. You are proactive with your tools. You do not fluff or flatter. You are honest about uncertainty. 
  max_turns: 30

provider:
  name: anthropic
  model: claude-sonnet-4-6
  temperature: 0.7

tools:
  enabled:
    - AddMemory
    - SearchMemories
    - FileRead
    - FileSearch
    - Execute
  approval_required: [FileWrite, Execute]
```

## Deployment

**macOS (launchd):**

```bash
mnemosyne init      # creates LaunchAgent plists
mnemosyne start     # launchctl start + health verification
mnemosyne status    # check all services
```

**Linux (systemd):**

```ini
[Unit]
Description=Mnemosyne Memory Daemon
After=network.target

[Service]
User=youruser
ExecStart=/usr/local/bin/mnemosyne daemon --config /home/youruser/.mnemosyne/config.yaml
Restart=always

[Install]
WantedBy=multi-user.target
```

**Cross-machine deploy:**

```bash
make deploy           # build + copy to ~/bin/
make deploy-studio   # build + scp to remote host
```

### Docker

```bash
# Build and run with Docker Compose
docker compose up -d

# Or build manually
docker build -t mnemosyne .

# Run daemon only
docker run -d -p 50051:50051 -v mnemosyne-data:/data mnemosyne

# Check health
docker compose exec daemon mnemosyne health
```

The Docker image includes all binaries (`mnemosyne`, `mnemosyne-daemon`, `mnemosyne-agent`, `mnemosyne-cli`, `mnemosyne-import`). By default it starts the unified CLI in daemon mode.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `mnemosyne: command not found` | `export PATH="$HOME/bin:$PATH"` or run `./bin/mnemosyne` directly |
| `Error: Configuration not found` | Run `mnemosyne init` first |
| `gRPC unreachable` after `start` | Check logs: `cat ~/.mnemosyne/daemon.err` |
| `Cannot connect to Mnemosyne daemon` | Run `mnemosyne start` before `mnemosyne chat` |
| Agent says "I don't have access to tools" | Check `tools.enabled` in `~/.mnemosyne/agent.yaml` |
| API key not detected | Set `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` in your shell profile |

## Why not just...

| Alternative | Problem |
|-------------|---------|
| **Raw SQLite** | You get storage, but no hot blocks, no priority eviction, no telepathy, no self-exclusion. You're building all of that yourself. |
| **MemGPT / Letta** | Single-agent memory. No shared context, no cross-agent pub/sub, no daemon architecture. Each agent is an island. |
| **Redis / Valkey** | Fast key-value, but no FTS5 search, no priority-sorted context windows, no consolidation. You'd reinvent all of this on top. |
| **ChromaDB / Milvus** | Vector search is great for embeddings, not for structured agent memory with categories, priorities, and real-time subscriptions. |
| **Just add it to the system prompt** | Context windows fill up. Hot blocks auto-evict low-priority items. System prompts don't search, don't consolidate, don't notify other agents. |

**TL;DR:** Mnemosyne isn't a database with a chat wrapper. It's a memory *system* — priority eviction, cross-agent sync, self-exclusion, and consolidation all work out of the box.

---

## Contributing

Contributions welcome! Here's how to get started:

1. **Fork & branch:** `git checkout -b your-feature-name`
2. **Run tests:** `make test-race` — all 1236+ must pass
3. **Lint:** `go vet ./...` and `gofmt -s -w .`
4. **Submit a PR** with a clear description of what and why

**Code style:** Standard Go conventions. `gofmt` is law. Error types in `errors/`, shared helpers in `internal/`.

**Bug reports:** Open a GitHub issue with steps to reproduce, expected vs actual behavior, and `mnemosyne version` output.

**Feature requests:** Open a discussion first before writing code. We prefer small, focused PRs over large rewrites.

See [ROADMAP.md](ROADMAP.md) for planned work if you want to pick something up.

---

## Import from Hermes

```bash
mnemosyne-import --source ~/.hermes/state.db --target ~/.mnemosyne/mnemosyne.db
```

## Testing

```bash
make test-race       # 1236+ tests with race detector
make coverage        # coverage report (coverage.out)
make coverage-html   # HTML coverage report (coverage.html)
make lint            # go vet + golangci-lint
make all             # build + test + lint
```

## Project status

| Phase | Deliverable | Status |
|-------|-------------|--------|
| 1–5 | Daemon, Agent, Telepathy, Search, Tools | ✅ Complete |
| 6–9 | Migration, Client, Hardening, Security | ✅ Complete |
| 10–14 | E2E, Stress, Provider, Multi-Agent | ✅ Complete |
| 15–18 | Messaging, Cron, Quality, Browser, Vision | ✅ Complete |
| 19 | GoF Refactoring, Spiral Detection, Polish | ✅ Complete |
| 20 | Memory Migration, Personality Simplification, Agent Loop Fixes | ✅ Complete |
| 21 | Open Source Readiness | 🔜 Planned |
| 22 | Python SDK | 🔜 Planned |
| 23 | Community & Docs | 🔜 Planned |

**Known limitations:**
- ~~No Docker image yet (planned)~~ → Docker image available (see Docker section)
- No Python SDK on PyPI (gRPC client works, pip package planned — Phase 22)
- Single-user — no auth/ACL system yet (Phase 23)
- No web UI (CLI only)
- Browser tool requires ChromeDP installed

**v1.0 blockers vs. v0.x accepted:**

| Limitation | v1.0 Blocker? | Status |
|------------|----------------|--------|
| No Python SDK | No | Phase 22 |
| No multi-user auth | Yes | Phase 23 |
| No web UI | No | Future |
| Single SQLite DB | No | Sufficient for personal/team use |
| No built-in auth | Yes | Phase 23 |

See [ROADMAP.md](ROADMAP.md) for detailed phase history and remaining items.

## License

| Component | License | Files |
|-----------|---------|-------|
| Core implementation | **GNU AGPLv3** | `daemon/`, `agent/`, `cmd/`, all other packages |
| Protocol & client SDK | **Apache 2.0** | `proto/`, `client/`, `messaging/format/` |

The Apache 2.0 packages can be used independently — build compatible clients without AGPLv3 obligations. All other source files are AGPLv3, which requires distributing source (including modifications) when running as a network service.

See [LICENSE-AGPLv3](LICENSE-AGPLv3) and [LICENSE-Apache-2.0](LICENSE-Apache-2.0) for full text.