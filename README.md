# Mnemosyne

[![License: AGPLv3](https://img.shields.io/badge/Core-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![License: Apache 2.0](https://img.shields.io/badge/Protocol-Apache%202.0-green.svg)](https://opensource.org/licenses/Apache-2.0)

A shared memory daemon for AI agents. Mnemosyne gives agents persistent, searchable, shared memory — every agent sees the same hot blocks, the same facts, the same context. Built with Go, SQLite, and gRPC.

## Architecture

```
┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│  Agent A     │   │  Agent B     │   │  Agent C     │
│  (Go SDK)    │   │  (gRPC)     │   │  (CLI)       │
└──────┬───────┘   └──────┬──────┘   └──────┬───────┘
       │                  │                  │
       └──────────────────┼──────────────────┘
                          │ gRPC (TCP/Unix)
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
                  │  Telepathy    │  ← pub/sub event routing
                  │  Consolidation│  ← auto-expiry + idle sweep
                  └───────────────┘
```

**Core concepts:**
- **Hot Block** — Each agent gets a computed, priority-sorted context block (memories + user profile), truncated to a character limit. No more stale caches.
- **Telepathy** — When agent A modifies memory, agent B gets notified via gRPC server-stream, broken feedback loops.
- **Consolidation** — Expired entries are swept hourly. Idle low-priority entries are cleaned up after 30 days.

## Quick Start

```bash
# Clone and build
git clone https://github.com/aj-nt/mnemosyne.git
cd mnemosyne
make all              # build + test + lint

# First-run setup (creates dirs, configs, personality, launchd plists)
mnemosyne init

# Start services
mnemosyne start

# Verify health
mnemosyne health

# Start chatting
mnemosyne chat
```

Or with the unified CLI:

```bash
# View status
mnemosyne status

# Config management
mnemosyne config validate
mnemosyne config show
mnemosyne config edit
mnemosyne config dirs

# Memory management
mnemosyne memory add memory environment "os-version" "macOS 15 arm64"
mnemosyne memory search "project directory"
mnemosyne memory list
mnemosyne memory stats

# Hot block
mnemosyne hot-block
```

## Unified CLI

The `mnemosyne` command provides all functionality in a single binary:

| Command | Description |
|---------|-------------|
| `mnemosyne init` | First-run setup (dirs, configs, personality) |
| `mnemosyne start` | Start daemon and agent services |
| `mnemosyne stop` | Stop services |
| `mnemosyne status` | Show daemon/agent status |
| `mnemosyne daemon` | Run daemon (foreground) |
| `mnemosyne agent` | Run agent (foreground or server) |
| `mnemosyne chat` | Interactive chat session |
| `mnemosyne memory` | Memory management (add/get/search/list/remove/stats) |
| `mnemosyne hot-block` | Show computed hot block |
| `mnemosyne health` | Daemon health check |
| `mnemosyne config` | Config management (validate/show/edit/dirs) |
| `mnemosyne version` | Version info |

Legacy binaries (`mnemosyne-daemon`, `mnemosyne-agent`, `mnemosyne-cli`) still work — the unified CLI delegates to them where needed.

## Configuration

`~/.mnemosyne/config.yaml` (daemon):

```yaml
daemon:
  socket: ~/.mnemosyne/mnemosyne.sock
  tcp: localhost:50051
  tls:
    cert: ""
    key: ""

database:
  path: ~/.mnemosyne/mnemosyne.db
  wal_mode: true
  busy_timeout: 5000

hot_block:
  memory_char_limit: 8000
  user_char_limit: 3000
  min_priority: 2
  boost_categories: [project, environment, quirk]
  suppress_categories: [observation]

consolidation:
  max_idle_days: 60

logging:
  level: info
  format: json
```

`~/.mnemosyne/agent.yaml` (agent):

```yaml
agent:
  id: mnemosyne
  name: mnemosyne
  role: assistant
  personality: "direct, dry-witted collaborator"
  personality_file: ~/.mnemosyne/personalities/bartimaeus.md
  max_turns: 30

provider:
  name: anthropic
  model: claude-sonnet-4-6
  temperature: 0.7
  fallback1: openai
  openai_model: glm-5.1:cloud
  openai_base_url: http://localhost:11434/v1

mnemosyne:
  address: localhost:50051
  chat_listen: ":50052"

tools:
  enabled:
    - AddMemory
    - SearchMemories
    - ReplaceMemory
    - RemoveMemory
    - Consolidate
    - GetHotBlock
    - FileRead
    - FileSearch
    - FileWrite
    - Execute
    - SearchWeb
    - FetchURL
    - SearchSessions
    - ListSessions
    - GetSession
    - AddTodo
    - ListTodos
    - CompleteTodo
    - RemoveTodo
    - ScheduleCron
    - SendMessage
    - Browse
    - ListAgents
    - RegisterAgent
    - FindAgent
  approval_required: [FileWrite, Execute]
  allowed_dirs: [/tmp, ~]

delegation:
  enabled: true
  max_depth: 1
  max_turns: 10
```

## Import from Hermes

```bash
mnemosyne-import --source ~/.hermes/state.db --target ~/.mnemosyne/mnemosyne.db
```

## Deployment

### macOS (launchd)

```bash
mnemosyne init            # Creates LaunchAgent plists
mnemosyne start           # Uses launchctl to start services
mnemosyne status          # Verify services are running
```

Or manual:
```bash
make deploy               # Build + copy to ~/bin/
make deploy-studio        # Build + scp to Studio
```

### Linux (systemd)

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

## Testing

```bash
make test-race         # 814 tests with race detector
go vet ./...           # static analysis
make all               # build + test + lint
```

## Project Status

| Phase | Deliverable | Status |
|-------|-------------|--------|
| 1 | Daemon Foundation | ✅ Merged |
| 2 | Agent Core | ✅ Merged |
| 3 | Telepathy (pub/sub) | ✅ Merged |
| 4 | Search & Consolidation | ✅ Merged |
| 5 | Agent Tools & Anthropic Provider | ✅ Merged |
| 6 | Data Migration (Hermes import) | ✅ Merged |
| 7 | Client Polish & Admin CLI | ✅ Merged |
| 8 | Hardening & Polish | ✅ Merged |
| 9 | Security & Daily-Driver | ✅ Merged |
| 10-13 | E2E, Stress, Provider Improvements | ✅ Merged |
| 14A-E | Multi-Agent (Reg, Telepathy, Delegate, Scheduler, Validate) | ✅ Merged |
| 15A-E | Messaging (Discord, Telegram, iMessage, Email, Unification) | ✅ Merged |
| 16A-B | CronJobs & Proactive Notifications | ✅ Merged |
| 17A-E | Conversation Quality, Sessions, Todos, Architecture, ChatStream | ✅ Merged |
| 18A-D | Browser, Vision, Multi-Agent, Production Hardening | ✅ Merged |
| 19A | Unified CLI & First-Run Experience | ✅ Current |

## License

Dual-licensed:

| Component | License | Files |
|-----------|---------|-------|
| Core implementation | **GNU AGPLv3** | `daemon/`, `agent/`, `cmd/`, all other packages |
| Protocol & client SDK | **Apache 2.0** | `proto/`, `client/`, `messaging/format/` |

The Apache 2.0 packages can be used independently — anyone can build compatible clients without AGPLv3 obligations. All other source files are AGPLv3, which requires distributing source (including modifications) when running as a network service.

See [LICENSE-AGPLv3](LICENSE-AGPLv3) and [LICENSE-Apache-2.0](LICENSE-Apache-2.0) for full text.