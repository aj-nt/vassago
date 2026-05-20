# Orchestrator v1 — Go Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Port the orchestrator spike (`orchestrator_v3.py`) to a standalone Go binary that decomposes high-level goals into task DAGs via LLM, dispatches them to Vassago daemon, polls for completion, synthesizes results, and publishes a final report.

**Architecture:** Separate binary `cmd/orchestrator/` using shared `daemon/internal/ollama/` package. Linear pipeline (not state machine for v1 — matches the spike's proven simplicity). LLM calls via Template Method + Strategy so providers are swappable. Hardcoded fallback decomposition and basic cross-reference synthesis when LLM is unavailable.

**Tech Stack:** Go 1.21+, gRPC (vassago-sdk client), Ollama OpenAI-compatible endpoint, YAML config, slog logging.

**Files created:**
- `daemon/internal/ollama/ollama.go` — extracted OllamaClient (shared package)
- `daemon/internal/ollama/ollama_test.go` — tests for existing behavior
- `cmd/orchestrator/main.go` — entry point
- `cmd/orchestrator/config.go` — config + defaults
- `cmd/orchestrator/config_test.go` — config tests
- `cmd/orchestrator/decomposer.go` — LLM decomposition with fallback
- `cmd/orchestrator/decomposer_test.go` — decomposition tests
- `cmd/orchestrator/orchestrator.go` — pipeline orchestration
- `cmd/orchestrator/orchestrator_test.go` — orchestration tests
- `cmd/orchestrator/synthesis.go` — LLM synthesis with fallback
- `cmd/orchestrator/synthesis_test.go` — synthesis tests

**Files modified:**
- `cmd/vagent/ollama.go` — replaced with import from `daemon/internal/ollama/`
- `cmd/vagent/worker_runner.go` — updated imports
- `Makefile` — add orchestrator build target

---

### Task 0: Extract OllamaClient to shared package

**Objective:** Move the OllamaClient from `cmd/vagent/ollama.go` to `daemon/internal/ollama/` so both vagent and orchestrator can use it.

**Files:**
- Create: `daemon/internal/ollama/ollama.go`
- Create: `daemon/internal/ollama/ollama_test.go`
- Modify: `cmd/vagent/ollama.go` (replace with re-export)
- Modify: `cmd/vagent/worker_runner.go` (update imports)
- Modify: `cmd/vagent/worker_test.go` (update imports)

**Step 1: Write failing test for OllamaClient creation**

```go
// daemon/internal/ollama/ollama_test.go
package ollama

import (
    "testing"
)

func TestNewClient_SetsDefaults(t *testing.T) {
    c := NewClient(Config{
        BaseURL:     "http://localhost:11434/v1",
        Model:       "test-model",
        MaxTokens:   512,
        Temperature: 0.5,
        TimeoutSec:  30,
    })
    if c == nil {
        t.Fatal("expected non-nil client")
    }
    if c.model != "test-model" {
        t.Errorf("model = %q, want %q", c.model, "test-model")
    }
    if c.maxTokens != 512 {
        t.Errorf("maxTokens = %d, want 512", c.maxTokens)
    }
}
```

**Step 2: Run test to verify failure** — `go test -run TestNewClient -v ./daemon/internal/ollama/` — FAIL (package doesn't exist)

**Step 3: Write daemon/internal/ollama/ollama.go**

Copy the existing `cmd/vagent/ollama.go` as-is, rename `newOllamaClient` → `NewClient`, export `OllamaClient` struct (keep fields unexported, use constructor). Export `Config`, `Message`, `ChatRequest`, `ChatResponse`, `Choice`. Use `Config` instead of `OllamaConfig`.

**Step 4: Run test to verify pass** — should pass.

**Step 5: Add test for GLM reasoning fallback**

```go
func TestChat_GLMReasoningFallback(t *testing.T) {
    // Test with a mock HTTP server that returns empty content + reasoning_content
    // Verify the client extracts reasoning_content
}
```

**Step 6: Run all ollama tests** — `go test -race -count=1 ./daemon/internal/ollama/` — PASS

**Step 7: Update cmd/vagent/ollama.go to re-export**

```go
package main

import "github.com/aj-nt/vassago/daemon/internal/ollama"

// Re-export for backward compatibility within the vagent package.
type OllamaClient = ollama.Client
type OllamaConfig = ollama.Config
type ollamaMessage = ollama.Message

func newOllamaClient(cfg OllamaConfig) *OllamaClient {
    return ollama.NewClient(cfg)
}
```

**Step 8: Update worker_runner.go imports** — replace `cmd/vagent/ollama` references with shared package. Since we re-export, the types resolve correctly. But `executeTask` uses `ollama.Chat` on `*OllamaClient` — verify it works through the type alias.

Actually: re-export is fragile. Better approach: just change `worker_runner.go` to import `daemon/internal/ollama` directly and use `ollama.Client`, `ollama.Config`, `ollama.Message`. Delete `cmd/vagent/ollama.go`.

**Step 9: Build and test everything**

```bash
go build ./cmd/vagent/ ./daemon/internal/ollama/
go test -race -count=1 ./cmd/vagent/ ./daemon/internal/ollama/
```

**Step 10: Commit**

```bash
git add daemon/internal/ollama/ cmd/vagent/
git commit -m "refactor: extract OllamaClient to daemon/internal/ollama/"
```

---

### Task 1: Orchestrator config and entry point

**Objective:** Create the binary skeleton — config loading, CLI parsing, daemon connection.

**Files:**
- Create: `cmd/orchestrator/config.go`
- Create: `cmd/orchestrator/config_test.go`
- Create: `cmd/orchestrator/main.go`

**Step 1: Write failing test for config defaults**

```go
// cmd/orchestrator/config_test.go
package main

import (
    "os"
    "path/filepath"
    "testing"
)

func TestDefaultConfig(t *testing.T) {
    cfg := DefaultConfig()
    if cfg.DaemonAddr == "" {
        t.Error("DaemonAddr should have default")
    }
    if cfg.PollInterval <= 0 {
        t.Error("PollInterval should be positive")
    }
    if cfg.MaxWait <= 0 {
        t.Error("MaxWait should be positive")
    }
    if cfg.Decomposer.Model == "" {
        t.Error("Decomposer.Model should have default")
    }
    if cfg.Synthesizer.Model == "" {
        t.Error("Synthesizer.Model should have default")
    }
}

func TestLoadConfig_CreatesDefault(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "orchestrator.yaml")
    os.Setenv("ORCHESTRATOR_CONFIG", path)
    defer os.Unsetenv("ORCHESTRATOR_CONFIG")

    // Config doesn't exist — should create default
    cfg := loadConfig()
    if cfg == nil {
        t.Fatal("expected config")
    }
    if _, err := os.Stat(path); err != nil {
        t.Errorf("default config not created at %s: %v", path, err)
    }
}
```

**Step 2: Run test to verify failure** — FAIL (package doesn't compile)

**Step 3: Write config.go**

```go
// cmd/orchestrator/config.go
package main

import (
    "os"
    "path/filepath"

    "gopkg.in/yaml.v3"
)

type Config struct {
    DaemonAddr   string         `yaml:"daemon_addr"`
    RepoPath     string         `yaml:"repo_path"`
    PollInterval int            `yaml:"poll_interval_sec"`
    MaxWait      int            `yaml:"max_wait_sec"`
    Decomposer   LLMConfig      `yaml:"decomposer"`
    Synthesizer  LLMConfig      `yaml:"synthesizer"`
}

type LLMConfig struct {
    Provider    string  `yaml:"provider"`
    Model       string  `yaml:"model"`
    BaseURL     string  `yaml:"base_url"`
    MaxTokens   int     `yaml:"max_tokens"`
    Temperature float64 `yaml:"temperature"`
    TimeoutSec  int     `yaml:"timeout_sec"`
}

func DefaultConfig() *Config {
    return &Config{
        DaemonAddr:   "localhost:50051",
        PollInterval: 3,
        MaxWait:      300,
        Decomposer: LLMConfig{
            Provider:    "ollama",
            Model:       "glm-5.1:cloud",
            BaseURL:     "http://localhost:11434/v1",
            MaxTokens:   2000,
            Temperature: 0.1,
            TimeoutSec:  120,
        },
        Synthesizer: LLMConfig{
            Provider:    "ollama",
            Model:       "glm-5.1:cloud",
            BaseURL:     "http://localhost:11434/v1",
            MaxTokens:   4096,
            Temperature: 0.2,
            TimeoutSec:  120,
        },
    }
}
```

**Step 4: Implement loadConfig() with env var and defaults**

**Step 5: Write main.go** — parse CLI args, load config, connect daemon, run pipeline. Skeleton only at this point.

**Step 6: Build and test**

```bash
go build ./cmd/orchestrator/
go test -race -count=1 ./cmd/orchestrator/
```

**Step 7: Commit**

```bash
git add cmd/orchestrator/
git commit -m "feat: orchestrator config and entry point skeleton"
```

---

### Task 2: Decomposer — LLM decomposition with fallback

**Objective:** Port `decompose_goal()` and `_hardcoded_decompose()` from `orchestrator_v3.py` to Go, using the shared Ollama client.

**Files:**
- Create: `cmd/orchestrator/decomposer.go`
- Create: `cmd/orchestrator/decomposer_test.go`

**Step 1: Write failing test for hardcoded decomposition**

```go
func TestHardcodedDecompose(t *testing.T) {
    tasks := hardcodedDecompose("audit my codebase", "/tmp/repo")
    if len(tasks) != 3 {
        t.Fatalf("expected 3 tasks, got %d", len(tasks))
    }
    // First two should be leaf tasks (ready, no parents)
    for i := 0; i < 2; i++ {
        if tasks[i].Status != "ready" {
            t.Errorf("task %d status = %s, want ready", i, tasks[i].Status)
        }
        if len(tasks[i].ParentIDs) != 0 {
            t.Errorf("task %d has parents, want none", i)
        }
    }
    // Third should be synthesis (blocked, two parents)
    synth := tasks[2]
    if synth.Status != "blocked" {
        t.Errorf("synthesis status = %s, want blocked", synth.Status)
    }
    if synth.AgentType != "synthesis" {
        t.Errorf("synthesis agent_type = %s, want synthesis", synth.AgentType)
    }
    if len(synth.ParentIDs) != 2 {
        t.Errorf("synthesis parent count = %d, want 2", len(synth.ParentIDs))
    }
}
```

**Step 2: Run test to verify failure** — FAIL

**Step 3: Write decomposer.go**

```go
package main

import (
    "fmt"
    "github.com/aj-nt/vassago/daemon/internal/ollama"
    "github.com/aj-nt/vassago-sdk/client"
    "github.com/google/uuid"
)

// DecomposerStrategy defines how to decompose a goal into tasks.
type DecomposerStrategy interface {
    Decompose(goal, repoPath string, codebase map[string]interface{}) ([]client.TaskSpec, error)
}

// LLMDecomposer calls the LLM for decomposition.
type LLMDecomposer struct {
    client  *ollama.Client
    catalog map[string]AgentSpec
}

func (d *LLMDecomposer) Decompose(goal, repoPath string, codebase map[string]interface{}) ([]client.TaskSpec, error) {
    // Build prompt, call LLM, parse JSON, return tasks
    // On failure, fall back to hardcodedDecompose
}

// Decomposer orchestrates the decomposition pipeline.
type Decomposer struct {
    strategy DecomposerStrategy
}

func (d *Decomposer) Decompose(goal, repoPath string) ([]client.TaskSpec, error) {
    // Scan codebase, call strategy, validate result
}

type AgentSpec struct {
    Description string   `json:"description"`
    Tools       []string `json:"tools"`
    Languages   []string `json:"languages"`
    Produces    string   `json:"produces"`
}

type TaskSpec struct {
    TaskID     string   `json:"task_id"`
    ParentIDs  []string `json:"parent_ids"`
    Status     string   `json:"status"`
    Goal       string   `json:"goal"`
    AgentType  string   `json:"agent_type"`
    Context    map[string]interface{} `json:"context"`
}
```

**Step 4: Implement hardcodedDecompose — direct port from spike**

**Step 5: Implement LLMDecomposer.Decompose** — port the DECOMPOSE_PROMPT and JSON parsing logic. On LLM failure, fall back to hardcoded.

**Step 6: Write test for LLM failure falling back to hardcoded**

**Step 7: Run all decomposer tests** — `go test -race -count=1 -run TestDecompos ./cmd/orchestrator/` — PASS

**Step 8: Commit**

```bash
git add cmd/orchestrator/decomposer*
git commit -m "feat: LLM goal decomposition with hardcoded fallback"
```

---

### Task 3: Synthesis engine

**Objective:** Port `synthesize_results()` and `_basic_cross_reference()` from the spike to Go.

**Files:**
- Create: `cmd/orchestrator/synthesis.go`
- Create: `cmd/orchestrator/synthesis_test.go`

**Step 1: Write failing test for basic cross-reference**

```go
func TestBasicCrossReference(t *testing.T) {
    results := map[string]*TaskResult{
        "task_1": {
            TaskID:   "task_1",
            Status:   "success",
            Findings: map[string]interface{}{
                "bare_panics": 3,
                "god_objects": 2,
            },
        },
        "task_2": {
            TaskID: "task_2",
            Status: "success",
            Findings: map[string]interface{}{
                "exit_code": 1,
            },
        },
    }
    synth := basicCrossReference(results)
    if synth.Title == "" {
        t.Error("synthesis should have a title")
    }
    if len(synth.HighFindings) == 0 {
        t.Error("should have findings from task_2 (non-zero exit code)")
    }
}
```

**Step 2: Run test to verify failure** — FAIL

**Step 3: Write synthesis.go**

```go
package main

import (
    "encoding/json"
    "fmt"
    "github.com/aj-nt/vassago/daemon/internal/ollama"
)

type SynthesisResult struct {
    Title            string               `json:"title"`
    ExecutiveSummary string               `json:"executive_summary"`
    HighFindings     []Finding            `json:"high_findings"`
    MediumFindings   []Finding            `json:"medium_findings"`
    LowFindings      []Finding            `json:"low_findings"`
    CrossReferences  []CrossRefInsight    `json:"cross_reference_insights"`
    CodebaseHealth   map[string]string    `json:"codebase_health"`
    Recommendations  []Recommendation     `json:"top_recommendations"`
}

type SynthesisStrategy interface {
    Synthesize(results map[string]*TaskResult) (*SynthesisResult, error)
}

type LLMSynthesizer struct {
    client *ollama.Client
}

type FallbackSynthesizer struct{}

// SynthesisEngine tries LLM first, falls back to basic.
type SynthesisEngine struct {
    llm      *LLMSynthesizer
    fallback *FallbackSynthesizer
}
```

**Step 4: Implement LLMSynthesizer.Synthesize** — port SYNTHESIS_PROMPT from spike, call LLM, parse JSON output

**Step 5: Implement FallbackSynthesizer.Synthesize** — port `_basic_cross_reference()` logic

**Step 6: Write tests: LLM failure falls back, schema matching, empty results**

**Step 7: Run all synthesis tests** — PASS

**Step 8: Commit**

```bash
git add cmd/orchestrator/synthesis*
git commit -m "feat: LLM synthesis engine with fallback cross-reference"
```

---

### Task 4: Orchestrator pipeline

**Objective:** Wire everything together — the linear pipeline that receives a goal, decomposes, dispatches, polls, synthesizes, and publishes.

**Files:**
- Create: `cmd/orchestrator/orchestrator.go`
- Create: `cmd/orchestrator/orchestrator_test.go`
- Modify: `cmd/orchestrator/main.go`

**Step 1: Write integration test with mock daemon**

```go
func TestOrchestrator_Pipeline_DryRun(t *testing.T) {
    // Connect to test daemon, run with --dry-run
    // Verify it decomposes goal without dispatching
}

func TestOrchestrator_Pipeline_Full(t *testing.T) {
    // Start test daemon, create mock agent, run full pipeline
    // Verify tasks created, synthesis published
}
```

**Step 2: Run test to verify failure** — FAIL

**Step 3: Write orchestrator.go**

```go
package main

type Orchestrator struct {
    cfg          *Config
    daemon       client.MnemoClient
    decomposer   *Decomposer
    synthesis    *SynthesisEngine
    repoPath     string
}

func (o *Orchestrator) Run(goal string) (*SynthesisResult, error) {
    // Phase 1: Decompose
    // Phase 2: Dispatch tasks to daemon
    // Phase 3: Poll for completions (with TTL)
    // Phase 4: Collect results from blackboard
    // Phase 5: Synthesize
    // Phase 6: Publish final report
    // Phase 7: Return result
}
```

**Step 4: Implement codebase scanner** — port `scan_codebase()` from spike (find Go files, packages, largest files). Non-blocking — best-effort context for decomposer, not required.

**Step 5: Implement polling loop** — for each leaf task, poll until done/failed or timeout. Handle TTL expiry via `ResetTimedOutTasks`. On timeout, proceed with whatever results exist.

**Step 6: Implement result collection** — query blackboard for results matching task output keys.

**Step 7: Wire into main.go** — parse `--goal`, `--repo`, `--dry-run`, `--addr`, `--timeout` flags. Create Orchestrator, call Run(), print summary.

**Step 8: Run tests** — `go test -race -count=1 ./cmd/orchestrator/` — PASS

**Step 9: Commit**

```bash
git add cmd/orchestrator/
git commit -m "feat: orchestrator pipeline — decompose, dispatch, poll, synthesize"
```

---

### Task 5: Build and E2E smoke test

**Objective:** Build the binary, update Makefile, run the spike's E2E test scenario.

**Files:**
- Modify: `Makefile`

**Step 1: Add orchestrator to Makefile build target**

```makefile
build:
    # ... existing ...
    go build -o $(BINARY_DIR)/orchestrator ./cmd/orchestrator
```

**Step 2: Build**

```bash
make build
```

**Step 3: E2E smoke test**

```bash
# Start daemon on temp port
# Run orchestrator: ./bin/orchestrator --dry-run "audit the vassago codebase"
# Verify decomposition output
# Run orchestrator for real with a vagent worker
# Verify tasks created, synthesis published
```

**Step 4: Run full test suite**

```bash
go test -race -count=1 ./cmd/orchestrator/ ./cmd/vagent/ ./daemon/internal/ollama/
```

**Step 5: Commit**

```bash
git add Makefile
git commit -m "feat: add orchestrator binary to build targets"
```

---

### Task 6: Run full suite, review, QA

**Step 1: Full test suite**

```bash
go test -race -count=1 ./...
```

**Step 2: Lint**

```bash
go vet ./...
```

**Step 3: Build all**

```bash
make build
```

**Step 4: Red team code review** — load `red-team-code-review` skill, review all new files

**Step 5: Fix any issues, commit**

**Step 6: Push**

```bash
git push origin main
```

---

## Summary

**7 commits** expected:
1. refactor: extract OllamaClient to daemon/internal/ollama/
2. feat: orchestrator config and entry point skeleton
3. feat: LLM goal decomposition with hardcoded fallback
4. feat: LLM synthesis engine with fallback cross-reference
5. feat: orchestrator pipeline — decompose, dispatch, poll, synthesize
6. feat: add orchestrator binary to build targets
7. chore: build verification and QA fixes (if any)

**Binary:** `bin/orchestrator` — standalone, connects to daemon via gRPC.

**Usage:**
```bash
./bin/orchestrator "audit the vassago codebase"
./bin/orchestrator --dry-run "review my project"
./bin/orchestrator --addr=studio:50051 --repo=/path/to/repo "analyze security"
```

**What we're NOT building in v1:**
- State Pattern (linear pipeline is sufficient for a single orchestrator)
- Observer/pub-sub DAG advancement (polling works)
- Circuit Breaker (no real failing agents yet)
- Chain of Responsibility for agent matching (catalog is hardcoded)
- Webhook/gRPC-push task dispatch (polling is baseline)
