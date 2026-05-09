"""Tests for Mnemosyne Python SDK client and types."""

import pytest
import asyncio
from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock, patch

# We test against the type conversions and client structure
# without requiring a running daemon (unit tests only).
# Integration tests run separately against a live daemon.

from mnemosyne.types import (
    Memory, HotBlock, Session, SessionInfo, ChatMessage,
    TodoItem, Skill, CronJob, UpdateEvent, ConsolidationResult,
    MessageEvent, Channel, ConsolidateScope,
    _from_timestamp, _to_timestamp,
)
from mnemosyne.exceptions import (
    MnemosyneError, NotFoundError, ValidationError,
    ConnectionError, TimeoutError,
)
from mnemosyne.client import MnemosyneClient


# ------------------------------------------------------------------ #
# Types tests
# ------------------------------------------------------------------ #

class TestTimestampConversion:
    """Test Unix timestamp ↔ datetime conversion."""

    def test_from_timestamp_zero(self):
        assert _from_timestamp(0) is None

    def test_from_timestamp_positive(self):
        dt = _from_timestamp(1700000000)
        assert dt is not None
        assert dt.year >= 2023

    def test_to_timestamp_none(self):
        assert _to_timestamp(None) == 0

    def test_roundtrip(self):
        now = datetime(2024, 1, 15, 12, 0, 0, tzinfo=timezone.utc)
        ts = _to_timestamp(now)
        dt = _from_timestamp(ts)
        assert dt is not None
        assert dt.year == 2024
        assert dt.month == 1


class TestMemory:
    """Test Memory dataclass."""

    def test_from_proto(self):
        mock = MagicMock()
        mock.id = "abc123"
        mock.target = "memory"
        mock.category = "environment"
        mock.key = "deploy-server"
        mock.content = "node-3.internal"
        mock.priority = 4
        mock.source_agent = "python-sdk"
        mock.created_at = 1700000000
        mock.updated_at = 1700000001
        mock.accessed_at = 0
        mock.expires_at = 0

        mem = Memory.from_proto(mock)
        assert mem.id == "abc123"
        assert mem.target == "memory"
        assert mem.key == "deploy-server"
        assert mem.priority == 4
        assert mem.created_at is not None
        assert mem.accessed_at is None

    def test_str_representation(self):
        mem = Memory(
            id="abc123def456",
            target="memory",
            category="environment",
            key="deploy-server",
            content="node-3.internal",
            priority=4,
        )
        s = str(mem)
        assert "abc123de" in s
        assert "deploy-server" in s


class TestHotBlock:
    def test_from_proto(self):
        mock = MagicMock()
        mock.content = "Deploy server: node-3.internal"
        mock.char_count = 28
        mock.entry_count = 1
        mock.computed_at = 1700000000

        hb = HotBlock.from_proto(mock)
        assert hb.char_count == 28
        assert hb.entry_count == 1
        assert "28" in str(hb)


class TestSessionInfo:
    def test_from_proto(self):
        mock = MagicMock()
        mock.id = "sess-123"
        mock.agent_id = "python-sdk"
        mock.source = "python"
        mock.title = "Test session"
        mock.created_at = 1700000000
        mock.last_active_at = 1700000001
        mock.message_count = 5

        si = SessionInfo.from_proto(mock)
        assert si.id == "sess-123"
        assert si.message_count == 5


class TestChatMessage:
    def test_from_proto(self):
        mock = MagicMock()
        mock.role = "user"
        mock.content = "Hello"
        mock.tool_call_id = ""
        mock.tool_name = ""
        mock.timestamp = 1700000000

        msg = ChatMessage.from_proto(mock)
        assert msg.role == "user"
        assert msg.content == "Hello"


class TestTodoItem:
    def test_from_proto(self):
        mock = MagicMock()
        mock.id = "todo-1"
        mock.content = "Fix bug"
        mock.priority = 3
        mock.completed = False
        mock.created_at = 1700000000
        mock.completed_at = 0

        todo = TodoItem.from_proto(mock)
        assert todo.id == "todo-1"
        assert todo.completed is False


class TestSkill:
    def test_from_proto(self):
        mock = MagicMock()
        mock.id = "skill-1"
        mock.name = "deploy"
        mock.description = "Deploy the app"
        mock.triggers = ["deploy", "release"]
        mock.category = "devops"
        mock.content = "Deploy steps..."
        mock.source_agent = "python-sdk"
        mock.priority = 3
        mock.created_at = 1700000000
        mock.updated_at = 1700000001

        skill = Skill.from_proto(mock)
        assert skill.name == "deploy"
        assert skill.triggers == ["deploy", "release"]


class TestCronJob:
    def test_from_proto(self):
        mock = MagicMock()
        mock.id = "cron-1"
        mock.name = "Daily check"
        mock.schedule = "0 9 * * *"
        mock.agent_id = "python-sdk"
        mock.message = "Run daily check"
        mock.enabled = True
        mock.created_at = 1700000000
        mock.updated_at = 0
        mock.last_fire_at = 0
        mock.next_fire_at = 1700003600

        job = CronJob.from_proto(mock)
        assert job.name == "Daily check"
        assert job.schedule == "0 9 * * *"
        assert job.enabled is True


class TestUpdateEvent:
    def test_from_proto(self):
        mock = MagicMock()
        mock.event_type = 1  # ADDED
        mock.target = "memory"
        mock.category = "environment"
        mock.key = "deploy-server"
        mock.agent_id = "python-sdk"
        mock.HasField = lambda name: name == "entry"

        entry_mock = MagicMock()
        entry_mock.id = "mem-1"
        entry_mock.target = "memory"
        entry_mock.category = "environment"
        entry_mock.key = "deploy-server"
        entry_mock.content = "node-3"
        entry_mock.priority = 4
        entry_mock.source_agent = "python-sdk"
        entry_mock.created_at = 1700000000
        entry_mock.updated_at = 0
        entry_mock.accessed_at = 0
        entry_mock.expires_at = 0
        mock.entry = entry_mock
        mock.hot_block = MagicMock()
        mock.HasField = lambda name: name == "entry"

        event = UpdateEvent.from_proto(mock)
        assert event.target == "memory"
        assert event.key == "deploy-server"
        assert event.memory is not None
        assert event.memory.id == "mem-1"


class TestConsolidateScope:
    def test_values(self):
        assert ConsolidateScope.ALL.value == "CONSOLIDATE_SCOPE_ALL"
        assert ConsolidateScope.OBSERVATIONS.value == "CONSOLIDATE_SCOPE_OBSERVATIONS"


class TestMessageEvent:
    def test_from_proto(self):
        mock = MagicMock()
        mock.platform = "discord"
        mock.channel_id = "123456"
        mock.channel_name = "general"
        mock.sender_id = "user-1"
        mock.sender_name = "Alice"
        mock.message = "Hello!"
        mock.timestamp = 1700000000
        mock.message_id = "msg-1"

        event = MessageEvent.from_proto(mock)
        assert event.platform == "discord"
        assert event.message == "Hello!"


class TestChannel:
    def test_from_proto(self):
        mock = MagicMock()
        mock.id = "123"
        mock.name = "general"
        mock.platform = "discord"

        ch = Channel.from_proto(mock)
        assert ch.name == "general"
        assert ch.platform == "discord"


# ------------------------------------------------------------------ #
# Exception tests
# ------------------------------------------------------------------ #

class TestExceptions:
    def test_hierarchy(self):
        assert issubclass(NotFoundError, MnemosyneError)
        assert issubclass(ValidationError, MnemosyneError)
        assert issubclass(ConnectionError, MnemosyneError)
        assert issubclass(TimeoutError, MnemosyneError)

    def test_raise_catch(self):
        with pytest.raises(NotFoundError):
            raise NotFoundError("memory not found")


# ------------------------------------------------------------------ #
# Client tests (structure only, no network)
# ------------------------------------------------------------------ #

class TestMnemosyneClient:
    """Test client construction and configuration (no network calls)."""

    def test_default_address(self):
        client = MnemosyneClient()
        assert client.address == "localhost:50051"
        assert client.timeout == 30.0

    def test_custom_address(self):
        client = MnemosyneClient("192.168.1.100:50051", timeout=10.0)
        assert client.address == "192.168.1.100:50051"
        assert client.timeout == 10.0

    def test_api_key(self):
        client = MnemosyneClient(api_key="sk-test-key")
        assert client.api_key == "sk-test-key"
        meta = client._metadata()
        assert ("x-api-key", "sk-test-key") in meta

    def test_no_api_key_metadata(self):
        client = MnemosyneClient()
        assert client._metadata() == []

    def test_context_manager_protocol(self):
        client = MnemosyneClient()
        assert hasattr(client, "__aenter__")
        assert hasattr(client, "__aexit__")


# ------------------------------------------------------------------ #
# Integration test fixture (requires running daemon)
# ------------------------------------------------------------------ #

# To run integration tests against a live daemon:
#   pytest tests/ --live-daemon localhost:50051
#
# These are NOT run by default — they require a running Mnemosyne daemon.

@pytest.fixture
def live_daemon(request):
    """Fixture for integration tests against a live daemon."""
    return request.config.getoption("--live-daemon", default=None)


def pytest_addoption(parser):
    parser.addoption(
        "--live-daemon",
        action="store",
        default=None,
        help="Address of live Mnemosyne daemon for integration tests",
    )