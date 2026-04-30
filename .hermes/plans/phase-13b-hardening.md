# Phase 13B: Hardening & Resilience Pass

## Critical Findings (from audit)

### P0 — Must Fix Before Daily Use
1. **No retry on transient failures** — `ShouldRetry` is set by `classifyHTTPResponse` but never consumed. 429/503 errors are returned as errors with no retry.
2. **Bug: 5xx ShouldFallback checks wrong code** — Line 29 of http_helpers.go: `ShouldFallback: resp.StatusCode == 429` should be `resp.StatusCode >= 500`.
3. **No graceful shutdown / message flush** — Agent loop exit doesn't guarantee messages are persisted. SIGINT could lose the last turn.
4. **No streaming failure recovery** — If stream dies mid-turn (network drop), the agent returns error immediately. No retry or graceful fallback.

### P1 — High Priority
5. **RunStreaming has 0% test coverage** — The main interactive path is completely untested.
6. **No gRPC server connection limits/timeouts** — Server accepts unlimited connections with no idle timeout.
7. **No SQLite health check** — No ping/PRAGMA check before operations. If DB gets corrupted, every call silently fails.
8. **No HTTP client timeout on provider calls** — Long-running API calls can hang forever.

### P2 — Medium Priority
9. **Config LoadConfig/Validate/ToAgentConfig 0% coverage** — Config parsing untested.
10. **CLI RunInteractive/RunOneShot 0% coverage** — Hard to unit test but integration tests would help.
11. **formatResumedSession 0% coverage** — Simple to test.

## Implementation Plan

### Step 1: Provider Retry Logic (P0 #1, #2, #4)
- Add retry-with-backoff to `RunWithContext` and `RunStreaming`
- Fix the 5xx ShouldFallback bug
- Retry on 429 with exponential backoff (max 3 attempts)
- On streaming failure, fall back to non-streaming before retrying

### Step 2: Graceful Shutdown (P0 #3)
- Add signal handling to CLI (SIGINT/SIGTERM)
- On signal: flush messages to daemon, then exit
- Add `Shutdown()` method to Agent that persists and cleans up

### Step 3: HTTP Client Timeouts (P1 #8)
- Set 120s timeout on HTTP client used by Anthropic/OpenAI providers
- Respect context cancellation in provider calls

### Step 4: gRPC Server Hardening (P1 #6)
- Add max connection limit (100)
- Add idle connection timeout (5 min)
- Add max receive message size (10MB)

### Step 5: SQLite Health Check (P1 #7)
- Add `Ping()` method to Store
- gRPC service calls Ping() before critical operations
- If Ping fails, return UNAVAILABLE status

### Step 6: Coverage Improvements (P2)
- Tests for LoadConfig, Validate, ToAgentConfig
- Tests for formatResumedSession
- Integration test skeleton for RunStreaming

## TDD Approach
Write tests first for each fix, then implement. Every fix gets at least one test.