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

**Problem:** Tools are compiled Go functions. The set of tools is fixed at build time. An agent cannot create a new tool during a session, and it cannot share a procedure it discovers with other agents or future sessions. Skills (procedural knowledge) tell the agent *how* to think about something, but they're passive — the agent reads them, it doesn't call them. What's missing is **executable procedural memory**: procedures the agent creates, saves to the DB, and calls as first-class tools in future sessions.

**Example use case:** An agent works through an astrology chart calculation that involves multiple steps (ephemeris lookup, chart generation, interpretation). Currently it re-derives the approach each session. With Saved Tools, it creates `calculate_natal_chart` once — parametrized, validated, reproducible — and every future session calls it like any other tool.

#### Architecture

```
┌─────────────────────────────────────────────────────┐
│                    Session Start                      │
│                                                       │
│  Compiled Tools          Saved Tools                  │
│  (Go ToolRegistry)        (from DB via gRPC)          │
│       │                        │                       │
│       └──────────┬─────────────┘                       │
│                  ▼                                     │
│         Tool Definitions → LLM                         │
│         (offered as callable functions)                 │
│                                                       │
│  System Prompt: Skills + Hot Block                     │
│  (procedural knowledge, not callable)                  │
└─────────────────────────────────────────────────────┘
```

**SavedTool DB schema:**

```sql
CREATE TABLE saved_tools (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,      -- e.g., "calculate_natal_chart"
    description TEXT NOT NULL,            -- LLM-readable description
    type        TEXT NOT NULL,             -- "script" | "prompt_template"
    parameters  TEXT NOT NULL,             -- JSON schema for inputs
    script      TEXT NOT NULL,             -- executable content (bash, python, prompt template)
    category    TEXT NOT NULL DEFAULT '',   -- e.g., "astrology", "devops"
    triggers    TEXT NOT NULL DEFAULT '[]', -- JSON array of trigger keywords
    author      TEXT NOT NULL,             -- agent ID that created it
    priority    INTEGER NOT NULL DEFAULT 3, -- 1-5, like memories
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE VIRTUAL TABLE saved_tools_fts USING fts5(name, description, triggers, category, content=sa
ved_tools);
```

**Three tool types:**

1. **`script`** — A parameterized bash or Python script. The agent defines the script template with `{{param}}` placeholders. When called, parameters are substituted and executed via the existing `Execute` tool (with all its security checks: path validation, command validation, approval). Example: a deployment script that takes `environment` and `service` parameters.

2. **`prompt_template`** — A reusable LLM prompt template. The agent defines a structured prompt with `{{param}}` placeholders. When called, parameters are substituted and the result is injected as a user message. Not executed — just formulated. Example: a code review template that takes `language`, `code`, and `focus_area` parameters.

**Tool lifecycle:**

```
Agent discovers a repeatable pattern
       │
       ▼
Agent calls CreateTool (new built-in tool)
  - specifies name, description, type, parameters, script/template
  - optional: triggers (keywords for auto-suggestion)
  - optional: category (for organization)
       │
       ▼
Tool saved to daemon DB
  - Available to ALL agents immediately
  - FTS5 searchable like memories/skills
       │
       ▼
On next session start:
  - GetSavedToolDefinitions() fetches all tools
  - Each saved tool registered as a ToolHandler in ToolRegistry
  - LLM sees it alongside compiled-in tools
  - Agent calls it like any other tool
```

**New built-in agent tools:**

| Tool | Purpose |
|------|---------|
| `CreateTool` | Save a new tool (name, description, type, params, script) |
| `GetTool` | Retrieve a saved tool's definition |
| `ListTools` | List saved tools, optionally filtered by category |
| `SearchTools` | FTS5 search across tool names, descriptions, triggers |
| `UpdateTool` | Modify an existing saved tool |
| `RemoveTool` | Delete a saved tool |

**New gRPC methods:**

| Method | Purpose |
|--------|---------|
| `AddSavedTool` | Create a new saved tool |
| `GetSavedTool` | Retrieve by ID |
| `ListSavedTools` | List by category/author |
| `SearchSavedTools` | FTS5 search |
| `RemoveSavedTool` | Delete a saved tool |
| `UpdateSavedTool` | Modify an existing tool |

**Runtime registration:**

In `agent/agent.go`, alongside `GetSkillBlock()`:

```go
func (a *Agent) GetSavedToolDefinitions() ([]agentctx.ToolDefinition, error) {
    tools, err := a.Mnemo.SearchSavedTools(ctx, "", 50)
    // Convert each SavedTool to a ToolDefinition
    // Register each as a dynamic ToolHandler that:
    //   - For "script" type: substitutes params into script, calls Execute
    //   - For "prompt_template" type: substitutes params, returns formatted string
}
```

**Security model:**

- **Script tools** inherit all `Execute` tool security: path validation, command validation, approval for dangerous commands. Saved scripts go through the same `NeedsApproval()` check. No new attack surface.
- **`NeedsApproval` check on `CreateTool`** — creating a tool requires user approval (it's a persistent change). The agent explains what the tool does and the user confirms.
- **`UpdateTool`/`RemoveTool`** — also require approval. The agent that created a tool can update it; other agents can only use it.
- **Script content validation** — `CreateTool` validates that the script doesn't contain obviously malicious patterns (same checks as `Execute`). Saved tools can't bypass the approval system.
- **Audit trail** — `author` field tracks which agent created each tool, `created_at`/`updated_at` for traceability.

**Relationship to existing features:**

| Feature | What it is | Persistence | Callable by LLM | Executable |
|---------|-----------|-------------|-----------------|------------|
| **Memories** | Factual knowledge | DB + FTS5 | No (hot block injection) | No |
| **Skills** | Procedural knowledge (how-to) | DB + FTS5 | No (system prompt) | No |
| **Saved Tools** | Callable procedures | DB + FTS5 | **Yes** (as tool definitions) | **Yes** (via Execute or prompt injection) |
| **Compiled Tools** | Go functions | Binary | Yes | Yes |

Skills and Saved Tools complement each other: a Skill might say "use the `deploy_service` tool for deployments" and the corresponding Saved Tool provides the actual script.

**Implementation phases:**

**25A: Storage layer** (SQLite + gRPC)
- `saved_tools` table + FTS5 index in daemon DB
- Schema migration (v10 → v11)
- CRUD methods in `daemon/internal/store/saved_tools.go`
- gRPC methods: AddSavedTool, GetSavedTool, ListSavedTools, SearchSavedTools, RemoveSavedTool, UpdateSavedTool
- gRPC handler tests

**25B: Agent tools** (CreateTool, GetTool, etc.)
- New file: `agent/tools/saved_tools.go`
- RegisterSavedToolTools() — 6 tool handlers + JSON schemas
- Agent tool tests
- Wire into tool registry in `agent/setup/setup.go`

**25C: Runtime tool loading**
- `GetSavedToolDefinitions()` in agent.go
- Dynamic ToolHandler registration from DB at session start
- Script execution: parameter substitution → Execute (with security checks)
- Prompt template execution: parameter substitution → formatted string return
- Integration test: create tool, verify it appears in next session's tool list, call it

**25D: TUI + UX**
- `/tools` command in both TUI and old CLI to list saved tools
- `/tools create` wizard in TUI
- Approval prompt for CreateTool/UpdateTool/RemoveTool (using existing Phase 3b approval flow)

**Estimated effort:** 2-3 sessions for 25A+B, 1-2 sessions for 25C, 1 session for 25D.

---

### Phase 26: GitHub Integration (Monitoring)

**Problem:** When Mnemosyne goes public, we need to monitor the GitHub repo for issues, discussions, PRs, and community submissions. Currently there's no integration.

**Planned components:**

1. **GitHub webhook listener** — daemon endpoint that receives `issues`, `issue_comment`, `pull_request`, and `discussion` events
2. **Issue-to-memory pipeline** — new GitHub issues and PRs are ingested as memories (category: `github-issue`) so the agent is aware of them
3. **Agent notification** — when a new issue/PR is created, the agent is alerted via the event bus (existing pub/sub infrastructure)
4. **GitHub Auth** — OAuth app or personal access token stored in daemon config

This is a separate phase from Saved Tools and can be developed independently.

---

### Phase 27: Multi-User Auth (NOT STARTED)

Deferred. Requires design for API keys, agent isolation, per-user memory namespaces.