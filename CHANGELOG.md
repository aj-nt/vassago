# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2025-05-09

### Added
- **Skills** — Shared procedural memory for agents: AddSkill, GetSkill, ListSkills, SearchSkills, RemoveSkill
  - Store layer with CRUD + FTS5 full-text search (migration v4)
  - gRPC handlers (5 new RPCs) and proto definitions
  - Client interface + null client implementations
  - Agent tool registrations with JSON schemas
  - GetSkillBlock injection at all 6 system-prompt entry points ensures agents always have relevant skills loaded
  - Skills are priority-sorted, category-filtered, and searchable like memories

### Fixed
- **Spiral detection** — ProgressDetectionHandler now detects oscillating tool spirals (A→B→A→B patterns) alongside duplicate-signature detection
- **Self-censorship spiral** — YIELD recovery messages no longer trigger further loop detection
- **RepetitiveLoopHandler** — Disabled (threshold=0); was catching legitimate multi-step work
- **Spiral detection aggressiveness** — Relaxed thresholds to allow legitimate multi-step agent workflows
- **Agent config** — chat_listen now included in default agent configuration
- **3 dead functions** removed (found by static analysis)

### Testing
- 1223 tests passing with `-race` detector
- New: 5 store tests, 7 gRPC handler tests, 5 client integration tests, 5 agent tool tests
- Store coverage at 83.7%, gRPC coverage at 83.9%

## [0.1.0] - 2025-05-08

First public release. Core daemon, agent, and CLI working.

### Added
- **Memory daemon** — Go gRPC server backed by SQLite (WAL mode) with FTS5 full-text search
- **Hot block** — Per-agent priority-sorted context window with configurable boost/suppress categories
- **Telepathy** — Real-time pub/sub: agents subscribe to memory changes from other agents
- **Self-exclusion** — Agents never see their own writes echo back (prevents feedback loops)
- **Consolidation** — Auto-expiry of idle observations; manual consolidation for spring cleaning
- **Unified CLI** — `mnemosyne init|start|stop|chat|status|health|memory|hot-block|config|version`
- **launchd integration** — macOS services auto-start via `mnemosyne init` + `mnemosyne start`
- **Progress-based spiral detection** — Detects when agent loops (same tools, same results) and yields
- **Streaming chat** — Real-time streamed responses with tool-call approval prompts
- **Multi-provider** — Anthropic, OpenAI, Ollama with automatic fallback and demotion
- **Tool system** — 25+ tools: memory, files, terminal, web, sessions, todos, cron, messaging, delegation, browser, vision, image gen
- **Hermes migration** — `mnemosyne-import` to bring data from Hermes
- **Apache 2.0 protocol** — proto/ and client/ are permissively licensed for third-party clients

### Known Limitations
- No Docker image yet
- No Python SDK on PyPI (gRPC client works, pip package planned)
- Single-user — no auth/ACL system
- No web UI (CLI only)
- Browser tool requires ChromeDP installed

### Testing
- 1175+ tests passing with `-race` detector
- Red-team QA pass on every merge
- Integration tests with bufconn gRPC mock

[0.2.0]: https://github.com/aj-nt/mnemosyne/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/aj-nt/mnemosyne/releases/tag/v0.1.0