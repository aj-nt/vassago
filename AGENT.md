# Mnemosyne Agent — Design Spec

> The daemon remembers. The agent thinks.

## 1. Overview

Mnemosyne has two halves: the **daemon** (memory persistence + pub/sub) and the
**agent** (LLM interaction + tool execution). The daemon is designed; this
document covers the agent.

The agent is the part that:
- Reads your message
- Builds a prompt (system prompt + hot block + context)
- Calls the LLM
- Dispatches tool calls
- Loops until the LLM yields a final response
- Writes memories back to the daemon

It replaces the 7,315-line `run_agent.py` monolith.

## 2. Design Decisions

### 2.1 Decision Making: Hybrid

The LLM decides what to do. The agent decides how to enforce guardrails.

| Decision Type | Who Decides | Mechanism |
|---|---|---|
| What tool to call | LLM | Tool_choice in API request |
| Whether to loop again | LLM (stop) or agent (max turns) | StageAction enum |
| Memory writes | LLM requests, agent validates & writes | Tool call |
| Hot block composition | Daemon (compute), agent (inject) | gRPC GetHotBlock |
| Personality / system prompt | Config (user-defined) | YAML + template |
| Budget enforcement | Agent | Token counting, max turns |
| Safety / approval | Agent | Tool-level allow/deny lists |

The LLM is the driver. The agent is the steering lock — it doesn't choose the
destination, but it prevents the car from leaving the road.

### 2.2 Consolidation: Both

Two modes:

1. **Automatic consolidation** — the daemon runs a periodic sweep (configurable
   interval, default: every hour). It evicts stale memories, marks expired
   observations, and can merge low-priority entries that overlap. The agent
   doesn't need to think about this.

2. **Explicit consolidation** — the agent (or the human via CLI) calls
   `Consolidate` RPC. This is the "spring cleaning" mode — the LLM reads
   through a batch of memories, decides what to merge, what to drop, and what
   to promote.

Automatic consolidation handles the steady-state hygiene. Explicit consolidation
handles the deliberate curation. Both write results to the same database via the
same daemon.

### 2.3 Hot Block Lifecycle (Recommendation: Session-Start + Invalidation)

Three options with tradeoffs:

| Strategy | Description | Pros | Cons |
|---|---|---|---|
| Frozen | Compute once at session start, never update | Simple, predictable | Stale mid-session, no telepathy |
| On-demand | Re-compute every time the LLM loops | Always fresh | Latency per loop, wasted if unchanged |
| **Session-start + invalidation** | Compute at session start, update when pub/sub fires | Fresh when needed, minimal latency | Slightly complex, needs subscription management |

**Recommendation: Session-start + invalidation.**

Rationale: The hot block changes rarely compared to how often the LLM loops.
On a typical session, the hot block is computed once at the start and stays
stable. When another agent writes a memory (telepathy), the invalidation event
triggers a re-compute, which is exactly when you want it fresh. This gives you
telepathy without the overhead of re-computing every loop iteration.

The lifecycle:

```
Agent connects to daemon
    ↓
GetHotBlock(agent_id, config) → hot_block rendered text
    ↓
Inject into system prompt
    ↓
SubscribeHotBlock(agent_id, config) → stream
    ↓
[Agent works — LLM loops, tool calls, etc.]
    ↓
HotBlockUpdate arrives on stream
    ↓
Re-compute hot block, inject into next LLM call
    ↓
[Agent continues with fresh context]
    ↓
Session ends → unsubscribe
```

### 2.4 Multi-Agent Identity (Recommendation: Agent ID + Role)

Each agent instance has:
- A unique **agent ID** (UUID, assigned at first connection or configured)
- A **name** (human-readable, configured: "studio-dev", "qmm-assistant", etc.)
- A **role** (what kind of work this agent does: "dev", "personal", "monitor")

**Recommendation: Agent ID is identity. Role is context. Name is for humans.**

```
agent_id:  "550e8400-e29b-41d4-a716-446655440000"  (UUID, stable)
name:      "studio"                                   (configured, human-readable)
role:      "dev"                                      (configured, shapes hot block)
```

The agent_id is immutable and written into every memory as `source_agent`.
When you search memories, you can ask "what did the studio agent learn?" or
"what did any agent learn?" The role shapes the hot block — a dev agent boosts
`project` and `environment` categories; a personal agent boosts `user` and
`observation`.

This solves the telepathy question: all agents share the same memories (same
database), but each agent gets a hot block tuned to its role. A memory written
by the monitoring agent is visible in the dev agent's context if it's relevant
— that's the point.

### 2.5 Memory Prompt

The system prompt has a memory section that injects the hot block. Current
Hermes approach works well conceptually — we keep the general shape but formalize
it:

```
[Memory — what you know about the user and environment]
{hot_block_content}

[Instructions — how to use memory]
- You can search memories with SearchMemories when you need something not in the hot block.
- Use AddMemory to persist important facts, preferences, and lessons learned.
- Use ReplaceMemory to update existing memories (match by key).
- Use Consolidate to trigger eviction of stale entries and merge overlapping ones.
- Observations auto-expire after 30 days unless promoted to a higher category.
- Other agents may update memories while you work. If you receive a hot block
  update, trust it — it contains information from other agents you work with.
```

The key addition: **explicit mention of other agents updating memories**.
This primes the LLM to trust incoming updates rather than treat them as
contradictions of its own knowledge.

### 2.6 Interface: CLI First, Add More Later

Start with CLI. The agent is a command-line program you run in a terminal.
This is how Hermes works today and it's the fastest path to a working system.

But design for extensibility from day one:

```
┌──────────────┐
│  Interface   │  ← CLI today, Telegram/Discord/API tomorrow
│   Layer      │     Common interface: Message In, Message Out
├──────────────┤
│   Agent      │  ← The loop: prompt → LLM → tools → prompt
│   Core       │     Framework-agnostic. Doesn't know about terminals.
├──────────────┤
│  Mnemosyne   │  ← Memory daemon (gRPC)
│  Client      │     Shared across all agents
└──────────────┘
```

The interface layer is a thin adapter. CLI reads stdin, formats the response,
writes stdout. Telegram receives webhooks, formats the response, sends messages.
The agent core doesn't change — it receives a `UserMessage` and emits
`AssistantResponse` objects.

## 3. Architecture

### 3.1 Pipeline Stages (from Kore Design)

The agent loop is a 5-stage pipeline — adapted from the Kore orchestrator design
that proved itself with 561 tests. The key insight from Kore: **dispatch on
capabilities, not provider identity**. Each stage has typed input/output, no
`if _is_*_backend()` checks, and side effects only through the EventBus.

```
ConversationContext
    ↓
┌──────────────────────┐
│ Stage 1: Request Prep │  System prompt + hot block + cache markers + API kwargs
└────────┬─────────────┘
         ↓ PreparedRequest
┌──────────────────────┐
│ Stage 2: Provider Call│  Dispatch via ProviderRegistry, emit stream deltas
└────────┬─────────────┘
         ↓ ProviderResult
┌──────────────────────┐
│ Stage 3: Response     │  Parse into canonical ParsedResponse
│  Processing          │  (tool calls, reasoning, finish_reason)
└────────┬─────────────┘
         ↓ ParsedResponse
┌──────────────────────┐
│ Stage 4: Tool Dispatch│  Route to handlers, collect tool results
│                      │  Pre-dispatch: checkpoint, approval gates
│                      │  Post-dispatch: nudge counters, write metadata
└────────┬─────────────┘
         ↓ ToolDispatchResult
┌──────────────────────┐
│ Stage 5: Context     │  Append results, check limits, compress if needed
│  Management          │  Persist session, hydrate state for next iteration
└────────┬─────────────┘
         ↓ StageAction (CONTINUE / YIELD / TERMINATE)
    ┌────────────────┐
    │  CONTINUE      │ → Back to Stage 1
    │  YIELD         │ → Return final response
    │  TERMINATE     │ → Max iterations or fatal error
    └────────────────┘
```

Each stage is a callable with a `Process()` method. Testable in isolation via
mock dependencies. The Orchestrator drives the loop.

**Key differences from the Kore implementation:**
- No `if _is_*_backend()` checks — providers declare `Capabilities`, the pipeline
  dispatches on capabilities
- EventBus decouples display/logging from business logic — display is a *subscriber*
- Streaming deltas update `StreamState` and fire callbacks; the pipeline doesn't
  touch the terminal directly
- Context compression (if needed) happens in Stage 5, not scattered across stages

### 3.2 Component Breakdown

```
agent/
├── main.go                        # CLI entry point, signal handling
├── agent.go                       # Orchestrator: drives Stage 1→2→3→4→5 loop
├── config.go                      # Load config from YAML
│
├── context/
│   ├── context.go                 # ConversationContext, SessionState
│   ├── types.go                   # PreparedRequest, ParsedResponse, ProviderResult
│   │                              # ToolDispatchResult, StageAction, PipelineResult
│   └── capabilities.go           # ProviderCapabilities struct
│
├── pipeline/
│   ├── request_prep.go           # Stage 1: Build prompt, inject hot block
│   ├── provider_call.go          # Stage 2: Call LLM via ProviderRegistry
│   ├── response_processing.go    # Stage 3: Parse into ParsedResponse
│   ├── tool_dispatch.go          # Stage 4: Route tool calls, collect results
│   └── context_management.go     # Stage 5: Append results, check limits, persist
│
├── provider/
│   ├── registry.go               # ProviderRegistry: name → Provider
│   ├── provider.go                # Provider interface (Prepare, Execute, Parse)
│   ├── openai.go                  # OpenAI-compatible (covers Ollama, OpenRouter)
│   ├── anthropic.go              # Anthropic Messages API + prompt caching
│   └── mock.go                    # MockProvider for testing (canned responses)
│
├── tools/
│   ├── registry.go               # ToolRegistry: name → ToolHandler
│   ├── memory.go                  # AddMemory, SearchMemories, Replace, Remove, Consolidate
│   ├── terminal.go                # Shell command execution (approval gate)
│   ├── file.go                    # FileRead, FileWrite, FileSearch
│   └── delegate.go                # Sub-agent delegation (future)
│
├── eventbus/
│   └── bus.go                     # EventBus: emit/subscribe for observability
│                                  # Display layer subscribes to events, not mixed in
│
├── interface/
│   ├── cli.go                     # Terminal interface (stdin/stdout)
│   ├── api.go                     # HTTP API interface (future)
│   └── telegram.go                # Telegram bot interface (future)
│
└── mnemosyne/
    └── client.go                  # gRPC client for Mnemosyne daemon
```

### 3.3 The Agent Loop (Orchestrator Pattern)

Adapted from the Kore orchestrator with 561 passing tests. The Orchestrator
is the driver — it creates context, wires stages, and loops. It does NOT
contain business logic itself.

```go
type Orchestrator struct {
    registry    ProviderRegistry
    eventBus    EventBus
    tools       ToolRegistry
    mnemo       MnemosyneClient
    maxIters    int
    // Stages — overridable for testing
    stage1      RequestPrepStage
    stage2      ProviderCallStage
    stage3      ResponseProcessingStage
    stage4      ToolDispatchStage
    stage5      ContextManagementStage
}

func (o *Orchestrator) Run(ctx context.Context, session *SessionState) (*PipelineResult, error) {
    // Get hot block from Mnemosyne daemon
    hotBlock, sub, err := o.mnemo.GetHotBlockAndSubscribe(ctx, session.AgentID, session.HotBlockConfig)
    if err != nil {
        return nil, fmt.Errorf("hot block: %w", err)
    }
    defer sub.Close()

    // Build initial conversation context
    convCtx := &ConversationContext{
        SessionID:     session.ID,
        Messages:      session.Messages,
        SystemPrompt:  o.buildSystemPrompt(hotBlock),
        Tools:         o.tools.Schema(),
        MaxIterations: o.maxIters,
        Interrupt:     NewInterruptSignal(),
    }

    for {
        // Check termination conditions
        if convCtx.Interrupt.IsSet() {
            return &PipelineResult{Interrupted: true}, nil
        }
        if convCtx.Iteration >= convCtx.MaxIterations {
            return &PipelineResult{MaxIterationsHit: true}, nil
        }

        // Emit iteration_start (display layer is a subscriber, not mixed in)
        o.eventBus.Emit(PipelineEvent{Kind: "iteration_start", Iteration: convCtx.Iteration})

        // Stage 1: Request Preparation
        prepared := o.stage1.Process(convCtx)

        // Stage 2: Provider Call (dispatches on capabilities, not provider name)
        result := o.stage2.Process(convCtx, prepared)

        // Stage 3: Response Processing (canonical ParsedResponse)
        parsed := o.stage3.Process(convCtx, result)

        // Stage 4: Tool Dispatch (route to handlers, collect results)
        dispatch := o.stage4.Process(convCtx, parsed)

        // Check for hot block updates from other agents (telepathy)
        select {
        case update := <-sub.Updates():
            hotBlock = update.HotBlock
            convCtx.SystemPrompt = o.buildSystemPrompt(hotBlock)
        default:
        }

        // Stage 5: Context Management (append, persist, loop decision)
        action := o.stage5.Process(convCtx, dispatch.ToolResults, parsed)

        o.eventBus.Emit(PipelineEvent{Kind: "iteration_end", Action: action})

        switch action {
        case StageActionYIELD:
            return &PipelineResult{Response: parsed, Iterations: convCtx.Iteration}, nil
        case StageActionTERMINATE:
            return &PipelineResult{Response: parsed, Iterations: convCtx.Iteration}, nil
        case StageActionCONTINUE:
            convCtx.Iteration++
            continue
        }
    }
}
```

Key patterns from Kore:
- **Stages are overridable** — inject `MockProviderCallStage` in tests, or swap
  `RequestPrepStage` for one with a custom prompt builder
- **EventBus for observability** — display, logging, and metrics subscribe to
  events. The pipeline never calls `print()` or `log()` directly
- **Typed data flows between stages** — `PreparedRequest` → `ProviderResult` →
  `ParsedResponse` → `ToolDispatchResult` → `StageAction`. No `if isinstance()`
- **ProviderRegistry dispatches on capabilities** — no `_is_*_backend()` checks.
  Each provider declares what it can do; the pipeline adapts

### 3.4 Tool Definitions (for LLM)

The tools the LLM sees are defined in JSON Schema format, same as OpenAI's
function calling. Here's the memory tool set:

```json
[
  {
    "name": "AddMemory",
    "description": "Save a memory to persistent storage. Memories survive across sessions.",
    "parameters": {
      "target": {"type": "string", "enum": ["memory", "user"]},
      "category": {"type": "string", "enum": ["user", "environment", "quirk", "project", "observation"]},
      "key": {"type": "string", "description": "Unique key for upsert"},
      "content": {"type": "string"},
      "priority": {"type": "integer", "minimum": 1, "maximum": 5, "default": 3}
    }
  },
  {
    "name": "SearchMemories",
    "description": "Search memories by keywords. Use real words, not wildcards.",
    "parameters": {
      "query": {"type": "string"},
      "target": {"type": "string", "enum": ["memory", "user"]},
      "category": {"type": "string"},
      "limit": {"type": "integer", "default": 20}
    }
  },
  {
    "name": "ReplaceMemory",
    "description": "Update an existing memory by key. Must match old content for safety.",
    "parameters": {
      "target": {"type": "string"},
      "key": {"type": "string"},
      "old_content": {"type": "string", "description": "Must match current content"},
      "new_content": {"type": "string"},
      "new_priority": {"type": "integer"}
    }
  },
  {
    "name": "RemoveMemory",
    "description": "Delete a memory by key.",
    "parameters": {
      "target": {"type": "string"},
      "key": {"type": "string"}
    }
  },
  {
    "name": "Consolidate",
    "description": "Trigger memory consolidation: evict stale entries, review overlapping ones.",
    "parameters": {
      "scope": {"type": "string", "enum": ["ALL", "IDLE", "OVERLAPPING", "OBSERVATIONS", "EXPIRED"], "default": "IDLE"},
      "batch_size": {"type": "integer", "default": 10, "description": "How many candidates to return for review"}
    }
  }
]
```

**Skills tools** (shared procedural memory):

```json
[
  {
    "name": "AddSkill",
    "description": "Save a reusable skill (procedure, workflow, or reference) that all agents can access.",
    "parameters": {
      "name": {"type": "string", "description": "Unique skill name (kebab-case)"},
      "description": {"type": "string", "description": "One-line summary"},
      "triggers": {"type": "array", "items": {"type": "string"}, "description": "Keywords that trigger this skill"},
      "category": {"type": "string", "enum": ["devops", "software-development", "data-science", "research", "writing", "general"]},
      "content": {"type": "string", "description": "Full skill content (markdown)"},
      "priority": {"type": "integer", "minimum": 1, "maximum": 5, "default": 3}
    }
  },
  {
    "name": "GetSkill",
    "description": "Retrieve a specific skill by ID.",
    "parameters": {"id": {"type": "string", "description": "Skill ID"}}
  },
  {
    "name": "ListSkills",
    "description": "List all skills, optionally filtered by category.",
    "parameters": {"category": {"type": "string", "description": "Filter by category"}}
  },
  {
    "name": "SearchSkills",
    "description": "Search skills by keywords. Use real words, not wildcards.",
    "parameters": {"query": {"type": "string"}, "limit": {"type": "integer", "default": 20}}
  },
  {
    "name": "RemoveSkill",
    "description": "Delete a skill by ID.",
    "parameters": {"id": {"type": "string", "description": "Skill ID"}}
  }
]
```

### 3.5 Session Persistence

Sessions and messages are recorded in the Mnemosyne daemon's database via
gRPC. The agent does NOT write to SQLite directly.

The daemon provides session lifecycle RPCs (see DESIGN.md §6):

```protobuf
rpc CreateSession(CreateSessionRequest) returns (SessionResponse);
rpc EndSession(EndSessionRequest) returns (Empty);
rpc AddMessages(AddMessagesRequest) returns (Empty);
rpc GetSession(GetSessionRequest) returns (SessionResponse);
```

The agent calls `CreateSession` at conversation start, `AddMessages` after each
turn (user message + assistant response + tool results), and `EndSession` when
the conversation ends. This lets the daemon store session data for future
search via `SearchSessions`.

Session resume (`--resume`) works by:
1. Agent calls `GetSession(session_id)` to retrieve prior messages
2. Messages are loaded into `SessionState.Messages`
3. The pipeline continues from where it left off

### 3.6 Error Handling & Fallback

**Open item — to be designed before Phase 4 implementation.**

The Kore `ProviderResult` has `should_retry` and `should_fallback` flags. The
agent needs comparable error handling for:

- **Provider errors**: 429/503 (retryable), 401/400 (fail immediately), connection timeout
- **Fallback chains**: primary → fallback1 → fallback2 (config-driven)
- **Daemon unreachable**: what happens when the memory daemon is down?
- **Stream interruption**: Ctrl+C during streaming, stale streams, timeouts

Decisions needed:
- How many retries? Exponential backoff parameters?
- Fallback chain format in `agent.yaml`
- Daemon-down behavior: fail tool calls with message, or buffer and flush?
- Stream timeout handling (carried over from Kore's `StreamConfig`)

### 3.7 Streaming

The LLM provider streams response tokens. The pipeline handles this in
Stage 2 and Stage 3:

**Stage 2 (Provider Call):** If the provider supports streaming
(`Capabilities.SupportsStreaming`), the call returns a stream of deltas.
The stage accumulates text into `StreamState.AccumulatedText` and emits
`stream_delta` events on the EventBus.

**Stage 3 (Response Processing):** Waits for the stream to complete (or
time out via `StreamState.Config`), then parses the full accumulated text
into `ParsedResponse`.

**Display layer:** subscribes to `stream_delta` events on the EventBus.
Each delta writes to the terminal in real-time. The pipeline never touches
the terminal — it just emits events.

**Stream interruption:** If the user presses Ctrl+C during streaming,
`InterruptSignal` is set. Stage 2 detects this and returns the partial
response. Stage 3 parses what it has. The pipeline yields the partial
response.

**Timeout handling** (from Kore's `StreamConfig`):

```yaml
streaming:
  stale_timeout: 30s       # No delta for 30s → stale
  stale_char_rate: 0.5     # Below 0.5 chars/sec → stale
  max_inactivity: 120s     # Hard timeout for no activity
  empty_content_max_retries: 3  # Consecutive empty responses allowed
```

## 4. Open Items

These need design decisions before implementation:

| # | Item | Area | When |
|---|---|---|---|
| 1 | Error handling: retries, backoff, fallback chain format | Agent §3.6 | Before Phase 2 (agent implementation) |
| 2 | Daemon-down behavior: fail tool calls vs. buffer-and-flush | Agent §3.6 | Before Phase 2 (agent implementation) |
| 3 | Stream interruption: Ctrl+C, stale detection, timeout params | Agent §3.7 | Before Phase 2 (agent implementation) |
| 4 | Role-to-HotBlockConfig resolution: agent-only or daemon-aware? | Daemon §5.1 | Before Phase 3 (telepathy) |
| 5 | Auto-consolidation goroutine: startup only or periodic? Trigger RPC? | Daemon §7.4 | Before Phase 4 (search & consolidation) |

## 5. Configuration

```yaml
# agent.yaml
agent:
  id: "550e8400-e29b-41d4-a716-446655440000"  # UUID, assigned at first connection
  name: "studio-dev"                            # Human-readable name
  role: "dev"                                   # Role: shapes hot block priorities
  personality: "direct, dry-witted"             # System prompt personality directive
  max_turns: 50                                 # Max tool-call loops per conversation

provider:
  name: "openai"                            # Provider: openai | anthropic | ollama
  model: "gpt-4o"                           # Model name
  api_key_env: "OPENAI_API_KEY"             # Environment variable for API key
  base_url: ""                              # Override for OpenAI-compatible APIs
  temperature: 0.7

mnemosyne:
  address: "unix://~/.mnemosyne/mnemosyne.sock"
  # address: "localhost:50051"              # For LAN/internet

hot_block:
  memory_char_limit: 4000
  user_char_limit: 2000
  min_priority: 2

tools:
  enabled:
    - AddMemory
    - SearchMemories
    - ReplaceMemory
    - RemoveMemory
    - Consolidate
    - AddSkill
    - GetSkill
    - ListSkills
    - SearchSkills
    - RemoveSkill
    - Terminal
    - FileRead
    - FileWrite
    - FileSearch
  approval_required:                         # Require user approval before executing
    - Terminal                              # Safety: don't run arbitrary commands unchecked

logging:
  level: "info"
  sessions_dir: "~/.mnemosyne/sessions"
```

### 5.1 Hot Block Per-Role Defaults

```yaml
# Role-specific hot block categories (merged with base config)
roles:
  dev:
    boost_categories: ["project", "environment", "quirk"]
    suppress_categories: ["observation"]
  personal:
    boost_categories: ["user", "observation", "environment"]
    suppress_categories: ["quirk"]
  monitor:
    boost_categories: ["environment", "project"]
    suppress_categories: ["user"]
```

## 6. Interface Layer

### 6.1 CLI (Phase 1)

The simplest interface. Read stdin, write stdout. This is what we build first.

```bash
# Start a conversation
mnemosyne-agent chat

# Start with a prompt
mnemosyne-agent chat "What were we working on yesterday?"

# Resume a session
mnemosyne-agent chat --resume <session-id>

# One-shot mode
echo "Add this to memory: the server is at node-3.internal" | mnemosyne-agent chat
```

The CLI handles:
- Terminal formatting (markdown rendering, color)
- Session management (save/resume conversations)
- Signal handling (Ctrl+C to interrupt tool execution)
- Streaming output (token-by-token display)

### 6.2 HTTP API (Phase 2)

```bash
mnemosyne-agent serve --port 8080
```

Simple REST endpoint: POST /chat with `{"message": "..."}` returns streamed
response. This enables web UIs, integrations, and remote access.

### 6.3 Telegram / Discord / iMessage (Phase 3)

Each interface is a thin adapter that:
- Receives messages from the platform
- Formats them as `UserMessage`
- Calls `agent.Run(ctx, msg)`
- Formats `AssistantResponse` back to the platform

The agent core doesn't change. Only the interface layer does.

## 7. Deployment

### 7.1 Development (Current)

```
QMM (dev machine) — personal machine
├── mnemosyne-daemon    (Unix socket, connects to Studio daemon for integration)
├── mnemosyne-agent     (CLI, connects to local daemon for dev)
└── go test ./...       (TDD)
```

### 7.2 Testing

```
Always-on server ("studio")
├── mnemosyne-daemon    (TCP :50051, always-on)
├── mnemosyne-agent     (CLI, for ad-hoc testing)
└── Integration tests   (continuous)
```

### 7.3 Production

```
LAN / Internet
├── mnemosyne-daemon    (TCP + TLS, always-on, behind gateway)
├── mnemosyne-agent     (multiple instances, different roles)
│   ├── studio-dev      (role: dev, boost: project/environment)
│   ├── studio-monitor  (role: monitor, boost: environment/project)
│   └── qmm-personal    (role: personal, boost: user/observation)
└── mnemosyne-cli       (admin: stats, search, maintenance)
```

## 8. What We're NOT Building (Yet)

These are explicitly out of scope for the initial phases:

| Feature | Why Not Now |
|---|---|
| Web UI | CLI first. Web adds complexity without adding agent functionality. |
| Multi-user auth | Single user for now. ACL system later. |
| MCP (Model Context Protocol) | Interesting but premature. gRPC is simpler and sufficient. |
| Browser tool | Complex, adds dependencies. Terminal and files are enough for v1. |
| Delegation (sub-agents) | Needs solid single-agent first. Telepathy makes this natural later. |
| Image generation | Not core to memory. Add as a tool when needed. |
| RAG over conversation history | Session search covers this. Dedicated RAG is overengineering. |

## 9. Testing Strategy

Same rules as the daemon:

- **Prototyping**: Manual verification, branches, don't check in
- **Production code**: TDD — write test first
- **QA Pass**: Tests pass, edge cases covered, no panics
- **Red Team QA**: Break it, crash it, starve it, lie to it

### Test Layers

| Layer | What | How |
|---|---|---|
| Unit | Individual stages, tool handlers, prompt building | Go tests, mocks for LLM |
| Integration | Full agent loop with fake LLM | Mock provider returns canned responses |
| End-to-end | Agent + daemon + real LLM | Careful: costs money, rate limits |
| Red Team | Malformed tool calls, infinite loops, context overflow | Negative tests |

### Mock Provider

The key to testing the agent loop without calling a real LLM:

```go
type MockProvider struct {
    Responses []ChatResponse  // Pre-canned responses
    CallCount int
}

func (m *MockProvider) ChatCompletion(ctx context.Context, msgs []Message, tools []Tool) (ChatResponse, error) {
    if m.CallCount >= len(m.Responses) {
        return ChatResponse{Text: "done"}, nil
    }
    resp := m.Responses[m.CallCount]
    m.CallCount++
    return resp, nil
}
```

This lets us test:
- "LLM calls a tool, gets a result, calls another tool, then yields text"
- "LLM yields text immediately (no tool calls)"
- "Maximum turns reached"
- "Hot block update arrives mid-loop"
- "Tool call fails"

Without ever calling OpenAI.

## 10. Host Assignment

| Host | Role | Mnemosyne Daemon | Agent |
|---|---|---|---|
| QMM (dev machine) | Development | No (connects to daemon host) | CLI for ad-hoc dev |
| Studio (always-on server) | Testing + always-on daemon | Yes (Unix socket + TCP :50051) | CLI for testing, cron agents |
| Future (LAN/Internet) | Production | Yes (TCP + TLS) | Multiple agents |

QMM is the development machine. Studio is the always-on test bed. The daemon
runs on Studio so it's always available, and agents on any machine can connect
via gRPC.

## 11. Repository Structure

```
mnemosyne/                          # Monorepo
├── daemon/                         # The memory daemon (DESIGN.md)
│   ├── cmd/mnemosyne-daemon/
│   ├── cmd/mnemosyne-cli/
│   ├── cmd/mnemosyne-import/
│   ├── internal/
│   └── proto/
│
├── agent/                          # The agent (this document)
│   ├── cmd/mnemosyne-agent/
│   ├── pipeline/
│   ├── prompt/
│   ├── provider/
│   ├── tools/
│   └── interface/
│
├── client/                         # Shared gRPC client library
│   └── client.go
│
├── configs/
│   ├── mnemosyne.yaml              # Daemon config
│   ├── agent-dev.yaml              # Dev agent config
│   └── agent-monitor.yaml          # Monitor agent config
│
├── proto/
│   └── mnemosyne.proto
│
├── tests/
│   ├── integration/
│   └── fixtures/
│
├── go.mod
├── go.sum
├── DESIGN.md                       # Daemon spec (existing)
├── AGENT.md                        # Agent spec (this document)
├── ROADMAP.md                      # Phased delivery (existing, updated)
└── README.md
```

Shared `client/` package — both the daemon's gRPC service implementation and
the agent's connection use the same proto and client code.