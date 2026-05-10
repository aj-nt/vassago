# Mnemosyne Roadmap

## Completed

### Phase 24: Bubble Tea TUI (COMPLETE)
Branch: `feat/bubbletea-tui`, merged to main (commit 665c88b)

- Phase 1: Program loop, streaming SSE → tea.Msg, slash commands, --tui flag
- Phase 2: Viewport, chat history, turn state machine, lipgloss panels
- Phase 3: textarea multi-line input, input history, tool approval scaffolding
- Phase 3b: Interactive tool approval via tea.Program.Send() + approvalCh (thread-safe)
- Phase 4: Mouse support (WithMouseCellMotion), one-shot TUI mode (RunOneShotBubble)
- Phase 5: Assessment only — tea_model.go uses lipgloss exclusively, no raw ANSI

---

## Planned

### Phase 25: Saved Tools — Runtime Tool Creation and Sharing
### Phase 25: Saved Tools — Runtime Tool Creation and Sharing

**Problem:** Tools are compiled Go functions. The set of tools is fixed at build time. An agent cannot create a new tool during a session, and it cannot share a procedure it discovers with other agents or future sessions. Skills (procedural knowledge) are prompt-injected — the agent reads them but doesn't "call" them. What's missing is **executable procedural memory**: procedures the agent creates, saves to the DB, and calls as first-class tools in future sessions.

**Example use cases:**

| Tool Type | Example | What it does |
|-----------|---------|--------------|
| `script` | `deploy_service` | Takes `{env: staging\|prod, service: string}`, substitutes into a bash template, runs via Execute with approval |
| `script` | `calculate_natal_chart` | Takes `{birth_date, birth_time, latitude, longitude}`, runs a Python script that computes planetary positions |
| `prompt_template` | `code_review` | Takes `{language, code, focus_area}`, formats a structured review prompt, returns the LLM's analysis |
| `prompt_template` | `debugging_protocol` | Takes `{error_message, language, stack_trace}`, formats a systematic debugging prompt |

**Relationship to existing features:**

| Feature | What it is | Persistent? | Callable by LLM? | Executable? |
|---------|-----------|-------------|-------------------|-------------|
| **Memories** | Factual knowledge | DB + FTS5 | No (hot block) | No |
| **Skills** | Procedural knowledge (how-to) | DB + FTS5 | No (system prompt) | No |
| **Saved Tools** | Callable procedures | DB + FTS5 | **Yes** (as tool defs) | **Yes** |
| **Compiled Tools** | Go functions | Binary | Yes | Yes |

#### Architecture

**Database schema (v11 migration):**

```sql
CREATE TABLE saved_tools (
    id          TEXT PRIMARY KEY,       -- UUID
    name        TEXT NOT NULL UNIQUE,    -- e.g., "calculate_natal_chart"
    description TEXT NOT NULL,           -- LLM-readable description for tool schema
    type        TEXT NOT NULL,           -- "script" or "prompt_template"
    parameters  TEXT NOT NULL,           -- JSON schema for inputs (like compiled tool schemas)
    script      TEXT NOT NULL DEFAULT '', -- executable content (bash/python/prompt template)
    category    TEXT NOT NULL DEFAULT '', -- e.g., "astrology", "devops"
    triggers    TEXT NOT NULL DEFAULT '[]', -- JSON array of trigger keywords for discovery
    author      TEXT NOT NULL,           -- agent ID that created it
    priority    INTEGER NOT NULL DEFAULT 3, -- 1-5, like skills
    created_at  INTEGER NOT NULL,       -- Unix timestamp
    updated_at  INTEGER NOT NULL         -- Unix timestamp
);

CREATE VIRTUAL TABLE saved_tools_fts USING fts5(
    name, description, triggers, category, content=saved_tools
);
```

**gRPC messages (proto/mnemosyne.proto):**

```protobuf
message AddSavedToolRequest {
    string name = 1;
    string description = 2;
    string type = 3;           // "script" or "prompt_template"
    string parameters = 4;      // JSON schema
    string script = 5;         // template content
    string category = 6;
    repeated string triggers = 7;
    string author = 8;
    int32 priority = 9;
}

message SavedToolEntry {
    string id = 1;
    string name = 2;
    string description = 3;
    string type = 4;
    string parameters = 5;
    string script = 6;
    string category = 7;
    repeated string triggers = 8;
    string author = 9;
    int32 priority = 10;
    int64 created_at = 11;
    int64 updated_at = 12;
}

// CRUD: AddSavedTool, GetSavedTool, ListSavedTools, SearchSavedTools,
//       UpdateSavedTool, RemoveSavedTool
```

**Agent tool implementations (agent/tools/saved_tools.go):**

6 new tools: `CreateTool`, `GetTool`, `ListTools`, `SearchTools`, `UpdateTool`, `RemoveTool`

`CreateTool` validates and stores the tool definition. Key validation:
- `name` must be unique, alphanumeric + underscores, 2-64 chars
- `type` must be "script" or "prompt_template"
- `parameters` must be valid JSON Schema (at minimum: type=object, properties listed)
- `script` must be non-empty
- Script-type tools undergo the same command validation as Execute (no subshells, no dangerous commands)

**Runtime tool loading (agent/agent.go):**

At session start, after compiled tools are registered:

```go
// GetSavedToolDefinitions fetches active saved tools and returns them as
// ToolDefinitions for the LLM. Only tools whose triggers match the
// conversation context are included (triggered loading).
// If no triggers match, the top tools by priority are loaded (up to 10).
func (a *Agent) GetSavedToolDefinitions(convCtx *agentctx.ConversationContext) []agentctx.ToolDefinition {
    tools, err := a.Mnemo.SearchSavedTools(ctx, "", "", 50)
    // Filter by trigger matching against the user's first message
    // Convert each SavedTool to a ToolDefinition
}

// RegisterSavedToolHandlers creates dynamic ToolHandlers for saved tools.
// Each handler:
//   - For "script" type: substitutes {{param}} in script, calls Execute with approval
//   - For "prompt_template" type: substitutes {{param}} in template, returns formatted string
func (a *Agent) RegisterSavedToolHandlers(registry *tools.ToolRegistry, savedTools []SavedToolEntry) {
    for _, st := range savedTools {
        switch st.Type {
        case "script":
            registry.Register(st.Name, makeScriptToolHandler(st), st.Parameters)
        case "prompt_template":
            registry.Register(st.Name, makePromptTemplateHandler(st), st.Parameters)
        }
    }
}
```

**Script type execution flow:**

```
LLM calls: CalculateNatalChart(birth_date="1990-01-15", birth_time="23:09", latitude="47.0435", longitude="-122.9142")
    │
    ▼
SavedToolHandler looks up "CalculateNatalChart" in DB
    │
    ▼
Parameter substitution: script template has {{birth_date}}, {{birth_time}}, etc.
    │
    ▼
Result: a concrete bash/python command
    │
    ▼
Calls existing Execute tool with security checks (path validation, command validation, approval)
    │
    ▼
Returns result to LLM
```

**Prompt template execution flow:**

```
LLM calls: CodeReview(language="python", code="def foo(): ...", focus_area="security")
    │
    ▼
SavedToolHandler looks up "CodeReview" in DB
    │
    ▼
Parameter substitution: template has {{language}}, {{code}}, {{focus_area}}
    │
    ▼
Result: a fully-formed prompt string
    │
    ▼
Returns the formatted string as the tool result (no execution, no security risk)
```

**Security model:**

- **CreateTool requires approval** — persistent tools are a security boundary. The agent must explain what the tool does, and the user confirms (existing approval system from Phase 3b).
- **Script tools inherit all Execute security** — `NeedsApproval()`, path validation, command validation. A saved script doesn't bypass any checks.
- **UpdateTool/RemoveTool require approval** — same as CreateTool.
- **No new code execution surface** — scripts run through the existing Execute handler. No eval, no arbitrary code execution beyond what Execute already allows.
- **Audit trail** — `author` field tracks which agent created each tool. `created_at`/`updated_at` for traceability.
- **Trigger-based loading** — not all 50 tools go to the LLM every turn. Only tools whose triggers match the conversation (or top-priority defaults) are injected, keeping the system prompt manageable.

**Discovery and context management:**

- `GetSavedToolDefinitions()` is called alongside `GetSkillBlock()` at session start
- Tools are injected into the tool list sent to the LLM (the `tools` array in the OpenAI request)
- Trigger matching: if the user says "deploy to staging", tools with triggers ["deploy", "staging"] are loaded
- Fallback: if no triggers match, load the top 10 by priority
- After tool creation, the new tool is immediately available in the current session (no restart needed)

#### Implementation phases

**25A: Storage layer** (daemon/internal/store/saved_tools.go)
- `saved_tools` table + FTS5, schema migration v10→v11
- Store CRUD: AddSavedTool, GetSavedTool, ListSavedTools, SearchSavedTools, UpdateSavedTool, RemoveSavedTool
- gRPC service methods: AddSavedTool, GetSavedTool, ListSavedTools, SearchSavedTools, UpdateSavedTool, RemoveSavedTool
- gRPC handler tests
- Estimated: ~300 lines store, ~150 lines gRPC, ~100 lines tests

**25B: Agent tools** (agent/tools/saved_tools.go)
- 6 tool handlers + JSON schemas: CreateTool, GetTool, ListTools, SearchTools, UpdateTool, RemoveTool
- RegisterSavedToolTools() wiring in setup.go
- Tool approval for CreateTool/UpdateTool/RemoveTool (leverages Phase 3b approval system)
- Agent tool tests
- Estimated: ~400 lines + ~150 lines tests

**25C: Runtime tool loading and execution** (agent/agent.go)
- `GetSavedToolDefinitions()` — fetch and convert to ToolDefinitions
- `RegisterSavedToolHandlers()` — create dynamic handlers
- Script handler: parameter substitution → Execute with security
- Prompt template handler: parameter substitution → string return
- Trigger-based loading with priority fallback
- Integration test: create tool, verify it appears in tool list, call it
- Estimated: ~250 lines + ~100 lines tests

**25D: TUI discovery** (agent/interface/tea_model.go)
- `/tools` command — list saved tools with name, type, description
- `/tools create` — interactive wizard in TUI (future, not blocking)
- One-shot: `mnemosyne chat --message "create a tool that..."` works naturally
- Estimated: ~100 lines

**Total estimated effort:** 4-5 sessions for 25A-D.

---

### Phase 26: Reasoning Model Support (DeepSeek-v4-pro, OpenAI o-series) — COMPLETE

**Commit:** 9ced143

- Added `StreamReasoning` event type and `Reasoning` field to `StreamEvent` (agent/context/stream.go)
- Extended OpenAI SSE parser to recognize `reasoning_content` (DeepSeek) and `reasoning` (OpenAI o-series) delta fields (agent/provider/openai_stream.go)
- Accumulated reasoning text in `streamingCaller` into `ParsedResponse.Reasoning` (agent/streaming_caller.go)
- CLI: reasoning tokens displayed with `💭` dim prefix, clean newline transition to content
- TUI: reasoning accumulated in chat history with live updates, reset on content start
- Integration tests: SSE stream parsing with both `reasoning_content` and `reasoning` fields

---

### Phase 27: GitHub Integration (Monitoring)

**Problem:** When Mnemosyne goes public, we need to monitor the GitHub repo for issues, discussions, PRs, and community submissions. Currently there's no integration.

**Planned components:**

1. **GitHub webhook listener** — daemon endpoint that receives `issues`, `issue_comment`, `pull_request`, and `discussion` events
2. **Issue-to-memory pipeline** — new GitHub issues and PRs are ingested as memories (category: `github-issue`) so the agent is aware of them
3. **Agent notification** — when a new issue/PR is created, the agent is alerted via the event bus (existing pub/sub infrastructure)
4. **GitHub Auth** — OAuth app or personal access token stored in daemon config

This is a separate phase from Saved Tools and can be developed independently.

---

### Phase 28: Multi-User Auth (NOT STARTED)

Deferred. Requires design for API keys, agent isolation, per-user memory namespaces.