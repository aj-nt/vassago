# Mnemosyne Python gRPC Client Example

Connect to a running Mnemosyne daemon, add a memory, read the hot block,
search memories, and subscribe to real-time updates from other agents.

## Prerequisites

```bash
pip install grpcio grpcio-tools
```

The daemon must be running (`mnemosyne start`). This script assumes the
generated Python stubs are in `../../proto/` (run `make proto-python`).

## Quick start

```bash
# Generate Python stubs from the proto definitions
make proto-python

# Run the example
python demo.py
```

## What it demonstrates

1. **AddMemory** — Write a fact that persists across sessions
2. **GetHotBlock** — Read the computed context window for an agent
3. **Search** — Full-text search across all memories
4. **Subscribe** — Real-time notifications when other agents write (Telepathy)
5. **Self-exclusion** — Your own writes don't echo back to you

The core insight: two agents sharing one daemon, but neither sees its own
writes in the subscription stream. That's what breaks the echo loop.