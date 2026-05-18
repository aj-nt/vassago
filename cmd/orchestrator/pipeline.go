// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aj-nt/vassago-sdk/client"
	"github.com/aj-nt/vassago/internal/ollama"
)

// Orchestrator coordinates the full pipeline: decompose → dispatch → poll → synthesize → publish.
type Orchestrator struct {
	cfg        *Config
	daemon     client.MnemoClient
	decomposer *Decomposer
	synthesis  *SynthesisEngine
	repoPath   string
}

// NewOrchestrator creates an orchestrator with the given config and daemon connection.
func NewOrchestrator(cfg *Config, daemon client.MnemoClient) *Orchestrator {
	repoPath := cfg.RepoPath
	if repoPath == "" {
		repoPath = "."
	}

	// Set up decomposer
	decompCfg := cfg.Decomposer
	decompLLM := ollama.NewClient(ollama.Config{
		BaseURL:     decompCfg.BaseURL,
		Model:       decompCfg.Model,
		MaxTokens:   decompCfg.MaxTokens,
		Temperature: decompCfg.Temperature,
		TimeoutSec:  decompCfg.TimeoutSec,
	})
	llmDecomp := NewLLMDecomposer(decompLLM)

	// Set up synthesizer
	synthCfg := cfg.Synthesizer
	synthLLM := ollama.NewClient(ollama.Config{
		BaseURL:     synthCfg.BaseURL,
		Model:       synthCfg.Model,
		MaxTokens:   synthCfg.MaxTokens,
		Temperature: synthCfg.Temperature,
		TimeoutSec:  synthCfg.TimeoutSec,
	})

	return &Orchestrator{
		cfg:        cfg,
		daemon:     daemon,
		decomposer: NewDecomposer(llmDecomp),
		synthesis: &SynthesisEngine{
			llm:      NewLLMSynthesizer(synthLLM),
			fallback: &FallbackSynthesizer{},
		},
		repoPath: repoPath,
	}
}

// DryRun decomposes a goal and prints the resulting task DAG without dispatching.
func (o *Orchestrator) DryRun(goal string) error {
	slog.Info("Decomposing goal (dry run)", "goal", goal)

	tasks, err := o.decomposer.Decompose(goal, o.repoPath)
	if err != nil {
		return fmt.Errorf("decomposition failed: %w", err)
	}

	slog.Info("Decomposition complete", "task_count", len(tasks))
	fmt.Println()
	for _, t := range tasks {
		fmt.Printf("  %-35s [%-20s] %s\n", t.TaskID, t.AgentType, t.Status)
		if len(t.ParentIDs) > 0 {
			fmt.Printf("    depends on: %v\n", t.ParentIDs)
		}
	}
	fmt.Println()
	return nil
}

// Run executes the full orchestrator pipeline.
func (o *Orchestrator) Run(goal string) (*SynthesisResult, error) {
	slog.Info("Phase 1: Decomposition", "goal", goal, "repo", o.repoPath)

	tasks, err := o.decomposer.Decompose(goal, o.repoPath)
	if err != nil {
		return nil, fmt.Errorf("decomposition: %w", err)
	}
	slog.Info("Decomposed into tasks", "count", len(tasks))

	// Phase 2: Dispatch tasks to daemon
	slog.Info("Phase 2: Dispatching tasks")
	taskMap := make(map[string]TaskSpec)
	var synthesisTask *TaskSpec

	for _, t := range tasks {
		tid := t.TaskID
		at := t.AgentType
		goalText := t.Goal
		ctxJSON, _ := json.Marshal(t.Context)
		priority := int32(3)
		if at == "synthesis" {
			priority = 4
		}

		_, err := o.daemon.AddTask(nil, tid, at, goalText, string(ctxJSON), priority, 300, 2)
		if err != nil {
			return nil, fmt.Errorf("dispatch task %s: %w", tid, err)
		}

		taskMap[tid] = t
		slog.Info("Dispatched task", "id", tid, "type", at)

		if at == "synthesis" {
			st := t
			synthesisTask = &st
		}
	}

	if synthesisTask == nil {
		return nil, fmt.Errorf("no synthesis task in DAG")
	}

	// Phase 3: Poll for worker completions
	slog.Info("Phase 3: Polling for results", "max_wait_sec", o.cfg.MaxWaitSec)
	synthTid := synthesisTask.TaskID
	start := time.Now()
	pollInterval := time.Duration(o.cfg.PollIntervalSec) * time.Second

	synthesisReady := false
	for time.Since(start) < time.Duration(o.cfg.MaxWaitSec)*time.Second {
		time.Sleep(pollInterval)

		synthesisReady = o.pollAndCheckDAG(taskMap, synthTid)
		if synthesisReady {
			break
		}
	}

	if !synthesisReady {
		slog.Warn("Timeout waiting for workers", "elapsed_sec", int(time.Since(start).Seconds()))
		// Check one more time — maybe the DAG advanced but we missed the window
		st, _ := o.daemon.GetTask(nil, synthTid)
		if st != nil && st.Status == "ready" {
			synthesisReady = true
		}
	}

	// Phase 4: Collect results and synthesize
	slog.Info("Phase 4: Synthesizing results")
	taskResults := o.collectResults(synthesisTask)
	if len(taskResults) == 0 {
		slog.Warn("No results to synthesize")
	}

	synthesis, err := o.synthesis.Synthesize(taskResults)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}
	slog.Info("Synthesis complete", "title", synthesis.Title)

	// Phase 5: Publish final report
	slog.Info("Phase 5: Publishing final report")
	resultKey := fmt.Sprintf("%s_report", synthTid)
	if ok, ok2 := synthesisTask.Context["output_key"]; ok2 {
		if k, ok3 := ok.(string); ok3 {
			resultKey = k
		}
	}

	reportJSON, _ := json.Marshal(synthesis)
	_, err = o.daemon.AddMemory(nil, "memory", "orchestrator_results", resultKey, string(reportJSON), 5, "")
	if err != nil {
		slog.Warn("Failed to store report", "key", resultKey, "error", err)
	} else {
		slog.Info("Final report published", "key", resultKey)
		o.daemon.CompleteTask(nil, synthTid, resultKey)
	}

	return synthesis, nil
}

// pollAndCheckDAG checks if the synthesis task's parents are all done.
// Returns true when synthesis is ready.
func (o *Orchestrator) pollAndCheckDAG(taskMap map[string]TaskSpec, synthTid string) bool {
	// Collect all leaf tasks (non-synthesis)
	allDone := true
	for tid, t := range taskMap {
		if t.AgentType == "synthesis" {
			continue
		}
		entry, err := o.daemon.GetTask(nil, tid)
		if err != nil {
			slog.Warn("Failed to get task", "id", tid, "error", err)
			continue
		}
		if entry == nil {
			continue
		}
		if entry.Status != "done" {
			allDone = false
		}
	}

	if !allDone {
		return false
	}

	// All parents done — check if synthesis is still blocked
	st, err := o.daemon.GetTask(nil, synthTid)
	if err != nil || st == nil {
		return false
	}

	if st.Status == "blocked" {
		// Synthesis task isn't auto-promoted by daemon — orchestrator does it.
		// For now, synthesis just reads from blackboard directly (doesn't need promotion).
		return true
	}

	return st.Status == "ready" || st.Status == "done"
}

// collectResults reads result memory entries for the synthesis task's dependents.
func (o *Orchestrator) collectResults(synth *TaskSpec) map[string]*TaskResult {
	results := make(map[string]*TaskResult)
	if synth == nil {
		return results
	}

	dependents, _ := synth.Context["dependents"].([]string)
	// Also check if it's []interface{} from JSON roundtrip
	if depsRaw, ok := synth.Context["dependents"].([]interface{}); ok {
		for _, d := range depsRaw {
			if s, ok := d.(string); ok {
				dependents = append(dependents, s)
			}
		}
	}

	for _, tid := range dependents {
		task, err := o.daemon.GetTask(nil, tid)
		if err != nil || task == nil || task.Status != "done" {
			continue
		}

		// Search for the result in the blackboard
		entries, err := o.daemon.SearchMemories(nil, fmt.Sprintf("result_%s", tid), "memory", 5)
		if err != nil {
			slog.Warn("Failed to search for result", "task_id", tid, "error", err)
			continue
		}

		for _, entry := range entries {
			var tr TaskResult
			if json.Unmarshal([]byte(entry.Content), &tr) == nil && tr.TaskID == tid {
				results[tid] = &tr
				break
			}
		}
	}

	return results
}
