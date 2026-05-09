# Mnemosyne Python SDK

Async-first Python client for the [Mnemosyne](https://github.com/aj-nt/mnemosyne) memory daemon.

## Installation

```bash
pip install mnemosyne-sdk
```

**Prerequisite:** The Mnemosyne daemon must be running. Start it with:

```bash
mnemosyne start
```

## Quick Start

```python
import asyncio
from mnemosyne import MnemosyneClient

async def main():
    async with MnemosyneClient("localhost:50051") as client:
        # Health check
        status = await client.ping()
        print(f"Daemon status: {status}")

        # Add a memory
        mem = await client.add_memory(
            key="deploy-server",
            content="The deploy server is at node-3.internal",
            category="environment",
            priority=4,
        )
        print(f"Added: {mem}")

        # Get the hot block (context window)
        block = await client.get_hot_block(target="memory")
        print(f"Hot block: {block.entry_count} entries, {block.char_count} chars")

        # Search memories
        results = await client.search_memories("deploy")
        for m in results:
            print(f"  [{m.category}] {m.key}: {m.content[:60]}")

asyncio.run(main())
```

## API Reference

### Connection

```python
# Connect with defaults (localhost:50051)
async with MnemosyneClient() as client:
    ...

# Custom address and timeout
async with MnemosyneClient("192.168.1.100:50051", timeout=10.0) as client:
    ...

# With API key
async with MnemosyneClient("localhost:50051", api_key="sk-your-key") as client:
    ...
```

### Memory CRUD

| Method | Description |
|--------|-------------|
| `add_memory(key, content, ...)` | Add or update a memory (upsert by target+key) |
| `get_memory(memory_id)` | Get a memory by ID |
| `update_memory(memory_id, ...)` | Partial update (content, priority, category) |
| `remove_memory(memory_id)` | Delete a memory |
| `list_memories(target, category, ...)` | List memories with filters |

### Search

| Method | Description |
|--------|-------------|
| `search_memories(query, target, limit)` | Full-text search |
| `search_sessions(query, limit)` | Search past conversations |
| `list_sessions(limit)` | List recent sessions |

### Hot Block

| Method | Description |
|--------|-------------|
| `get_hot_block(target, agent_id, char_limit)` | Get computed context window |

### Sessions

| Method | Description |
|--------|-------------|
| `create_session(agent_id, source, title)` | Create a new session |
| `add_messages(session_id, messages)` | Add messages to a session |
| `end_session(session_id)` | End a session |
| `get_session(session_id, include_messages)` | Get session details |

### Consolidation

| Method | Description |
|--------|-------------|
| `consolidate(scope, category, batch_size)` | Trigger memory consolidation |
| `get_consolidation_candidates(scope, ...)` | Preview consolidation candidates |

### Telepathy (Pub/Sub)

```python
# Subscribe to real-time memory changes
async for event in client.subscribe(agent_id="my-agent"):
    print(f"Event: {event.event_type} key={event.key}")
```

### Agent Management

| Method | Description |
|--------|-------------|
| `register_agent(agent_id, name, role)` | Register an agent |
| `heartbeat(agent_id)` | Keep-alive heartbeat |

### Todos

| Method | Description |
|--------|-------------|
| `add_todo(content, priority, source_agent)` | Add a todo item |
| `list_todos(include_completed)` | List todos (async iterator) |
| `complete_todo(todo_id)` | Mark a todo as completed |
| `remove_todo(todo_id)` | Delete a todo |

### Skills

| Method | Description |
|--------|-------------|
| `add_skill(name, description, ...)` | Add a skill |
| `get_skill(skill_id)` | Get a skill by ID |
| `list_skills(source_agent, category)` | List skills |
| `remove_skill(skill_id)` | Delete a skill |
| `search_skills(query, limit)` | Search skills |

### Cron Jobs

| Method | Description |
|--------|-------------|
| `create_cron_job(name, schedule, ...)` | Create a cron job |
| `list_cron_jobs(agent_id)` | List cron jobs |
| `update_cron_job(job_id, ...)` | Update a cron job |
| `delete_cron_job(job_id)` | Delete a cron job |

### Messaging

| Method | Description |
|--------|-------------|
| `send_message(platform, target, message)` | Send a message |
| `list_channels(platform)` | List available channels |
| `subscribe_to_messages(platform)` | Subscribe to incoming messages |

## Error Handling

```python
from mnemosyne.exceptions import NotFoundError, ValidationError, MnemosyneError

try:
    mem = await client.get_memory("nonexistent-id")
except NotFoundError:
    print("Memory not found")
except ValidationError as e:
    print(f"Invalid input: {e}")
except MnemosyneError as e:
    print(f"Unexpected error: {e}")
```

## LangChain Integration

Install with LangChain support:

```bash
pip install mnemosyne-sdk[langchain]
```

```python
from mnemosyne import MnemosyneClient
from mnemosyne.integrations.langchain import (
    MnemosyneMemoryTool,
    MnemosyneSearchTool,
    MnemosyneHotBlockTool,
)

client = MnemosyneClient("localhost:50051")
await client.connect()

tools = [
    MnemosyneMemoryTool(client=client),
    MnemosyneSearchTool(client=client),
    MnemosyneHotBlockTool(client=client),
]

# Use with any LangChain agent
from langchain.agents import initialize_agent
agent = initialize_agent(tools, llm, agent="zero-shot-react-description")
```

## Jupyter Notebook

See [`examples/python/quickstart.ipynb`](../../examples/python/quickstart.ipynb) for an interactive walkthrough of the full API.

## Development

```bash
cd sdk/python
pip install -e ".[dev]"
pytest tests/
```

## License

Apache 2.0 — same as the Mnemosyne protocol and client SDK.