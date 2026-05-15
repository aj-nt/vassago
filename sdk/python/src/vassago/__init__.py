"""Mnemosyne Python SDK — async client for the Mnemosyne memory daemon.

Usage::

    from vassago import MnemosyneClient

    async with MnemosyneClient("localhost:50051") as client:
        await client.add_memory(key="server", content="node-3.internal")
        block = await client.get_hot_block()
        print(block.content)

See https://github.com/aj-nt/vassago for full documentation.
"""

__version__ = "0.1.0"

from .client import MnemosyneClient
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
    MnemosyneError,
    ConnectionError,
    NotFoundError,
    ValidationError,
    TimeoutError,
    AuthenticationError,
    ServiceUnavailableError,
)

__all__ = [
    # Client
    "MnemosyneClient",
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
    "MnemosyneError",
    "ConnectionError",
    "NotFoundError",
    "ValidationError",
    "TimeoutError",
    "AuthenticationError",
    "ServiceUnavailableError",
]