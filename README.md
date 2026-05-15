# Vassago — AI Agent Memory Daemon

**Named after the Goetic demon Vassago**, who "declares things past, present, and future, and discovers things hidden and lost." A memory daemon for AI agents.

## What It Is

Vassago is a gRPC-based memory daemon written in Go. It provides shared, durable memory for AI agents — storing conversations, preferences, knowledge, and observations in SQLite with FTS5 full-text search. Multi-agent, language-agnostic, local-first.

## Quick Start

```bash
# Build
make build

# Run
./bin/vassago-daemon --config config.yaml
```

## Architecture

- **Daemon** — gRPC server with SQLite backing store
- **CLI** — `vassago` command-line client
- **SDK** — Go and Python client libraries
- **Pub/Sub** — real-time event notifications for connected agents
- **Cron** — scheduled job execution
- **Extraction** — LLM-powered autonomous memory extraction from conversations

## Packages

| Package | Coverage |
|---------|----------|
| daemon/internal/store | 80.9% |
| daemon/internal/grpc | 82.3% |
| daemon/internal/cron | 89.1% |
| daemon/internal/pubsub | 100% |
| daemon/internal/daemon | 54.5% |

## License

AGPLv3
