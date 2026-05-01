# TradingAgents → Mnemosyne: Crossover Design Ideas

> Source: [tauricresearch/tradingagents](https://github.com/tauricresearch/tradingagents) v0.2.4  
> Date: 2026-04-30  
> Status: Analysis complete, ideas scoped for Mnemosyne Phase 14+

---

## 1. TradingAgents Architecture Summary

**What it is:** A LangGraph-orchestrated multi-agent system for financial trading analysis. ~8K lines of Python, 27 source files. Research prototype — not production trading software.

**Agent Pipeline:**
```
Analysts (market/social/news/fundamentals)
  → Bull Researcher ↔ Bear Researcher (debate, N rounds)
    → Research Manager (synthesizes debate → structured plan)
      → Trader (translates plan → transaction proposal)
        → Aggressive ↔ Conservative ↔ Neutral (risk debate, N rounds)
          → Portfolio Manager (final decision, structured output)
```

**Tech stack:** Python 3.10+, LangChain + LangGraph (StateGraph), yfinance/Alpha Vantage, Pydantic structured output.

---

## 2. Patterns Worth Stealing for Mnemosyne

### 2.1 Structured Output with Fallback ⭐⭐⭐

**What TradingAgents does:** The three decision-making agents (Research Manager, Trader, Portfolio Manager) use Pydantic schemas (`ResearchPlan`, `TraderProposal`, `PortfolioDecision`) with `with_structured_output()`. If the provider doesn't support structured output, it gracefully falls back to free-text generation + regex parsing.

**Why it matters for Mnemosyne:** Currently Mnemosyne's tool calls return free-text JSON strings that the LLM has to format correctly. Structured output would:
- Guarantee tool call arguments parse correctly (kills the "empty tool args crash" bug permanently)
- Let us define typed input schemas for every tool
- Make agent output machine-readable for downstream tools (e.g., the memory log could be structured)

**How to adapt (Go):**
```go
// Define tool input schemas as Go structs with JSON tags
type StoreMemoryInput struct {
    Category string   `json:"category" enum:"user,environment,quirk,project,observation"`
    Content  string   `json:"content" minLength:"10" maxLength:"5000"`
    Key      string   `json:"key,omitempty" pattern:"^[a-z][a-z0-9_-]*$"`
    Priority int      `json:"priority,omitempty" minimum:"1" maximum:"5"`
}

// In Anthropic tool definitions, add input_schema as JSON Schema
// Anthropic's tool_use content blocks already do this — we just need to
// enforce validation server-side when we receive the tool_call
```

**Implementation notes:**
- Anthropic's API already supports `input_schema` on tool definitions — we're just not validating on receipt
- Add a `validate.ToolInput(schema, args)` step in `tool_dispatch.go` before execution
- On validation failure, return a structured error back to the LLM instead of crashing

---

### 2.2 Adversarial Debate Pipeline ⭐⭐⭐

**What TradingAgents does:** Bull/Bear researchers argue N rounds, then a Research Manager synthesizes. Aggressive/Conservative/Neutral risk analysts debate N rounds, then Portfolio Manager decides. The debate state tracks `history`, `bull_history`, `bear_history`, `current_response`, `count` — all in a typed `InvestDebateState` / `RiskDebateState`.

**Why it matters for Mnemosyne:** Multi-agent Telepathy (Phase 14) needs agents to arrive at better decisions through structured disagreement. Current design has agents publish events and merge — but no mechanism for agents to challenge each other's conclusions.

**How to adapt:**
```
DebateState {
    History       []DebateTurn    // full transcript
    Positions     map[AgentID]string  // each agent's latest argument
    RoundCount    int
    MaxRounds     int
    Synthesizer   AgentID  // who makes the final call
}

DebateTurn {
    Speaker    AgentID
    Argument   string
    RespondingTo AgentID  // which prior argument this addresses
}
```

- Add a `DebateModerator` pipeline stage that:
  1. Collects initial positions from all subscribed agents
  2. Routes each agent's argument to opposing agents for rebuttal
  3. Counts rounds, terminates when max reached or consensus detected
  4. Passes full debate transcript to synthesizer agent
- The synthesizer uses structured output (see 2.1) to produce a final decision

**Use cases in Mnemosyne:**
- Two agents analyzing the same codebase from different angles (security vs performance)
- Conflicting memory entries: let agents argue about which is correct before merging
- Planning: one agent proposes, another critiques, synthesizer decides

---

### 2.3 Two-Tier LLM Routing (Deep Think vs Quick Think) ⭐⭐

**What TradingAgents does:** Config has `deep_think_llm` and `quick_think_llm`. Analysts and debaters use quick. Research Manager and Portfolio Manager use deep. This is a coarser version of Mnemosyne's smart routing.

**Why it matters:** Mnemosyne's smart routing (per-turn cost optimization) sometimes sends short prompts to the wrong model. TradingAgents' approach is simpler and more predictable: *role-based* routing instead of *prompt-length* routing.

**How to adapt:**
- Keep smart routing as the default
- Add **role-based routing hints** to the agent config:
  ```yaml
  agent_roles:
    analyst:
      model_preference: quick   # use quick_think_llm
    synthesizer:
      model_preference: deep     # use deep_think_llm  
    moderator:
      model_preference: deep
  ```
- This combines the best of both: role-based for predictable quality, smart routing for cost savings on easy turns

---

### 2.4 Append-Only Decision Log with Deferred Reflection ⭐⭐⭐

**What TradingAgents does:** `TradingMemoryLog` is an append-only markdown file. Each decision is stored as "pending." Later, after the holding period expires, a reflection pass:
1. Fetches actual returns (raw + alpha vs SPY)
2. Calls the LLM to reflect on whether the decision was correct
3. Updates the entry from `pending` → resolved with the reflection appended
4. Entries are rotated when `max_entries` is exceeded (oldest resolved entries dropped first; pending entries are never pruned)

**Why it matters for Mnemosyne:** This is exactly what Mnemosyne's memory system is missing — **outcome tracking and self-correction**. Currently memories are stored but never evaluated against reality. The hot block has no concept of "was this memory useful?"

**How to adapt:**
- Add an `outcome_status` column to the SQLite `memories` table: `pending | confirmed | contradicted | superseded`
- Add a `reflection` column: free-text LLM-generated assessment
- Add a cron job (or Telepathy event) that:
  1. Checks for pending memories older than some threshold
  2. Gathers evidence (new data, user feedback, downstream usage)
  3. Calls the LLM to reflect
  4. Updates status + reflection
- Hot block should **boost** confirmed memories and **suppress** contradicted ones
- This creates a feedback loop: memories that turned out wrong get deprioritized

**Key design detail from TradingAgents:** The `get_past_context()` method retrieves *same-ticker* decisions (full detail) and *cross-ticker* reflections (summary only). This is smart — it shows the agent its own track record on the specific topic, plus general lessons from other domains. Mnemosyne should do the same: same-category memories at full detail, cross-category at reflection-only.

---

### 2.5 Checkpoint/Resume with Crash Recovery ⭐⭐

**What TradingAgents does:** LangGraph checkpointing saves state after every graph node. If a run crashes (API timeout, rate limit), it resumes from the last successful step. Config: `checkpoint_enabled: true`.

**Why it matters:** Mnemosyne agents already have session persistence issues. A checkpoint system would let long agent runs survive:
- LLM API failures (rate limits, timeouts)
- Network interruptions (SSH tunnel drops)
- Daemon restarts

**How to adapt:**
- Store checkpoint state in SQLite (already have the DB)
- Checkpoint after every pipeline stage (not every turn — that's too granular)
- On agent startup, check for incomplete sessions and offer `/resume`
- Keep it simpler than LangGraph: just serialize `AgentState` + `PipelineStage` after each stage completes

---

### 2.6 Signal Extraction Without a Second LLM Call ⭐

**What TradingAgents does:** `SignalProcessor` originally used an LLM call to extract the rating from the Portfolio Manager's decision. They replaced it with a deterministic regex parser (`parse_rating`) because structured output guarantees the format.

**Lesson for Mnemosyne:** Don't use LLM calls where deterministic parsing works. Our `compactMessagesForNextTurn()` should do more deterministic stripping before falling back to LLM summarization. Current hot block truncation could also use smarter heuristics (strip old tool results, keep recent user messages) instead of blind char-limit chopping.

---

### 2.7 Atomic File Writes ⭐

**What TradingAgents does:** `update_with_outcome()` writes to a `.tmp` file then `os.replace()` — atomic on POSIX. Never corrupts the log even if the process dies mid-write.

**Lesson:** Mnemosyne's SQLite WAL mode already handles this for the DB, but any file-based operations (config, exported memories, session files) should use the same pattern. Check all `write_file` callers.

---

## 3. What NOT to Take from TradingAgents

| Pattern | Why Skip |
|---------|----------|
| **LangGraph StateGraph** | Too Python-specific, too magical. Mnemosyne's explicit Go pipeline is clearer and faster. |
| **Mutable dict state passing** | TradingAgents passes `state["investment_debate_state"]["count"]` through dicts — fragile. Mnemosyne's typed Go structs are better. |
| **yfinance as primary data source** | Not relevant — Mnemosyne isn't a trading tool. |
| **Sequential analyst pipeline** | Analysts run one after another. Mnemosyne's Telepathy events allow parallel analysis. We should stay parallel. |
| **LangChain dependency chain** | 20+ LangChain packages, frequent breaking changes. Mnemosyne's direct Anthropic/gRPC integration is cleaner. |
| **Markdown-based memory log** | Clever for a single-file Python project, but Mnemosyne already has SQLite FTS5 which is strictly superior for search/concurrency. |

---

## 4. Priority Ranking for Mnemosyne

| # | Idea | Effort | Impact |
|---|------|--------|--------|
| 1 | Structured tool input validation | 1-2 days | Kills empty-args crash, improves reliability |
| 2 | Append-only decision log + deferred reflection | 3-5 days | Self-correcting memory system, biggest UX win |
| 3 | Adversarial debate pipeline for multi-agent | 5-7 days | Core differentiator for Phase 14 Telepathy |
| 4 | Role-based LLM routing hints | 1 day | Simple config addition, pairs with smart routing |
| 5 | Checkpoint/resume for long runs | 3-4 days | Survives API failures, important for reliability |
| 6 | Deterministic parsing over LLM calls | 1 day | Small perf win, already partially done |
| 7 | Atomic file writes for non-DB operations | 0.5 days | Safety hygiene, easy win |

---

## 5. Mnemosyne Debug Session Notes

### Shell Script Bug
`~/.local/bin/mnemosyne` line 33 has an unmatched `"` — likely a corrupted script or encoding issue. Recreate the script from a known-good version.

### GetHotBlock Hang
The CLI calls `GetHotBlock` synchronously before each prompt cycle, AND the agent may call it again as a tool during execution. If the gRPC call to the daemon hangs (slow SSH tunnel, daemon busy), the entire agent blocks.

**Recommended fixes:**
1. Add `context.WithTimeout(5*time.Second)` to all gRPC calls in `client/client.go`
2. Remove redundant pre-prompt `GetHotBlock` call in `cli.go` if the agent will call it during execution anyway
3. Add gRPC call duration logging to diagnose slow calls
4. Consider making the initial `GetHotBlock` async (fire-and-forget with cached fallback)

---

*End of design doc. Generated 2026-04-30.*