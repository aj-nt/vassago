# Mnemosyne

**Shared memory for AI agents.** One daemon, many agents, zero echo loops.

[![License: AGPLv3](https://img.shields.io/badge/Core-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![License: Apache 2.0](https://img.shields.io/badge/Protocol-Apache%202.0-green.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report](https://goreportcard.com/badge/github.com/aj-nt/mnemosyne)](https://goreportcard.com/report/github.com/aj-nt/mnemosyne)

> Mnemosyne is a memory daemon that gives AI agents persistent, searchable, ***shared*** memory via gRPC.

Every connected agent sees the same hot blocks, the same facts, the same context — and when one agent learns something, the others know immediately. Self-exclusion prevents echo loops.

**How:** A Go daemon backed by SQLite (WAL mode) with a gRPC API. Agents connect, get a priority-sorted "hot block" of relevant context, search memories, and subscribe to real-time updates when other agents write.

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

Now connect an agent. Use the Python SDK:

```python
from mnemosyne import MnemosyneClient

async with MnemosyneClient("localhost:50051") as client:
    await client.add_memory(
        key="deploy-server",
        content="The deploy server is at node-3.internal",
        category="environment",
        priority=4,
    )
    block = await client.get_hot_block()
    print(block.content)
```

---

## What you get

| Feature | What it does |
|---------|-------------|
| **Hot Block** | Per-agent priority-sorted context window — inject relevant context into any agent's prompt |
| **Skills** | Shared procedural memory — reusable workflows, debug procedures, project conventions |
| **Telepathy** | Real-time pub/sub: agent A writes memory → agent B gets notified instantly |
| **Self-exclusion** | Agents never see their own writes echo back — no feedback loops |
| **Consolidation** | Auto-expiry sweeps idle observations. Configurable TTL per priority tier |
| **FTS5 Search** | Full-text search across all memories, sessions, and skills |
| **Multi-agent** | One database, many agents — serialized writes, concurrent reads |
| **gRPC API** | Connect from Go, Python, any language. Protocol definitions are Apache 2.0 |

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
| `mnemosyne init` | First-run setup (dirs, configs, launchd plists) |
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

Standalone binaries (`mnemosyne-daemon`, `mnemosyne-cli`, `mnemosyne-import`) are also built and available.

## Configuration

Daemon config (`~/.mnemosyne/config.yaml`):

```yaml
daemon:
  socket: ~/.mnemosyne/mnemosyne.sock   # Unix socket (local)
  tcp: localhost:50051                   # TCP listener (LAN/internet)

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

## Client Libraries

**Go:** Import `github.com/aj-nt/mnemosyne/client` — the Go client is in this repo.

**Python:** The Python SDK lives in `sdk/python/`. Install locally:

```bash
cd sdk/python && pip install -e .
```

```python
from mnemosyne import MnemosyneClient

async with MnemosyneClient("localhost:50051") as client:
    await client.add_memory(key="foo", content="bar", category="project")
    results = await client.search_memories("foo")
    block = await client.get_hot_block()
```

See [`sdk/python/README.md`](sdk/python/README.md) for the full API reference.

**Other languages:** Use the gRPC proto definitions in `proto/`. The protocol is Apache 2.0 licensed.

---

## Import from Hermes

```bash
mnemosyne-import --source ~/.hermes/state.db --target ~/.mnemosyne/mnemosyne.db
```

See [DESIGN.md §8](DESIGN.md#8-data-migration) for migration details.

---

## Testing

```bash
make test-race       # run tests with race detector
make coverage        # coverage report (coverage.out)
make coverage-html   # HTML coverage report (coverage.html)
make lint            # go vet + golangci-lint
make all             # build + test + lint
```

---

## Contributing

1. **Fork & branch:** `git checkout -b your-feature-name`
2. **Run tests:** `make test-race` — all must pass
3. **Lint:** `go vet ./...` and `gofmt -s -w .`
4. **Submit a PR** with a clear description of what and why

**Code style:** Standard Go conventions. `gofmt` is law.

---

## License

| Component | License | Files |
|-----------|---------|-------|
| Core implementation | **GNU AGPLv3** | `daemon/`, `cmd/`, all other packages |
| Protocol & client SDK | **Apache 2.0** | `proto/`, `client/`, `messaging/format/` |

The Apache 2.0 packages can be used independently — build compatible clients without AGPLv3 obligations. All other source files are AGPLv3, which requires distributing source (including modifications) when running as a network service.

See [LICENSE-AGPLv3](LICENSE-AGPLv3) and [LICENSE-Apache-2.0](LICENSE-Apache-2.0) for full text.
