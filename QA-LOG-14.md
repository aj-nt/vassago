# QA Audit Log — Post-Phase 14

**Date:** 2026-05-01
**Auditor:** AJ + Hermes
**Branch:** main (commit 9160b1d)
**Tests:** 378 passing with -race

---

## Findings

### CRITICAL

(None)

### HIGH

#### H1: Phase 14 features disabled in production configs
- **File:** `configs/agent-anthropic.yaml`, `~/mnemosyne-studio.yaml`
- **Details:** Neither config file includes `delegation:` or `scheduler:` sections. Both Phase 14C (Delegation) and Phase 14D (Scheduled Tasks) have `enabled: false` as defaults, so they are silently disabled in production. The features are built and tested but won't activate without explicit config.
- **Fix:** Add delegation and scheduler sections to both config files with `enabled: true` (and sensible defaults).

#### H2: context.Background() in heartbeat/subscription goroutines
- **File:** `client/agent.go:81`, `client/agent.go:191`, `agent/cmd/mnemosyne-agent/main.go:92`
- **Details:** The heartbeat loop creates its own context (`context.WithCancel(context.Background())`), which is correct for the goroutine lifecycle. However, `heartbeatLoop` uses `context.WithTimeout(context.Background(), 5*time.Second)` for each heartbeat call (line 191), which is fine for timeout but means heartbeats don't inherit any parent cancellation signal beyond the goroutine's own context. The subscription in `main.go:92` uses `context.Background()` directly, meaning if the agent needs to shut down mid-subscription, cancellation relies solely on `streamCancel()` from `Close()`. This works but is fragile if `Close()` is never called (e.g., SIGKILL).
- **Fix:** Low urgency — the current design is correct for graceful shutdown. Could derive subscription context from a top-level agent context for cleaner cancellation chains.

### MEDIUM

#### M1: Low test coverage in key packages
- `agent/` — 38.1%
- `client/` — 37.0%
- `daemon/internal/daemon` — 25.2%
- `daemon/internal/grpc` — 46.8%
- `daemon/internal/migration` — 42.6%
- `agent/interface/` — 45.6%
- **Fix:** Target these packages for increased integration testing in Phase 15.

#### M2: SetHotBlockCache not thread-safe
- **File:** `agent/agent.go:284-285`
- **Details:** `SetHotBlockCache(cache)` directly writes `a.hotBlockCache = cache` without a mutex or atomic. If called concurrently with `GetHotBlock()` (which reads `a.hotBlockCache`), there's a data race. In practice, `SetHotBlockCache` is called once during startup before the agent processes any turns, so this is not a live bug. But it's not safe by Go's memory model.
- **Fix:** Use `sync/atomic.Value` or add a mutex to the Agent struct for the cache pointer.

#### M3: fmt.Sprintf in hot paths
- **File:** `client/agent.go:54`, `daemon/internal/store/consolidate.go:118`
- **Details:** fmt.Sprintf in the hot path of hot block construction or consolidation could be optimized with strings.Builder. Not a performance issue at current scale, but noted for future.
- **Fix:** Low priority. Optimize if consolidate becomes a bottleneck.

### LOW

#### L1: Stale roadmap items — completed work still marked `[ ]`
- **File:** `ROADMAP.md`
- **Details:**
  - Line 219: `client/hotblock_cache.go` — marked `[ ]` but file exists and is tested (Phase 14B) ✅ DONE
  - Line 345: `agent/tools/delegate.go` — marked `[ ]` but file exists and is tested (Phase 14C) ✅ DONE
  - Line 566: `Kills empty-args crash permanently` — marked `[ ]` but the fix is in `tool_dispatch.go:48-53` (Phase 14E) ✅ DONE

#### L2: TODO comments in import tool
- **File:** `daemon/cmd/mnemosyne-import/main.go:36`, `daemon/internal/migration/hermes.go:272`
- **Details:** Two TODO comments — dry-run mode and session ID mapping. These are in the import tool, which is only used for one-time data migration.
- **Fix:** Low priority — remove or implement when Hermes migration is run for real.

#### L3: Phase 1 CI item still unchecked
- **File:** `ROADMAP.md:166`
- **Details:** CI pipeline (`go test -race ./...` on push) is documented in `ci.yml` and `qa-gate.yml` but the checkbox isn't marked. The workflows exist and work.
- **Fix:** Mark as done.

#### L4: log.Fatalf in main packages only
- **Files:** `agent/cmd/mnemosyne-agent/main.go` (5 uses), `daemon/cmd/mnemosyne-import/main.go` (2 uses)
- **Details:** All log.Fatal/log.Fatalf calls are in `main` packages, which is acceptable. No library code uses log.Fatal.

### INFO

#### I1: All 378 tests pass with -race
No failures, no flakes detected across 5 consecutive runs of tools/scheduler/client packages.

#### I2: go vet produces no warnings

#### I3: No hardcoded credentials or API keys in the codebase

#### I4: gRPC auth is implemented (API key via interceptor)

#### I5: File path traversal is properly sanitized (filepath.Join + checks)

#### I6: No panics in production code (only in test files)

#### I7: Scheduler shutdown is clean — no goroutine leaks

#### I8: Delegation tool has proper 2-minute timeout with context cancellation

#### I9: HotBlockCache is thread-safe (sync.RWMutex)
The `hotBlockCache` struct itself is well-protected. The thread-safety gap is only on the pointer assignment in `SetHotBlockCache`, which is called once at startup.

#### I10: Empty args crash is already fixed (tool_dispatch.go:48-53)
Null/empty JSON args are normalized to `{}` before parsing.

#### I11: Proto generation requires protoc-gen-go tools
`make generate` fails if protoc tools aren't installed, but the generated files are committed and current. This is a build-time dependency, not a runtime issue.

---

## Roadmap Updates Needed

These items should be marked `[x]`:

1. **Line 219**: `client/hotblock_cache.go` — local cache exists and is tested (14B) ✅
2. **Line 345**: `agent/tools/delegate.go` — delegation tool built in 14C ✅  
3. **Line 566**: `Kills empty-args crash permanently` — fixed in 14E ✅
4. **Line 166**: CI pipeline — ci.yml and qa-gate.yml exist and work ✅

## Config Updates Needed

Both `configs/agent-anthropic.yaml` and `~/mnemosyne-studio.yaml` need:

```yaml
delegation:
  enabled: true
  max_depth: 1
  max_turns: 10

scheduler:
  enabled: true
  interval: 3600  # 1 hour in seconds
```

---

## Red Team QA Pass (2026-05-01)

### Attack Surfaces Tested
- Daemon store layer: nil/empty fields, oversized content, SQL injection, FTS5 injection, concurrent writes, boundary timestamps, priority overflow, special chars, spoofed agent IDs, hot block starvation, char limit overflow
- Agent tool validation: SQL injection, type confusion, missing required fields, null bytes, very long input, enum violations
- Agent tool dispatch: unknown tools, empty/null args, wrong types, SQL injection in tool names, invalid JSON, rapid-fire tool calls
- gRPC service: stub tests (need real server integration)

### RED TEAM FINDINGS

#### MEDIUM: Empty content accepted by AddMemory
- The store has no minimum content length. AddMemory with empty string content succeeds.
- The tool handler does check `content == ""` and returns an error, so the tool-level defense works.
- The store-level validation rejects it with a content-length check (max 100K chars).
- **Risk**: LOW — the tool handler catches it before the store. But if called directly via gRPC, empty content is allowed.

#### MEDIUM: Null bytes not rejected by validation
- ValidateArgs does not reject strings containing `\x00`.
- SQLite handles null bytes in TEXT columns (stores as-is), but they can cause issues in FTS5 indexing and JSON serialization.
- **Risk**: MEDIUM — null bytes in category/key could corrupt FTS5 indexes.

#### LOW: ApplyDefaults doesn't apply all optional defaults
- The AddMemory schema defines `default: 20` for `limit` and `default: 3` for `priority`, but ApplyDefaults only checks for `"default"` keys at the top level of `properties[name]`.
- Nested defaults (like `limit` inside `properties.limit.default`) ARE applied, but some defaults like `priority` use the handler's fallback instead.
- **Risk**: LOW — handlers have their own default fallbacks.

#### LOW: Category not marked as required in AddMemory schema
- AddMemory's schema only marks `key` and `content` as required. `category` and `target` have defaults in the handler.
- If called via gRPC without category, it gets the handler's default ("environment").
- **Risk**: LOW — defaults are applied correctly by the handler.

#### INFO: Content size limit exists (100K chars)
- Store rejects content > 100,000 characters with `validation: content too long: N chars, max 100000`.
- This is a good defensive control.

#### INFO: Priority validation works (1-5)
- Store rejects priority 0, -1, and 999 with `validation: invalid priority N: must be 1-5`.
- Agent tool validation also enforces this range.

### Tests Added
- `daemon/internal/store/redteam_test.go` — 11 tests
- `agent/tools/redteam_test.go` — 8 tests
- `agent/pipeline/redteam_test.go` — 8 tests
- `daemon/internal/grpc/redteam_test.go` — 9 stub tests (need integration)