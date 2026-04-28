# Mnemosyne

Shared memory daemon for AI agents.

## What

Mnemosyne is a standalone daemon that provides persistent memory for AI agents via gRPC. Multiple agents connect to the same daemon and share memories in real-time — *telepathy* between agents.

- **SQLite** for storage — proven, fast, no separate database server
- **Go** — single binary, no runtime, cross-compile for any OS
- **gRPC** — typed API, streaming for real-time updates, gateway for HTTP later
- **Per-agent hot blocks** — each agent gets a context-appropriate set of memories injected into its system prompt
- **Pub/sub** — when Agent A learns something, Agent B knows within milliseconds

## Why

Hermes' memory subsystem has architectural flaws that can't be patched:
- Three conflicting config sources, deep merge that won't let you remove defaults
- Hot block frozen at agent init, never updated
- Single-agent assumption — no mechanism for agents to share memory
- Self-referential quirks consuming 84% of the memory budget
- Observations that never expire, FTS5 queries that silently fail

See [DESIGN.md](DESIGN.md) for the full architecture and rationale.

## Quick Start

```bash
# Build
make build

# Start the daemon
./bin/mnemosyne-daemon -config configs/mnemosyne.yaml

# Import from Hermes (optional)
./bin/mnemosyne-import -source ~/.hermes/state.db

# Use the CLI
./bin/mnemosyne-cli add --target memory --category project --key "design" --content "Mnemosyne: Go + gRPC memory daemon"
./bin/mnemosyne-cli search "gRPC"
./bin/mnemosyne-cli hot-block
```

## Documentation

- [DESIGN.md](DESIGN.md) — Architecture, schema, API, rationale
- [ROADMAP.md](ROADMAP.md) — Phased delivery plan

## Status

Phase 1 (Foundation) — not yet started. Design spec complete, under review.

## License

Proprietary — aj-nt