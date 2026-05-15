"""Vassago Python SDK — async client for the Vassago memory daemon.

Usage::

    from vassago import VassagoClient

    async with VassagoClient("localhost:50051") as client:
        await client.add_memory(key="server", content="node-3.internal")
        block = await client.get_hot_block()
        print(block.content)

See https://github.com/aj-nt/vassago for full documentation.
"""

__version__ = "0.1.0"

from .client import VassagoClient
from .types import (
    Memory,
    HotBlock,
    Session,
    SessionInfo,
    ChatMessage,
    TodoItem,
    Skill,
    CronJob,
    UpdateEvent,
    ConsolidationResult,
    MessageEvent,
    Channel,
    ConsolidateScope,
    Target,
    Category,
    EventEventType,
)
from .exceptions import (
    VassagoError,
    ConnectionError,
    NotFoundError,
    ValidationError,
    TimeoutError,
    AuthenticationError,
    ServiceUnavailableError,
)

__all__ = [
    # Client
    "VassagoClient",
    # Types
    "Memory",
    "HotBlock",
    "Session",
    "SessionInfo",
    "ChatMessage",
    "TodoItem",
    "Skill",
    "CronJob",
    "UpdateEvent",
    "ConsolidationResult",
    "MessageEvent",
    "Channel",
    "ConsolidateScope",
    "Target",
    "Category",
    "EventEventType",
    # Exceptions
    "VassagoError",
    "ConnectionError",
    "NotFoundError",
    "ValidationError",
    "TimeoutError",
    "AuthenticationError",
    "ServiceUnavailableError",
]