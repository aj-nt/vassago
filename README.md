# Mnemosyne

**Shared memory for AI agents.** One daemon, many agents, zero echo loops.

[![License: AGPLv3](https://img.shields.io/badge/Core-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![License: Apache 2.0](https://img.shields.io/badge/Protocol-Apache%202.0-green.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report](https://goreportcard.com/badge/github.com/aj-nt/mnemosyne)](https://goreportcard.com/report/github.com/aj-nt/mnemosyne)

> **v0.1.0** — The memory daemon. Agents connect via gRPC to read, write, search, and subscribe to shared memory.

Mnemosyne is a memory daemon that gives AI agents persistent, searchable, ***shared*** memory. Every connected agent sees the same hot blocks, the same facts, the same context — and when one agent learns something, the others know immediately.

**The problem it solves:** Agents without shared memory are amnesiacs. They forget everything between sessions, can't coordinate, and echo each other's outputs in feedback loops. Mnemosyne fixes all three.

**How:** A Go daemon backed by SQLite with a gRPC API. Agents connect, get a priority-sorted "hot block" of relevant context, search memories, and subscribe to real-time updates when other agents write. Self-exclusion prevents echo loops — the core innovation behind Mnemosyne's telepathy system.

---

## Try it in 5 minutes

**Prerequisites:** Go 1.21+ and `make`.

```bash
git clone https://github.com/aj-nt/mnemosyne.git && cd mnemosyne
make install              # build + install to ~/bin
export PATH="$HOME/bin:$PATH"
mnemosyne init            # interactive setup (creates config + dirs)
mnemosyne start           # start the daemon
mnemosyne status          # verify it's healthy
```

Now connect an agent. Use [mnemoagent](https://github.com/aj-nt/mnemoagent) (Go CLI/TUI agent), or the Python SDK:

```bash
pip install mnemosyne-sdk
```

```python
import asyncio
from mnemosyne import MnemosyneClient

async def main():
    async with MnemosyneClient("localhost:50051") as client:
        # Add a memory
        await client.add_memory(
            key="deploy-server",
            content="The deploy server is at node-3.internal",
            category="environment",
            priority=4,
        )
        # Get the hot block
        block = await client.get_hot_block()
        print(block.content)

asyncio.run(main())
```

## What you get

| Feature | What it does |
|---------|-------------|
| **Hot Block** | Per-agent priority-sorted context window — inject relevant context into any agent's prompt |
| **Skills** | Shared procedural memory — reusable workflows, debug procedures, project conventions |
| **Telepathy** | Real-time pub/sub: agent A writes memory → agent B gets notified instantly |
| **Self-exclusion** | Agents never see their own writes echo back — no feedback loops |
| **Consolidation** | Auto-expiry sweeps idle observations after 30 days. Manual consolidation for spring cleaning |
| **FTS5 Search** | Full-text search across all memories, sessions, and skills |
| **gRPC API** | Connect from Go, Python, any language. Protocol is Apache 2.0 |

## Architecture

```
┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│  Agent A     │   │  Agent B     │   │  Agent C     │
│  (Go)        │   │  (gRPC)      │   │  (Python)    │
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

## CLI

Everything through one binary:

| Command | What it does |
|---------|-------------|
| `mnemosyne init` | First-run setup (dirs, configs, launchd) |
| `mnemosyne start` | Start the daemon (with health verification) |
| `mnemosyne stop` | Stop the daemon |
| `mnemosyne status` | Daemon health + database info |
| `mnemosyne daemon` | Run daemon in foreground |
| `mnemosyne memory` | Memory CRUD + search + stats |
| `mnemosyne hot-block` | Show computed context window |
| `mnemosyne sessions` | List, resume, and inspect past sessions |
| `mnemosyne export` | Export all memories to JSON |
| `mnemosyne import` | Import memories from JSON (with backup) |
| `mnemosyne health` | Daemon health check |
| `mnemosyne config` | Validate / show / edit config |
| `mnemosyne version` | Build info with commit hash |

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

## Deployment

**macOS (launchd):**

```bash
mnemosyne init      # creates LaunchAgent plist
mnemosyne start     # launchctl start + health verification
mnemosyne status    # check daemon is running
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

# Run daemon
docker run -d -p 50051:50051 -v mnemosyne-data:/data mnemosyne

# Check health
docker compose exec daemon mnemosyne health
```

The Docker image includes the unified CLI, `mnemosyne-daemon`, `mnemosyne-cli`, and `mnemosyne-import`. By default it starts the daemon.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `mnemosyne: command not found` | `export PATH="$HOME/bin:$PATH"` or run `./bin/mnemosyne` directly |
| `Error: Configuration not found` | Run `mnemosyne init` first |
| `gRPC unreachable` after `start` | Check logs: `cat ~/.mnemosyne/daemon.err` |
| Agent can't connect | Verify the daemon is running: `mnemosyne status` |

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
2. **Run tests:** `make test-race` — all must pass
3. **Lint:** `go vet ./...` and `gofmt -s -w .`
4. **Submit a PR** with a clear description of what and why

**Code style:** Standard Go conventions. `gofmt` is law.

See [ROADMAP.md](ROADMAP.md) for planned work.

---

## Import from Hermes

```bash
mnemosyne-import --source ~/.hermes/state.db --target ~/.mnemosyne/mnemosyne.db
```

## Python SDK

```bash
pip install mnemosyne-sdk
```

See [`sdk/python/README.md`](sdk/python/README.md) for the full API reference.

## Testing

```bash
make test-race       # run tests with race detector
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
| 21 | Open Source Readiness | ✅ Complete |
| 22 | Python SDK | ✅ Complete |
| 23 | Multi-User & Auth | 🔜 Planned |
| 24 | Bubble Tea TUI | ✅ Complete |
| 25 | Saved Tools — Runtime Tool Creation | ✅ Complete |
| 26 | Reasoning Model Support | ✅ Complete |

See [ROADMAP.md](ROADMAP.md) for detailed phase history.

## License

| Component | License | Files |
|-----------|---------|-------|
| Core implementation | **GNU AGPLv3** | `daemon/`, `cmd/`, all other packages |
| Protocol & client SDK | **Apache 2.0** | `proto/`, `client/`, `messaging/format/` |

The Apache 2.0 packages can be used independently — build compatible clients without AGPLv3 obligations. All other source files are AGPLv3, which requires distributing source (including modifications) when running as a network service.

See [LICENSE-AGPLv3](LICENSE-AGPLv3) and [LICENSE-Apache-2.0](LICENSE-Apache-2.0) for full text.
