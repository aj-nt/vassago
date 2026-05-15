"""
LangChain tool wrapper for Vassago.

Wraps the Vassago Python SDK as LangChain tools so agents using
LangChain can directly access persistent memory.

Usage::

    from vassago.integrations.langchain import (
        VassagoMemoryTool,
        VassagoSearchTool,
        VassagoHotBlockTool,
    )
    from vassago import VassagoClient

    client = VassagoClient("localhost:50051")
    tools = [
        VassagoMemoryTool(client),
        VassagoSearchTool(client),
        VassagoHotBlockTool(client),
    ]

Requires: langchain-core >= 0.1.0
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Optional

try:
    from langchain_core.tools import BaseTool
    from langchain_core.callbacks import CallbackManagerForToolRun
except ImportError:
    raise ImportError(
        "langchain-core is required for Vassago LangChain integration. "
        "Install with: pip install vassago-sdk[langchain]"
    )

if TYPE_CHECKING:
    from vassago import VassagoClient


class VassagoMemoryTool(BaseTool):
    """LangChain tool for adding memories to Vassago."""

    name: str = "vassago_add_memory"
    description: str = (
        "Save a fact or memory to persistent storage. "
        "Use this when you learn something important that should be remembered "
        "across conversations. Provide a key (short identifier), content (the fact), "
        "and optionally a category (environment, project, quirk, observation, user) "
        "and priority (1-5, higher = more important)."
    )
    client: object = None  # VassagoClient

    class Config:
        arbitrary_types_allowed = True

    def _run(
        self,
        key: str,
        content: str,
        category: str = "observation",
        priority: int = 3,
        target: str = "memory",
        run_manager: Optional[CallbackManagerForToolRun] = None,
    ) -> str:
        """Add a memory synchronously (requires async client — use arun instead)."""
        raise NotImplementedError("Use arun() for async Vassago client")

    async def _arun(
        self,
        key: str,
        content: str,
        category: str = "observation",
        priority: int = 3,
        target: str = "memory",
        run_manager: Optional[CallbackManagerForToolRun] = None,
    ) -> str:
        """Add a memory asynchronously."""
        mem = await self.client.add_memory(
            key=key,
            content=content,
            category=category,
            priority=priority,
            target=target,
        )
        return f"Memory saved: {mem.key} (id={mem.id[:8]}..., category={mem.category}, priority={mem.priority})"


class VassagoSearchTool(BaseTool):
    """LangChain tool for searching memories in Vassago."""

    name: str = "vassago_search"
    description: str = (
        "Search persistent memory for relevant facts. "
        "Use this when you need to recall information from previous conversations "
        "or look up stored knowledge. Provide a search query."
    )
    client: object = None  # VassagoClient

    class Config:
        arbitrary_types_allowed = True

    def _run(
        self,
        query: str,
        target: str = "memory",
        limit: int = 5,
        run_manager: Optional[CallbackManagerForToolRun] = None,
    ) -> str:
        raise NotImplementedError("Use arun() for async Vassago client")

    async def _arun(
        self,
        query: str,
        target: str = "memory",
        limit: int = 5,
        run_manager: Optional[CallbackManagerForToolRun] = None,
    ) -> str:
        """Search memories asynchronously."""
        results = await self.client.search_memories(
            query=query,
            target=target,
            limit=limit,
        )
        if not results:
            return f"No memories found matching '{query}'"
        lines = [f"Found {len(results)} memories:"]
        for m in results:
            lines.append(f"  [{m.category}] {m.key}: {m.content}")
        return "\n".join(lines)


class VassagoHotBlockTool(BaseTool):
    """LangChain tool for retrieving the hot block from Vassago."""

    name: str = "vassago_hot_block"
    description: str = (
        "Retrieve the current hot block — a priority-sorted summary of the most "
        "relevant context from persistent memory. Use this at the start of a "
        "conversation to load relevant context."
    )
    client: object = None  # VassagoClient

    class Config:
        arbitrary_types_allowed = True

    def _run(
        self,
        target: str = "memory",
        run_manager: Optional[CallbackManagerForToolRun] = None,
    ) -> str:
        raise NotImplementedError("Use arun() for async Vassago client")

    async def _arun(
        self,
        target: str = "memory",
        run_manager: Optional[CallbackManagerForToolRun] = None,
    ) -> str:
        """Get hot block asynchronously."""
        block = await self.client.get_hot_block(target=target)
        if not block.content:
            return "Hot block is empty — no relevant context available."
        return block.content