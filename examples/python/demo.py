#!/usr/bin/env python3
"""Mnemosyne Python SDK demo — add memory, read hot block, search, subscribe.

Prerequisites: pip install mnemosyne-sdk
Daemon must be running: mnemosyne start
"""

import asyncio
import sys

sys.path.insert(0, "../../sdk/python/src")  # For development

from mnemosyne import MnemosyneClient


DAEMON_ADDR = "localhost:50051"
AGENT_ID = "python-demo"


async def connect():
    """Connect to the daemon and verify health."""
    client = MnemosyneClient(DAEMON_ADDR)
    await client.connect()
    status = await client.ping()
    print(f"✓ Connected to Mnemosyne daemon (status: {status})")
    return client


async def add_memory(client):
    """Write a fact that persists across sessions."""
    mem = await client.add_memory(
        key="demo-deployment",
        content="The deploy server is at 192.168.1.100 (added by Python demo)",
        category="environment",
        priority=4,
        source_agent=AGENT_ID,
    )
    print(f"✓ Added memory: {mem}")
    return mem.id


async def read_hot_block(client):
    """Read the computed context window for this agent."""
    block = await client.get_hot_block(target="memory", agent_id=AGENT_ID)
    print(f"\n--- Hot Block ({block.char_count} chars, {block.entry_count} entries) ---")
    print(block.content[:500] + ("..." if block.char_count > 500 else ""))
    print("---\n")


async def search_memories(client, query):
    """Full-text search across all memories."""
    results = await client.search_memories(query=query, target="memory", limit=5)
    print(f"✓ Search '{query}': {len(results)} results")
    for m in results:
        print(f"  [{m.category}] {m.key}: {m.content[:60]}...")
    print()


async def subscribe_telepathy(client, duration=5):
    """Subscribe to real-time updates. Self-exclusion: you don't see your own writes."""
    print(f"✓ Subscribing as '{AGENT_ID}' for {duration}s (self-exclusion active)...")
    try:
        async for event in client.subscribe(agent_id=AGENT_ID, targets=["memory"]):
            print(f"  📡 Telepathy event: {event.event_type} from {event.agent_id[:8]}... "
                  f"[{event.category}] {event.key}")
    except Exception as e:
        print(f"  Subscription ended: {e}")


async def main():
    client = await connect()

    try:
        # 1. Add a memory
        mem_id = await add_memory(client)

        # 2. Read the hot block (includes our new memory)
        await read_hot_block(client)

        # 3. Search
        await search_memories(client, "deploy")

        # 4. List all memories
        all_mems = await client.list_memories()
        print(f"✓ Total memories: {len(all_mems)}")
        for m in all_mems[:5]:
            print(f"  [{m.category}] {m.key} (priority={m.priority})")

        # 5. Subscribe (5 seconds)
        await subscribe_telepathy(client, duration=5)

    finally:
        await client.close()


if __name__ == "__main__":
    asyncio.run(main())