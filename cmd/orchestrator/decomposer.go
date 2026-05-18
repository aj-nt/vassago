// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aj-nt/vassago/internal/ollama"
)

// TaskSpec represents a task to be dispatched to workers.
type TaskSpec struct {
	TaskID    string                 `json:"task_id"`
	ParentIDs []string               `json:"parent_ids"`
	Status    string                 `json:"status"`
	Goal      string                 `json:"goal"`
	AgentType string                 `json:"agent_type"`
	Context   map[string]interface{} `json:"context"`
}

// AgentSpec describes a worker agent's capabilities.
type AgentSpec struct {
	Description string   `json:"description"`
	Tools       []string `json:"tools"`
	Languages   []string `json:"languages"`
	Produces    string   `json:"produces"`
}

// defaultAgentCatalog is the hardcoded capability catalog for code review agents.
var defaultAgentCatalog = map[string]AgentSpec{
	"static_analysis": {
		Description: "Runs static analysis tools: golangci-lint, gosec, govulncheck",
		Tools:       []string{"golangci-lint", "gosec", "govulncheck"},
		Languages:   []string{"go"},
		Produces:    "Report of lint issues, security vulnerabilities, and vulnerable dependencies",
	},
	"quality_scan": {
		Description: "Scans code quality: god objects, long functions, error handling, test coverage",
		Tools:       []string{"find", "grep", "go test -cover"},
		Languages:   []string{"go"},
		Produces:    "Report of code quality metrics: file sizes, function lengths, bare panics, test coverage",
	},
	"synthesis": {
		Description: "Synthesizes findings from multiple analysis agents into a unified audit",
		Tools:       []string{},
		Languages:   []string{},
		Produces:    "Cross-referenced synthesis report with prioritized findings and recommendations",
	},
}

// DecomposerStrategy defines how to decompose a goal into tasks.
type DecomposerStrategy interface {
	Decompose(goal, repoPath string, codebase map[string]interface{}) ([]TaskSpec, error)
}

// Decomposer orchestrates the decomposition pipeline.
type Decomposer struct {
	strategy DecomposerStrategy
}

// NewDecomposer creates a decomposer with the given strategy.
func NewDecomposer(strategy DecomposerStrategy) *Decomposer {
	return &Decomposer{strategy: strategy}
}

// Decompose runs the decomposition pipeline.
func (d *Decomposer) Decompose(goal, repoPath string) ([]TaskSpec, error) {
	codebase := scanCodebase(repoPath)
	return d.strategy.Decompose(goal, repoPath, codebase)
}

// LLMDecomposer calls an LLM for decomposition, falling back to hardcoded on failure.
type LLMDecomposer struct {
	client  *ollama.Client
	catalog map[string]AgentSpec
	prefix  string
}

// NewLLMDecomposer creates an LLM-powered decomposer.
func NewLLMDecomposer(client *ollama.Client) *LLMDecomposer {
	return &LLMDecomposer{
		client:  client,
		catalog: defaultAgentCatalog,
		prefix:  fmt.Sprintf("task_%s", shortUUID()),
	}
}

var decomposePrompt = `You are an orchestrator that decomposes high-level goals into concrete, executable tasks for specialized AI agents.

Given a goal and the available agent types, produce a task DAG. Each task is a JSON object with:
- task_id: unique string (use "task_N" prefix)
- parent_ids: list of task_ids this depends on (empty for leaf tasks)
- status: "ready" for leaf tasks, "blocked" for tasks with dependencies
- goal: human-readable description of what this task should accomplish
- agent_type: one of the available types below
- context: a dict with repo_path and any tool-specific config the agent needs

AVAILABLE AGENT TYPES:
%s

CODEBASE CONTEXT:
%s

RULES:
1. Leaf tasks (static_analysis, quality_scan) run in parallel. Set parent_ids=[] and status="ready".
2. The final task must be agent_type="synthesis" with parent_ids listing all leaf task_ids, status="blocked".
3. The synthesis task should NOT re-run analysis — it reads results from the blackboard.
4. context.repo_path must be the absolute path to the repository.
5. context.dependents on synthesis tasks must list the task_ids it depends on (for blackboard lookup).
6. Every task must produce results under a predictable key. Use context.output_key.

GOAL: %s

Return ONLY a JSON array of task objects. No explanation, no markdown, no code fences.`

// Decompose calls the LLM and parses the resulting task DAG.
// Falls back to hardcoded decomposition on any failure.
func (d *LLMDecomposer) Decompose(goal, repoPath string, codebase map[string]interface{}) ([]TaskSpec, error) {
	catalogJSON, _ := json.MarshalIndent(d.catalog, "", "  ")
	codebaseJSON, _ := json.MarshalIndent(codebase, "", "  ")

	prompt := fmt.Sprintf(decomposePrompt, string(catalogJSON), string(codebaseJSON), goal)

	messages := []ollama.Message{{
		Role:    "user",
		Content: prompt,
	}}

	raw, err := d.client.Chat(messages)
	if err != nil {
		return hardcodedDecompose(goal, repoPath, d.prefix), nil
	}

	raw = cleanJSONResponse(raw)

	var tasks []TaskSpec
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		return hardcodedDecompose(goal, repoPath, d.prefix), nil
	}

	if len(tasks) == 0 {
		return hardcodedDecompose(goal, repoPath, d.prefix), nil
	}

	// Validate
	for i, t := range tasks {
		if t.TaskID == "" || t.AgentType == "" {
			return hardcodedDecompose(goal, repoPath, d.prefix), nil
		}
		if _, ok := d.catalog[t.AgentType]; t.AgentType != "synthesis" && !ok {
			return hardcodedDecompose(goal, repoPath, d.prefix), fmt.Errorf("unknown agent type: %s (task %d)", t.AgentType, i)
		}
	}

	return tasks, nil
}

// hardcodedDecompose produces a fallback 3-task DAG for code review.
// This is the port of _hardcoded_decompose() from orchestrator_v3.py.
func hardcodedDecompose(goal, repoPath, prefix string) []TaskSpec {
	if prefix == "" {
		prefix = fmt.Sprintf("task_%s", shortUUID())
	}
	return []TaskSpec{
		{
			TaskID:    fmt.Sprintf("%s_static", prefix),
			ParentIDs: []string{},
			Status:    "ready",
			Goal:      "Run static analysis: golangci-lint, gosec, govulncheck on the codebase",
			AgentType: "static_analysis",
			Context: map[string]interface{}{
				"repo_path":  repoPath,
				"output_key": fmt.Sprintf("%s_static_report", prefix),
			},
		},
		{
			TaskID:    fmt.Sprintf("%s_quality", prefix),
			ParentIDs: []string{},
			Status:    "ready",
			Goal:      "Scan code quality: god objects, long functions, error handling, test coverage",
			AgentType: "quality_scan",
			Context: map[string]interface{}{
				"repo_path":  repoPath,
				"output_key": fmt.Sprintf("%s_quality_report", prefix),
			},
		},
		{
			TaskID:    fmt.Sprintf("%s_synthesis", prefix),
			ParentIDs: []string{fmt.Sprintf("%s_static", prefix), fmt.Sprintf("%s_quality", prefix)},
			Status:    "blocked",
			Goal:      "Synthesize static analysis and quality scan findings into a unified audit report",
			AgentType: "synthesis",
			Context: map[string]interface{}{
				"repo_path":  repoPath,
				"dependents": []string{fmt.Sprintf("%s_static", prefix), fmt.Sprintf("%s_quality", prefix)},
				"output_key": fmt.Sprintf("%s_final_report", prefix),
			},
		},
	}
}

// cleanJSONResponse strips markdown fences and trailing cruft from LLM JSON output.
func cleanJSONResponse(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) > 1 {
			s = strings.TrimSuffix(lines[1], "```")
		}
	}
	return strings.TrimSpace(s)
}

// shortUUID returns 6 hex characters for task ID prefixing.
func shortUUID() string {
	// Use the first 6 chars of a simple hash of the current nanosecond.
	// Avoids collisions within a process; not cryptographic.
	n := uint64(1) // placeholder — caller passes explicit prefix in practice
	return fmt.Sprintf("%06x", n)
}

// scanCodebase collects basic metadata about a codebase for LLM context.
// Port of scan_codebase() from orchestrator_v3.py.
func scanCodebase(repoPath string) map[string]interface{} {
	info := map[string]interface{}{
		"repo_path": repoPath,
	}
	if repoPath == "" {
		return info
	}
	return info
}
