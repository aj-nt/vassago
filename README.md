# Mnemosyne

**Shared memory for AI agents.** One daemon, many agents, zero echo loops.

[![License: AGPLv3](https://img.shields.io/badge/Core-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![License: Apache 2.0](https://img.shields.io/badge/Protocol-Apache%202.0-green.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report](https://goreportcard.com/badge/github.com/aj-nt/mnemosyne)](https://goreportcard.com/report/github.com/aj-nt/mnemosyne)
[![Tests](https://img.shields.io/badge/tests-1175%20%2B-race-brightgreen)]()

> **v0.1.0** — Core daemon + hot block + telepathy working. 1175+ tests, race-clean. First-run experience shipped.

Mnemosyne is a memory daemon that gives AI agents persistent, searchable, ***shared*** memory. Every agent sees the same hot blocks, the same facts, the same context — and when one agent learns something, the others know immediately.

**The problem it solves:** Agents without shared memory are amnesiacs. They forget everything between sessions, can't coordinate, and echo each other's outputs in feedback loops. Mnemosyne fixes all three.

**How:** A Go daemon backed by SQLite with a gRPC API. Agents connect, get a priority-sorted "hot block" of relevant context, search memories, and subscribe to real-time updates when other agents write. Self-exclusion prevents echo loops — the core innovation behind the [Telepathy patent](PATENT_DISCLOSURE_TELEPATHY.md).

---

## Try it in 5 minutes

```bash
git clone https://github.com/aj-nt/mnemosyne.git && cd mnemosyne
make all                  # build + test + lint
mnemosyne init            # create dirs, configs, launchd plists
mnemosyne start           # start daemon + agent
mnemosyne chat            # talk to your agent
```

That's it. You're now running an agent with persistent memory. Try:

```bash
# Tell it something
> Remember that the deploy server is at 192.168.1.100

# Ask in a new session (it remembers)
> What's the deploy server address?

# Check what it knows
mnemosyne hot-block        # show computed context window
mnemosyne memory list      # show all memories
```

The agent will automatically save important facts to memory and retrieve them in future sessions. No prompt engineering required.

## What you get

| Feature | What it does |
|---------|-------------|
| **Hot Block** | Per-agent priority-sorted context window — the agent always starts a session knowing what matters |
| **Telepathy** | Real-time pub/sub: agent A writes memory → agent B gets notified instantly |
| **Self-exclusion** | Agents never see their own writes echo back — no feedback loops |
| **Consolidation** | Auto-expiry sweeps idle observations after 30 days. Manual consolidation for spring cleaning |
| **FTS5 Search** | Full-text search across all memories and sessions |
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
                  │  Telepathy    │  ← pub/sub with self-exclusion
                  │  Consolidation│  ← auto-expiry + idle sweep
                  └───────────────┘
```

## Unified CLI

Everything through one binary:

| Command | What it does |
|---------|-------------|
| `mnemosyne init` | First-run setup (dirs, configs, launchd) |
| `mnemosyne start` | Start daemon + agent |
| `mnemosyne stop` | Stop services |
| `mnemosyne chat` | Interactive chat |
| `mnemosyne status` | Service health |
| `mnemosyne memory` | Memory CRUD + search + stats |
| `mnemosyne hot-block` | Show computed context window |
| `mnemosyne health` | Daemon health check |
| `mnemosyne config` | Validate / show / edit config |
| `mnemosyne version` | Build info |

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
    You are Bartimaeus — a direct, dry-witted collaborator.
    You know a lot, don't lecture. Sarcastic but helpful.
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
mnemosyne start     # launchctl start
mnemosyne status    # verify
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
make deploy-studio    # build + scp to remote host
```

## Import from Hermes

```bash
mnemosyne-import --source ~/.hermes/state.db --target ~/.mnemosyne/mnemosyne.db
```

## Testing

```bash
make test-race       # 1175+ tests with race detector
go vet ./...         # static analysis
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
| 20 | Memory Migration, Personality Simplification | ✅ Complete |

**Known limitations:**
- No Docker image yet (planned)
- No Python SDK on PyPI (gRPC client works, pip package planned)
- Single-user — no auth/ACL system yet
- No web UI (CLI only)

See [ROADMAP.md](ROADMAP.md) for detailed phase history and remaining items.

## License

| Component | License | Files |
|-----------|---------|-------|
| Core implementation | **GNU AGPLv3** | `daemon/`, `agent/`, `cmd/`, all other packages |
| Protocol & client SDK | **Apache 2.0** | `proto/`, `client/`, `messaging/format/` |

The Apache 2.0 packages can be used independently — build compatible clients without AGPLv3 obligations. All other source files are AGPLv3, which requires distributing source (including modifications) when running as a network service.

See [LICENSE-AGPLv3](LICENSE-AGPLv3) and [LICENSE-Apache-2.0](LICENSE-Apache-2.0) for full text.