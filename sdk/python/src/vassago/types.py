"""Pythonic data types for the Mnemosyne Python SDK.

These dataclasses wrap the raw protobuf messages, providing:
- Readable field names (snake_case)
- Default values for optional fields
- datetime conversion from Unix timestamps
- String representations for debugging
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Optional
from enum import Enum


class Target(str, Enum):
    """Memory target types."""
    MEMORY = "memory"
    USER = "user"


class Category(str, Enum):
    """Memory categories."""
    ENVIRONMENT = "environment"
    PROJECT = "project"
    QUIRK = "quirk"
    OBSERVATION = "observation"
    USER = "user"


class EventEventType(str, Enum):
    """Telepathy event types."""
    ADDED = "ADDED"
    UPDATED = "UPDATED"
    REMOVED = "REMOVED"
    HOT_BLOCK_CHANGED = "HOT_BLOCK_CHANGED"
    CRON_FIRE = "CRON_FIRE"


class ConsolidateScope(str, Enum):
    """Consolidation scope types."""
    ALL = "CONSOLIDATE_SCOPE_ALL"
    OBSERVATIONS = "CONSOLIDATE_SCOPE_OBSERVATIONS"
    CATEGORY = "CONSOLIDATE_SCOPE_CATEGORY"
    EXPIRED = "CONSOLIDATE_SCOPE_EXPIRED"


def _from_timestamp(ts: int) -> Optional[datetime]:
    """Convert Unix timestamp to datetime."""
    if ts == 0:
        return None
    return datetime.fromtimestamp(ts, tz=timezone.utc)


def _to_timestamp(dt: Optional[datetime]) -> int:
    """Convert datetime to Unix timestamp."""
    if dt is None:
        return 0
    return int(dt.timestamp())


@dataclass
class Memory:
    """A memory entry in Mnemosyne."""
    id: str
    target: str
    category: str
    key: str
    content: str
    priority: int = 3
    source_agent: str = ""
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
    accessed_at: Optional[datetime] = None
    expires_at: Optional[datetime] = None

    @classmethod
    def from_proto(cls, msg) -> Memory:
        """Create a Memory from a protobuf MemoryEntry."""
        return cls(
            id=msg.id,
            target=msg.target,
            category=msg.category,
            key=msg.key,
            content=msg.content,
            priority=msg.priority,
            source_agent=msg.source_agent,
            created_at=_from_timestamp(msg.created_at),
            updated_at=_from_timestamp(msg.updated_at),
            accessed_at=_from_timestamp(msg.accessed_at),
            expires_at=_from_timestamp(msg.expires_at),
        )

    def __str__(self) -> str:
        return f"Memory(id={self.id[:8]}..., key={self.key}, category={self.category}, priority={self.priority})"


@dataclass
class HotBlock:
    """The computed hot block — priority-sorted context for prompt injection."""
    content: str
    char_count: int
    entry_count: int
    computed_at: Optional[datetime] = None

    @classmethod
    def from_proto(cls, msg) -> HotBlock:
        """Create a HotBlock from a protobuf HotBlockResponse."""
        return cls(
            content=msg.content,
            char_count=msg.char_count,
            entry_count=msg.entry_count,
            computed_at=_from_timestamp(msg.computed_at),
        )

    def __str__(self) -> str:
        return f"HotBlock({self.entry_count} entries, {self.char_count} chars)"


@dataclass
class SessionInfo:
    """Summary information about a session (without messages)."""
    id: str
    agent_id: str
    source: str
    title: str
    created_at: Optional[datetime] = None
    last_active_at: Optional[datetime] = None
    message_count: int = 0

    @classmethod
    def from_proto(cls, msg) -> SessionInfo:
        """Create a SessionInfo from a protobuf Session."""
        return cls(
            id=msg.id,
            agent_id=msg.agent_id,
            source=msg.source,
            title=msg.title,
            created_at=_from_timestamp(msg.created_at),
            last_active_at=_from_timestamp(msg.last_active_at),
            message_count=msg.message_count,
        )


@dataclass
class ChatMessage:
    """A single message in a session."""
    role: str
    content: str
    tool_call_id: str = ""
    tool_name: str = ""
    timestamp: Optional[datetime] = None

    @classmethod
    def from_proto(cls, msg) -> ChatMessage:
        return cls(
            role=msg.role,
            content=msg.content,
            tool_call_id=msg.tool_call_id,
            tool_name=msg.tool_name,
            timestamp=_from_timestamp(msg.timestamp),
        )


@dataclass
class Session:
    """A conversation session with optional messages."""
    id: str
    agent_id: str
    source: str
    title: str
    created_at: Optional[datetime] = None
    last_active_at: Optional[datetime] = None
    message_count: int = 0
    messages: list[ChatMessage] = field(default_factory=list)

    @classmethod
    def from_proto(cls, msg) -> Session:
        return cls(
            id=msg.id,
            agent_id=msg.agent_id,
            source=msg.source,
            title=msg.title,
            created_at=_from_timestamp(msg.created_at),
            last_active_at=_from_timestamp(msg.last_active_at),
            message_count=msg.message_count,
            messages=[ChatMessage.from_proto(m) for m in msg.messages],
        )


@dataclass
class TodoItem:
    """A todo item."""
    id: str
    content: str
    priority: int = 3
    completed: bool = False
    created_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None

    @classmethod
    def from_proto(cls, msg) -> TodoItem:
        return cls(
            id=msg.id,
            content=msg.content,
            priority=msg.priority,
            completed=msg.completed,
            created_at=_from_timestamp(msg.created_at),
            completed_at=_from_timestamp(msg.completed_at),
        )


@dataclass
class Skill:
    """A skill entry."""
    id: str
    name: str
    description: str
    triggers: list[str] = field(default_factory=list)
    category: str = ""
    content: str = ""
    source_agent: str = ""
    priority: int = 3
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None

    @classmethod
    def from_proto(cls, msg) -> Skill:
        return cls(
            id=msg.id,
            name=msg.name,
            description=msg.description,
            triggers=list(msg.triggers),
            category=msg.category,
            content=msg.content,
            source_agent=msg.source_agent,
            priority=msg.priority,
            created_at=_from_timestamp(msg.created_at),
            updated_at=_from_timestamp(msg.updated_at),
        )


@dataclass
class CronJob:
    """A cron job definition."""
    id: str
    name: str
    schedule: str
    agent_id: str
    message: str
    enabled: bool = True
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
    last_fire_at: Optional[datetime] = None
    next_fire_at: Optional[datetime] = None

    @classmethod
    def from_proto(cls, msg) -> CronJob:
        return cls(
            id=msg.id,
            name=msg.name,
            schedule=msg.schedule,
            agent_id=msg.agent_id,
            message=msg.message,
            enabled=msg.enabled,
            created_at=_from_timestamp(msg.created_at),
            updated_at=_from_timestamp(msg.updated_at),
            last_fire_at=_from_timestamp(msg.last_fire_at),
            next_fire_at=_from_timestamp(msg.next_fire_at),
        )


@dataclass
class UpdateEvent:
    """A Telepathy subscription event."""
    event_type: str
    target: str
    category: str
    key: str
    agent_id: str
    memory: Optional[Memory] = None
    hot_block: Optional[HotBlock] = None

    @classmethod
    def from_proto(cls, msg) -> UpdateEvent:
        memory = Memory.from_proto(msg.entry) if msg.HasField("entry") else None
        hot_block = HotBlock.from_proto(msg.hot_block) if msg.HasField("hot_block") else None
        event_name = msg.EventType.Name(msg.event_type) if msg.event_type else ""
        return cls(
            event_type=event_name,
            target=msg.target,
            category=msg.category,
            key=msg.key,
            agent_id=msg.agent_id,
            memory=memory,
            hot_block=hot_block,
        )


@dataclass
class ConsolidationResult:
    """Result of a consolidation operation."""
    removed: int
    merged: int
    summary: str

    @classmethod
    def from_proto(cls, msg) -> ConsolidationResult:
        return cls(
            removed=msg.removed,
            merged=msg.merged,
            summary=msg.summary,
        )


@dataclass
class MessageEvent:
    """An incoming message from a messaging platform."""
    platform: str
    channel_id: str
    channel_name: str
    sender_id: str
    sender_name: str
    message: str
    timestamp: Optional[datetime] = None
    message_id: str = ""

    @classmethod
    def from_proto(cls, msg) -> MessageEvent:
        return cls(
            platform=msg.platform,
            channel_id=msg.channel_id,
            channel_name=msg.channel_name,
            sender_id=msg.sender_id,
            sender_name=msg.sender_name,
            message=msg.message,
            timestamp=_from_timestamp(msg.timestamp),
            message_id=msg.message_id,
        )


@dataclass
class Channel:
    """A messaging channel."""
    id: str
    name: str
    platform: str

    @classmethod
    def from_proto(cls, msg) -> Channel:
        return cls(id=msg.id, name=msg.name, platform=msg.platform)