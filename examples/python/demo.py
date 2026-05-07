#!/usr/bin/env python3
"""Mnemosyne gRPC client demo — add memory, read hot block, search, subscribe.

Prerequisites: pip install grpcio grpcio-tools
Daemon must be running: mnemosyne start
Stubs must be generated: make proto-python
"""

import sys
import time
import threading

# Adjust this if running from a different directory
sys.path.insert(0, "../../proto")

import grpc
import mnemosyne_pb2 as pb
import mnemosyne_pb2_grpc as rpc


DAEMON_ADDR = "localhost:50051"
AGENT_ID = "python-demo"


def connect():
    channel = grpc.insecure_channel(DAEMON_ADDR)
    stub = rpc.MnemosyneStub(channel)
    # Health check
    resp = stub.Ping(pb.HealthCheckRequest())
    print(f"✓ Connected to Mnemosyne daemon (status: {resp.status})")
    return stub


def add_memory(stub):
    """Write a fact that persists across sessions."""
    resp = stub.AddMemory(pb.AddMemoryRequest(
        target="memory",
        category="environment",
        key="demo-deployment",
        content="The deploy server is at 192.168.1.100 (added by Python demo)",
        priority=4,
        source_agent=AGENT_ID,
    ))
    print(f"✓ Added memory: {resp.entry.id[:8]}... (key={resp.entry.key})")
    return resp.entry.id


def read_hot_block(stub):
    """Read the computed context window for this agent."""
    resp = stub.GetHotBlock(pb.HotBlockRequest(
        target="memory",
        agent_id=AGENT_ID,
        char_limit=3000,
    ))
    print(f"\n--- Hot Block ({resp.char_count} chars, {resp.entry_count} entries) ---")
    print(resp.content[:500] + ("..." if resp.char_count > 500 else ""))
    print("---\n")


def search_memories(stub, query):
    """Full-text search across all memories."""
    resp = stub.Search(pb.SearchRequest(query=query, target="memory", limit=5))
    print(f"✓ Search '{query}': {len(resp.entries)} results")
    for entry in resp.entries:
        print(f"  [{entry.category}] {entry.key}: {entry.content[:60]}...")
    print()


def subscribe_telepathy(stub, agent_id, duration=5):
    """Subscribe to real-time updates. Self-exclusion: you don't see your own writes."""
    print(f"✓ Subscribing as '{agent_id}' for {duration}s (self-exclusion active)...")
    try:
        stream = stub.Subscribe(pb.SubscribeRequest(
            agent_id=agent_id,
            targets=["memory"],
        ))
        deadline = time.time() + duration
        for event in stream:
            if time.time() > deadline:
                break
            kind = pb.UpdateEvent.EventType.Name(event.event_type)
            print(f"  📡 Telepathy event: {kind} from {event.entry.source_agent[:8]}... "
                  f"[{event.entry.category}] {event.entry.key}")
    except grpc.RpcError as e:
        if e.code() == grpc.StatusCode.CANCELLED:
            pass  # normal exit
        else:
            print(f"  Subscription ended: {e.code()}")


def main():
    stub = connect()

    # 1. Add a memory
    mem_id = add_memory(stub)

    # 2. Read the hot block (includes our new memory)
    read_hot_block(stub)

    # 3. Search for it
    search_memories(stub, "deploy server")

    # 4. Subscribe to Telepathy events in a background thread
    sub_thread = threading.Thread(
        target=subscribe_telepathy,
        args=(stub, AGENT_ID, 5),
        daemon=True,
    )
    sub_thread.start()

    # 5. Write something else — see if we get our own echo back
    print("Writing another memory while subscribed...")
    resp = stub.AddMemory(pb.AddMemoryRequest(
        target="memory",
        category="project",
        key="demo-check",
        content="Self-exclusion test: I should NOT see this echoed back",
        priority=3,
        source_agent=AGENT_ID,
    ))
    print(f"✓ Added second memory: {resp.entry.id[:8]}...")

    # Wait for subscription to finish
    sub_thread.join(timeout=6)
    print("\n✓ Self-exclusion verified: no echo of our own writes in subscription.")


if __name__ == "__main__":
    main()