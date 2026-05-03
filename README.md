# Mnemosyne

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
make all           # build + test + lint

# Set up (creates configs, directories, launchd plists)
make setup          # or: ./scripts/setup.sh

# Deploy locally
make deploy         # copies binaries to ~/bin/

# Or deploy to Studio
make deploy-studio  # scp + codesign

# Verify
mnemosyne-cli health
```

The setup script creates `~/.mnemosyne/` with:
- `config.yaml` — Daemon config (SQLite, hot block, consolidation)
- `agent.yaml` — Agent config (provider, tools, personality)
- `personalities/bartimaeus.md` — Default personality
- LaunchAgent plists for macOS auto-start

For Studio (always-on server), use `./scripts/setup.sh --studio` which configures
the daemon to listen on all interfaces (`0.0.0.0:50051`) and enables the chat server.

## CLI Reference

```bash
# Add a memory
mnemosyne-cli add memory environment "os-version" "macOS 15 arm64"

# Get memory by ID
mnemosyne-cli get <id>

# Search (FTS5)
mnemosyne-cli search "project directory"

# List memories
mnemosyne-cli list              # default: target=memory
mnemosyne-cli list user         # user profile entries

# Remove memory
mnemosyne-cli remove <id>

# Hot block (computed context)
mnemosyne-cli hot-block          # memory hot block
mnemosyne-cli hot-block user     # user profile

# Consolidation
mnemosyne-cli consolidate expired      # remove expired entries
mnemosyne-cli consolidate idle          # remove idle low-priority entries
mnemosyne-cli consolidate observations  # remove old observations
mnemosyne-cli consolidate all           # full sweep

# Statistics
mnemosyne-cli stats

# Health check
mnemosyne-cli health
```

## Configuration

`~/.mnemosyne/config.yaml`:

```yaml
daemon:
  socket: ~/.mnemosyne/mnemosyne.sock   # Unix socket (optional)
  tcp: localhost:50051                    # TCP listener
  api_key: ""                             # Set to require auth, empty = no auth
  tls:
    cert: ""                              # TLS cert path
    key: ""                               # TLS key path

database:
  path: ~/.mnemosyne/mnemosyne.db
  wal_mode: true
  busy_timeout: 5000

hot_block:
  memory_char_limit: 4000
  user_char_limit: 2000
  min_priority: 2
  observation_ttl_days: 30
```

## Import from Hermes

```bash
# Import your existing Hermes state.db
./mnemosyne-import --source ~/.hermes/state.db --target ~/.mnemosyne/mnemosyne.db
```

## Deployment

### macOS (launchd)

```bash
# Copy the plist
mkdir -p ~/Library/LaunchAgents
cp configs/com.mnemosyne.daemon.plist ~/Library/LaunchAgents/
# Edit the plist to set your username, then:
launchctl load ~/Library/LaunchAgents/com.mnemosyne.daemon.plist
```

### Linux (systemd)

```ini
[Unit]
Description=Mnemosyne Memory Daemon
After=network.target

[Service]
User=youruser
ExecStart=/usr/local/bin/mnemosyne-daemon --config /home/youruser/.mnemosyne/config.yaml
Restart=always

[Install]
WantedBy=multi-user.target
```

## API

gRPC service defined in `proto/mnemosyne.proto`. Key RPCs:

| RPC | Description |
|-----|-------------|
| AddMemory | Create or update a memory entry |
| GetMemory | Retrieve by ID |
| RemoveMemory | Delete by ID |
| ListMemories | List by target/category |
| Search | FTS5 full-text search |
| GetHotBlock | Computed context for an agent |
| Subscribe | Stream real-time memory updates |
| Consolidate | Run expiry/idle cleanup |

## Testing

```bash
go test ./... -race    # 127 tests with race detector
go vet ./...           # static analysis
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
| 8 | Hardening & Polish | ✅ Current |

## License

MIT