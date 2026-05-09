"""Async-first Python client for the Mnemosyne memory daemon.

Usage::

    from mnemosyne import MnemosyneClient

    async with MnemosyneClient("localhost:50051") as client:
        mem = await client.add_memory(
            key="deploy-server",
            content="The deploy server is at node-3.internal",
            category="environment",
            priority=4,
        )
        block = await client.get_hot_block(target="memory")
        print(block.content)
"""

from __future__ import annotations

import logging
from typing import AsyncIterator, Optional

import grpc
from grpc import aio

from .types import (
    Memory, HotBlock, Session, SessionInfo, ChatMessage,
    TodoItem, Skill, CronJob, UpdateEvent, ConsolidationResult,
    MessageEvent, Channel, ConsolidateScope,
    _to_timestamp,
)
from .exceptions import (
    MnemosyneError, ConnectionError, NotFoundError,
    ValidationError, TimeoutError, AuthenticationError,
    ServiceUnavailableError,
)
from ._proto import mnemosyne_pb2 as pb
from ._proto import mnemosyne_pb2_grpc as rpc
from ._proto import messaging_pb2 as mpb
from ._proto import messaging_pb2_grpc as mrpc

logger = logging.getLogger(__name__)

# gRPC status code → exception mapping
_STATUS_MAP = {
    grpc.StatusCode.NOT_FOUND: NotFoundError,
    grpc.StatusCode.INVALID_ARGUMENT: ValidationError,
    grpc.StatusCode.UNAUTHENTICATED: AuthenticationError,
    grpc.StatusCode.UNAVAILABLE: ServiceUnavailableError,
    grpc.StatusCode.DEADLINE_EXCEEDED: TimeoutError,
}


def _handle_error(exc: grpc.aio.AioRpcError) -> None:
    """Convert gRPC errors to Python exceptions."""
    cls = _STATUS_MAP.get(exc.code(), MnemosyneError)
    raise cls(f"{exc.code()}: {exc.details()}") from exc


class MnemosyneClient:
    """Async Python client for the Mnemosyne memory daemon.

    Args:
        address: Host:port or Unix socket path (e.g. "localhost:50051").
        api_key: Optional API key for authentication.
        timeout: Default timeout in seconds for unary RPCs.
    """

    def __init__(
        self,
        address: str = "localhost:50051",
        api_key: Optional[str] = None,
        timeout: float = 30.0,
    ):
        self.address = address
        self.api_key = api_key
        self.timeout = timeout
        self._channel: Optional[aio.Channel] = None
        self._stub: Optional[rpc.MnemosyneStub] = None
        self._messaging_stub: Optional[mrpc.MessagingStub] = None

    async def __aenter__(self) -> MnemosyneClient:
        await self.connect()
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        await self.close()

    async def connect(self) -> None:
        """Connect to the Mnemosyne daemon."""
        if self._channel is not None:
            return

        if self.address.startswith("/"):
            # Unix socket
            self._channel = aio.insecure_channel(f"unix://{self.address}")
        elif self.address.endswith(".sock") or ".sock:" in self.address:
            self._channel = aio.insecure_channel(f"unix://{self.address}")
        else:
            self._channel = aio.insecure_channel(self.address)

        self._stub = rpc.MnemosyneStub(self._channel)
        self._messaging_stub = mrpc.MessagingStub(self._channel)

    async def close(self) -> None:
        """Close the connection."""
        if self._channel is not None:
            await self._channel.close()
            self._channel = None
            self._stub = None
            self._messaging_stub = None

    def _metadata(self) -> list[tuple[str, str]]:
        """Build gRPC metadata (API key if set)."""
        if self.api_key:
            return [("x-api-key", self.api_key)]
        return []

    async def _call(self, method, request, timeout: Optional[float] = None):
        """Call a unary RPC with error handling."""
        try:
            return await method(
                request,
                metadata=self._metadata(),
                timeout=timeout or self.timeout,
            )
        except grpc.aio.AioRpcError as e:
            _handle_error(e)

    # ------------------------------------------------------------------ #
    # Health
    # ------------------------------------------------------------------ #

    async def ping(self) -> str:
        """Health check. Returns status string."""
        resp = await self._call(self._stub.Ping, pb.HealthCheckRequest())
        status_name = pb.HealthCheckResponse.ServingStatus.Name(resp.status)
        return status_name

    # ------------------------------------------------------------------ #
    # Memory CRUD
    # ------------------------------------------------------------------ #

    async def add_memory(
        self,
        key: str,
        content: str,
        category: str = "environment",
        target: str = "memory",
        priority: int = 3,
        source_agent: str = "python-sdk",
        expires_at: Optional[int] = None,
    ) -> Memory:
        """Add or update a memory entry.

        Args:
            key: Unique key for the memory (upsert by target+key).
            content: The memory content text.
            category: Memory category (environment, project, quirk, observation, user).
            target: "memory" or "user".
            priority: Priority 1-5 (higher = more important).
            source_agent: Agent ID that wrote this memory.
            expires_at: Unix timestamp for expiry (0 = no expiry).

        Returns:
            The created Memory entry.
        """
        req = pb.AddMemoryRequest(
            target=target,
            category=category,
            key=key,
            content=content,
            priority=priority,
            source_agent=source_agent,
            expires_at=expires_at or 0,
        )
        resp = await self._call(self._stub.AddMemory, req)
        return Memory.from_proto(resp.entry)

    async def get_memory(self, memory_id: str) -> Memory:
        """Get a memory by ID.

        Args:
            memory_id: The memory UUID.

        Returns:
            The Memory entry.

        Raises:
            NotFoundError: If the memory doesn't exist.
        """
        req = pb.GetMemoryRequest(id=memory_id)
        resp = await self._call(self._stub.GetMemory, req)
        return Memory.from_proto(resp)

    async def update_memory(
        self,
        memory_id: str,
        content: Optional[str] = None,
        priority: Optional[int] = None,
        category: Optional[str] = None,
        expires_at: Optional[int] = None,
    ) -> Memory:
        """Update a memory entry (partial update).

        Args:
            memory_id: The memory UUID.
            content: New content (optional).
            priority: New priority (optional).
            category: New category (optional).
            expires_at: New expiry timestamp (optional).

        Returns:
            The updated Memory entry.
        """
        req = pb.UpdateMemoryRequest(id=memory_id)
        if content is not None:
            req.content = content
        if priority is not None:
            req.priority = priority
        if category is not None:
            req.category = category
        if expires_at is not None:
            req.expires_at = expires_at
        resp = await self._call(self._stub.UpdateMemory, req)
        return Memory.from_proto(resp)

    async def remove_memory(self, memory_id: str) -> bool:
        """Remove a memory by ID.

        Args:
            memory_id: The memory UUID.

        Returns:
            True if the memory was deleted.
        """
        req = pb.RemoveMemoryRequest(id=memory_id)
        resp = await self._call(self._stub.RemoveMemory, req)
        return resp.deleted

    async def list_memories(
        self,
        target: str = "",
        category: Optional[str] = None,
        source_agent: Optional[str] = None,
        min_priority: Optional[int] = None,
    ) -> list[Memory]:
        """List memories with optional filters.

        Args:
            target: "memory", "user", or "" for both.
            category: Filter by category.
            source_agent: Filter by source agent.
            min_priority: Minimum priority filter.

        Returns:
            List of Memory entries.
        """
        req = pb.ListMemoriesRequest(target=target)
        if category is not None:
            req.category = category
        if source_agent is not None:
            req.source_agent = source_agent
        if min_priority is not None:
            req.min_priority = min_priority
        resp = await self._call(self._stub.ListMemories, req)
        return [Memory.from_proto(e) for e in resp.entries]

    # ------------------------------------------------------------------ #
    # Search
    # ------------------------------------------------------------------ #

    async def search_memories(
        self,
        query: str,
        target: str = "memory",
        limit: int = 10,
    ) -> list[Memory]:
        """Full-text search across memories.

        Args:
            query: FTS5 search query.
            target: "memory", "user", or "" for both.
            limit: Maximum results to return.

        Returns:
            List of matching Memory entries.
        """
        req = pb.SearchRequest(query=query, target=target, limit=limit)
        resp = await self._call(self._stub.Search, req)
        return [Memory.from_proto(e) for e in resp.entries]

    async def search_sessions(
        self,
        query: str,
        limit: int = 10,
    ) -> list[SessionInfo]:
        """Search past conversation sessions.

        Args:
            query: Search query.
            limit: Maximum results.

        Returns:
            List of SessionInfo entries.
        """
        req = pb.SearchSessionsRequest(query=query, limit=limit)
        resp = await self._call(self._stub.SearchSessions, req)
        return [SessionInfo.from_proto(s) for s in resp.sessions]

    async def list_sessions(self, limit: int = 20) -> list[SessionInfo]:
        """List recent sessions.

        Args:
            limit: Maximum number of sessions to return.

        Returns:
            List of SessionInfo entries.
        """
        req = pb.ListSessionsRequest(limit=limit)
        resp = await self._call(self._stub.ListSessions, req)
        return [SessionInfo.from_proto(s) for s in resp.sessions]

    # ------------------------------------------------------------------ #
    # Hot Block
    # ------------------------------------------------------------------ #

    async def get_hot_block(
        self,
        target: str = "memory",
        agent_id: str = "",
        char_limit: int = 8000,
    ) -> HotBlock:
        """Get the computed hot block — priority-sorted context for prompt injection.

        Args:
            target: "memory" or "user".
            agent_id: Agent ID for self-exclusion in Telepathy.
            char_limit: Maximum characters in the hot block.

        Returns:
            The HotBlock with formatted content.
        """
        req = pb.HotBlockRequest(
            target=target,
            agent_id=agent_id,
            char_limit=char_limit,
        )
        resp = await self._call(self._stub.GetHotBlock, req)
        return HotBlock.from_proto(resp)

    # ------------------------------------------------------------------ #
    # Sessions
    # ------------------------------------------------------------------ #

    async def create_session(
        self,
        agent_id: str = "python-sdk",
        source: str = "python",
        title: str = "",
    ) -> Session:
        """Create a new conversation session.

        Args:
            agent_id: Agent ID for this session.
            source: Source identifier (e.g. "python", "jupyter", "cron").
            title: Session title (auto-generated if empty).

        Returns:
            The created Session.
        """
        req = pb.CreateSessionRequest(
            agent_id=agent_id,
            source=source,
            title=title,
        )
        resp = await self._call(self._stub.CreateSession, req)
        return Session.from_proto(resp)

    async def add_messages(
        self,
        session_id: str,
        messages: list[ChatMessage],
    ) -> Session:
        """Add messages to a session.

        Args:
            session_id: The session UUID.
            messages: List of ChatMessage objects to add.

        Returns:
            The updated Session.
        """
        proto_msgs = []
        for m in messages:
            proto_msgs.append(pb.Message(
                role=m.role,
                content=m.content,
                tool_call_id=m.tool_call_id,
                tool_name=m.tool_name,
                timestamp=_to_timestamp(m.timestamp) or 0,
            ))
        req = pb.AddMessagesRequest(session_id=session_id, messages=proto_msgs)
        resp = await self._call(self._stub.AddMessages, req)
        return Session.from_proto(resp)

    async def end_session(self, session_id: str) -> bool:
        """End a conversation session.

        Args:
            session_id: The session UUID.

        Returns:
            True if the session was ended.
        """
        req = pb.EndSessionRequest(session_id=session_id)
        resp = await self._call(self._stub.EndSession, req)
        return resp.ended

    async def get_session(
        self,
        session_id: str,
        include_messages: bool = False,
    ) -> Session:
        """Get a session by ID.

        Args:
            session_id: The session UUID.
            include_messages: Whether to include messages in the response.

        Returns:
            The Session (with or without messages).
        """
        req = pb.GetSessionRequest(
            session_id=session_id,
            include_messages=include_messages,
        )
        resp = await self._call(self._stub.GetSession, req)
        return Session.from_proto(resp)

    # ------------------------------------------------------------------ #
    # Consolidation
    # ------------------------------------------------------------------ #

    async def consolidate(
        self,
        scope: ConsolidateScope = ConsolidateScope.OBSERVATIONS,
        category: Optional[str] = None,
        batch_size: Optional[int] = None,
    ) -> ConsolidationResult:
        """Trigger memory consolidation.

        Args:
            scope: What to consolidate (ALL, OBSERVATIONS, CATEGORY, EXPIRED).
            category: Required when scope is CATEGORY.
            batch_size: Maximum candidates to process.

        Returns:
            ConsolidationResult with removed/merged counts.
        """
        scope_enum = pb.ConsolidateScope.Value(scope.value)
        req = pb.ConsolidateRequest(scope=scope_enum)
        if category is not None:
            req.category = category
        if batch_size is not None:
            req.batch_size = batch_size
        resp = await self._call(self._stub.Consolidate, req)
        return ConsolidationResult.from_proto(resp)

    async def get_consolidation_candidates(
        self,
        scope: ConsolidateScope = ConsolidateScope.OBSERVATIONS,
        category: Optional[str] = None,
        batch_size: Optional[int] = None,
    ) -> list[tuple[Memory, str]]:
        """Get consolidation candidates without removing them.

        Args:
            scope: What scope to check.
            category: Required when scope is CATEGORY.
            batch_size: Maximum candidates to return.

        Returns:
            List of (Memory, reason) tuples.
        """
        scope_enum = pb.ConsolidateScope.Value(scope.value)
        req = pb.GetConsolidationCandidatesRequest(scope=scope_enum)
        if category is not None:
            req.category = category
        if batch_size is not None:
            req.batch_size = batch_size
        resp = await self._call(self._stub.GetConsolidationCandidates, req)
        return [(Memory.from_proto(c.entry), c.reason) for c in resp.candidates]

    # ------------------------------------------------------------------ #
    # Telepathy (Subscribe)
    # ------------------------------------------------------------------ #

    async def subscribe(
        self,
        agent_id: str,
        targets: list[str] | None = None,
        categories: list[str] | None = None,
    ) -> AsyncIterator[UpdateEvent]:
        """Subscribe to real-time memory change events (Telepathy).

        This is a server-streaming RPC. Use with ``async for``::

            async with MnemosyneClient() as client:
                async for event in client.subscribe(agent_id="my-agent"):
                    print(f"Event: {event.event_type} key={event.key}")

        Args:
            agent_id: Your agent ID (for self-exclusion).
            targets: List of targets to subscribe to (["memory", "user"]).
            categories: List of categories to filter by (empty = all).

        Yields:
            UpdateEvent objects as they arrive.
        """
        req = pb.SubscribeRequest(
            agent_id=agent_id,
            targets=targets or ["memory"],
            categories=categories or [],
        )
        try:
            stream = self._stub.Subscribe(req, metadata=self._metadata())
            async for event_msg in stream:
                yield UpdateEvent.from_proto(event_msg)
        except grpc.aio.AioRpcError as e:
            _handle_error(e)

    # ------------------------------------------------------------------ #
    # Agent Management
    # ------------------------------------------------------------------ #

    async def register_agent(
        self,
        agent_id: str,
        name: str = "",
        role: str = "",
    ) -> tuple[str, bool]:
        """Register an agent with the daemon.

        Args:
            agent_id: Agent identifier.
            name: Human-readable agent name.
            role: Agent role description.

        Returns:
            Tuple of (agent_id, is_new) — is_new is True if first registration.
        """
        req = pb.RegisterAgentRequest(agent_id=agent_id, name=name, role=role)
        resp = await self._call(self._stub.RegisterAgent, req)
        return (resp.agent_id, resp.is_new)

    async def heartbeat(self, agent_id: str) -> None:
        """Send a heartbeat to keep agent registration alive.

        Args:
            agent_id: The agent's registered ID.
        """
        req = pb.AgentId(id=agent_id)
        await self._call(self._stub.Heartbeat, req)

    # ------------------------------------------------------------------ #
    # Todos
    # ------------------------------------------------------------------ #

    async def add_todo(
        self,
        content: str,
        priority: int = 3,
        source_agent: str = "python-sdk",
    ) -> TodoItem:
        """Add a todo item.

        Args:
            content: Todo description.
            priority: Priority 1-5.
            source_agent: Agent creating the todo.

        Returns:
            The created TodoItem.
        """
        req = pb.AddTodoRequest(
            content=content,
            priority=priority,
            source_agent=source_agent,
        )
        resp = await self._call(self._stub.AddTodo, req)
        return TodoItem.from_proto(resp)

    async def list_todos(
        self,
        include_completed: bool = False,
        source_agent: str = "",
    ) -> AsyncIterator[TodoItem]:
        """List todo items (server-streaming).

        Args:
            include_completed: Include completed todos.
            source_agent: Filter by agent.

        Yields:
            TodoItem objects.
        """
        req = pb.ListTodosRequest(
            include_completed=include_completed,
            source_agent=source_agent,
        )
        try:
            stream = self._stub.ListTodos(req, metadata=self._metadata())
            async for item_msg in stream:
                yield TodoItem.from_proto(item_msg)
        except grpc.aio.AioRpcError as e:
            _handle_error(e)

    async def complete_todo(self, todo_id: str) -> TodoItem:
        """Mark a todo as completed.

        Args:
            todo_id: The todo UUID.

        Returns:
            The updated TodoItem.
        """
        req = pb.CompleteTodoRequest(id=todo_id)
        resp = await self._call(self._stub.CompleteTodo, req)
        return TodoItem.from_proto(resp)

    async def remove_todo(self, todo_id: str) -> None:
        """Remove a todo item.

        Args:
            todo_id: The todo UUID.
        """
        req = pb.RemoveTodoRequest(id=todo_id)
        await self._call(self._stub.RemoveTodo, req)

    # ------------------------------------------------------------------ #
    # Skills
    # ------------------------------------------------------------------ #

    async def add_skill(
        self,
        name: str,
        description: str,
        triggers: list[str] | None = None,
        category: str = "",
        content: str = "",
        source_agent: str = "python-sdk",
        priority: int = 3,
    ) -> Skill:
        """Add a skill.

        Returns:
            The created Skill.
        """
        req = pb.AddSkillRequest(
            name=name,
            description=description,
            triggers=triggers or [],
            category=category,
            content=content,
            source_agent=source_agent,
            priority=priority,
        )
        resp = await self._call(self._stub.AddSkill, req)
        return Skill.from_proto(resp)

    async def get_skill(self, skill_id: str) -> Skill:
        """Get a skill by ID."""
        req = pb.GetSkillRequest(id=skill_id)
        resp = await self._call(self._stub.GetSkill, req)
        return Skill.from_proto(resp)

    async def list_skills(
        self,
        source_agent: str = "",
        category: str = "",
    ) -> list[Skill]:
        """List skills."""
        req = pb.ListSkillsRequest(source_agent=source_agent, category=category)
        resp = await self._call(self._stub.ListSkills, req)
        return [Skill.from_proto(s) for s in resp.skills]

    async def remove_skill(self, skill_id: str) -> bool:
        """Remove a skill by ID."""
        req = pb.RemoveSkillRequest(id=skill_id)
        resp = await self._call(self._stub.RemoveSkill, req)
        return resp.deleted

    async def search_skills(self, query: str, limit: int = 10) -> list[Skill]:
        """Search skills by query."""
        req = pb.SearchSkillsRequest(query=query, limit=limit)
        resp = await self._call(self._stub.SearchSkills, req)
        return [Skill.from_proto(s) for s in resp.skills]

    # ------------------------------------------------------------------ #
    # Cron Jobs
    # ------------------------------------------------------------------ #

    async def create_cron_job(
        self,
        name: str,
        schedule: str,
        agent_id: str,
        message: str,
        enabled: bool = True,
    ) -> CronJob:
        """Create a cron job.

        Args:
            name: Human-readable job name.
            schedule: Cron expression (e.g. "0 9 * * *").
            agent_id: Agent that owns this job.
            message: Message to send when the job fires.
            enabled: Whether the job starts active.

        Returns:
            The created CronJob.
        """
        req = pb.CreateCronJobRequest(
            name=name,
            schedule=schedule,
            agent_id=agent_id,
            message=message,
            enabled=enabled,
        )
        resp = await self._call(self._stub.CreateCronJob, req)
        return CronJob.from_proto(resp)

    async def list_cron_jobs(
        self,
        agent_id: str = "",
        include_disabled: bool = False,
    ) -> list[CronJob]:
        """List cron jobs."""
        req = pb.ListCronJobsRequest(
            agent_id=agent_id,
            include_disabled=include_disabled,
        )
        resp = await self._call(self._stub.ListCronJobs, req)
        return [CronJob.from_proto(j) for j in resp.jobs]

    async def update_cron_job(
        self,
        job_id: str,
        name: Optional[str] = None,
        schedule: Optional[str] = None,
        message: Optional[str] = None,
        enabled: Optional[bool] = None,
    ) -> CronJob:
        """Update a cron job (partial update)."""
        req = pb.UpdateCronJobRequest(id=job_id)
        if name is not None:
            req.name = name
        if schedule is not None:
            req.schedule = schedule
        if message is not None:
            req.message = message
        if enabled is not None:
            req.enabled = enabled
        resp = await self._call(self._stub.UpdateCronJob, req)
        return CronJob.from_proto(resp)

    async def delete_cron_job(self, job_id: str) -> None:
        """Delete a cron job."""
        req = pb.DeleteCronJobRequest(id=job_id)
        await self._call(self._stub.DeleteCronJob, req)

    # ------------------------------------------------------------------ #
    # Messaging
    # ------------------------------------------------------------------ #

    async def send_message(
        self,
        platform: str,
        target: str,
        message: str,
        agent_id: str = "",
    ) -> tuple[bool, str]:
        """Send a message to a messaging platform.

        Args:
            platform: "discord", "telegram", "imessage", or "email".
            target: Channel ID, user ID, or email address.
            message: Message content.
            agent_id: Agent sending the message.

        Returns:
            Tuple of (success, message_id).
        """
        req = mpb.SendMessageRequest(
            platform=platform,
            target=target,
            message=message,
            agent_id=agent_id,
        )
        resp = await self._call(self._messaging_stub.SendMessage, req)
        return (resp.success, resp.message_id)

    async def list_channels(self, platform: str) -> list[Channel]:
        """List available channels for a messaging platform.

        Args:
            platform: "discord", "telegram", etc.

        Returns:
            List of Channel objects.
        """
        req = mpb.ListChannelsRequest(platform=platform)
        resp = await self._call(self._messaging_stub.ListChannels, req)
        return [Channel.from_proto(c) for c in resp.channels]

    async def subscribe_to_messages(
        self,
        platform: str,
        agent_id: str = "",
    ) -> AsyncIterator[MessageEvent]:
        """Subscribe to incoming messages from a platform.

        Args:
            platform: "discord", "telegram", etc.
            agent_id: Agent subscribing.

        Yields:
            MessageEvent objects as they arrive.
        """
        req = mpb.SubscribeToMessagesRequest(
            platform=platform,
            agent_id=agent_id,
        )
        try:
            stream = self._messaging_stub.SubscribeToMessages(
                req, metadata=self._metadata()
            )
            async for event_msg in stream:
                yield MessageEvent.from_proto(event_msg)
        except grpc.aio.AioRpcError as e:
            _handle_error(e)