"""Integration tests for the Mnemosyne Python SDK.

These tests require a running Mnemosyne daemon. Run with:

    pytest tests/ --live-daemon localhost:50051

They are NOT run by default — only when --live-daemon is provided.
"""

import os
import pytest
import asyncio
from datetime import datetime, timezone

from mnemosyne import MnemosyneClient, MnemosyneError, NotFoundError


# Skip all tests in this module unless --live-daemon is provided
def pytest_collection_modifyitems(config, items):
    if not config.getoption("--live-daemon", default=None):
        skip = pytest.mark.skip(reason="needs --live-daemon option to run")
        for item in items:
            item.add_marker(skip)


@pytest.fixture
def daemon_address(request):
    return request.config.getoption("--live-daemon", default="localhost:50051")


@pytest.fixture
async def client(daemon_address):
    """Provide a connected MnemosyneClient, cleaned up after each test."""
    c = MnemosyneClient(daemon_address)
    await c.connect()
    yield c
    await c.close()


class TestHealth:
    """Test daemon connectivity."""

    @pytest.mark.asyncio
    async def test_ping(self, client):
        status = await client.ping()
        assert status == "healthy"
        print(f"Daemon status: {status}")


class TestMemoryCRUD:
    """Test memory Create, Read, Update, Delete operations."""

    @pytest.mark.asyncio
    async def test_add_and_get_memory(self, client):
        mem = await client.add_memory(
            key="test-integration-key",
            content="Integration test memory content",
            category="observation",
            priority=2,
            source_agent="integration-test",
        )
        assert mem.id, "Memory should have an ID"
        assert mem.key == "test-integration-key"
        assert mem.content == "Integration test memory content"
        assert mem.category == "observation"

        # Get it back
        fetched = await client.get_memory(mem.id)
        assert fetched.id == mem.id
        assert fetched.content == "Integration test memory content"

    @pytest.mark.asyncio
    async def test_update_memory(self, client):
        mem = await client.add_memory(
            key="test-update-key",
            content="Original content",
            category="environment",
            priority=3,
        )
        updated = await client.update_memory(
            mem.id,
            content="Updated content",
            priority=5,
        )
        assert updated.content == "Updated content"
        assert updated.priority == 5

    @pytest.mark.asyncio
    async def test_remove_memory(self, client):
        mem = await client.add_memory(
            key="test-remove-key",
            content="To be removed",
        )
        deleted = await client.remove_memory(mem.id)
        assert deleted is True

    @pytest.mark.asyncio
    async def test_list_memories(self, client):
        # Add a few memories
        await client.add_memory(key="list-test-1", content="First", category="environment")
        await client.add_memory(key="list-test-2", content="Second", category="project")

        mems = await client.list_memories(target="memory")
        assert len(mems) > 0

    @pytest.mark.asyncio
    async def test_search_memories(self, client):
        await client.add_memory(
            key="search-test",
            content="The integration search test content",
            category="project",
        )
        results = await client.search_memories("integration search")
        assert len(results) > 0


class TestHotBlock:
    """Test hot block retrieval."""

    @pytest.mark.asyncio
    async def test_get_hot_block(self, client):
        await client.add_memory(
            key="hotblock-test",
            content="Hot block test content",
            category="environment",
            priority=4,
        )
        block = await client.get_hot_block(target="memory")
        assert block.content, "Hot block should not be empty"
        assert block.entry_count > 0


class TestSessions:
    """Test session management."""

    @pytest.mark.asyncio
    async def test_create_and_get_session(self, client):
        session = await client.create_session(
            agent_id="integration-test",
            source="python-test",
            title="Integration Test Session",
        )
        assert session.id, "Session should have an ID"
        assert session.title == "Integration Test Session"

        # Get it back
        fetched = await client.get_session(session.id, include_messages=False)
        assert fetched.id == session.id

    @pytest.mark.asyncio
    async def test_add_messages_and_end_session(self, client):
        from mnemosyne import ChatMessage

        session = await client.create_session(
            agent_id="integration-test",
            source="python-test",
        )
        await client.add_messages(session.id, [
            ChatMessage(role="user", content="Hello from integration test"),
            ChatMessage(role="assistant", content="Hello! I'm Mnemosyne."),
        ])

        ended = await client.end_session(session.id)
        assert ended is True


class TestTodos:
    """Test todo management."""

    @pytest.mark.asyncio
    async def test_add_and_complete_todo(self, client):
        todo = await client.add_todo(
            content="Integration test todo",
            priority=3,
        )
        assert todo.id, "Todo should have an ID"
        assert todo.content == "Integration test todo"

        completed = await client.complete_todo(todo.id)
        assert completed.completed is True