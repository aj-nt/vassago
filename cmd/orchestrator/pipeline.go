// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aj-nt/vassago-sdk/client"
	"github.com/aj-nt/vassago/internal/ollama"
)

// Orchestrator coordinates the full pipeline: decompose -> dispatch -> poll -> synthesize -> publish.
type Orchestrator struct {
	cfg        *Config
	ctx        context.Context
	daemon     client.MnemoClient
	decomposer *Decomposer
	synthesis  *SynthesisEngine
	repoPath   string
}

// NewOrchestrator creates an orchestrator with the given config and daemon connection.
func NewOrchestrator(ctx context.Context, cfg *Config, daemon client.MnemoClient) *Orchestrator {
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
		ctx:        ctx,
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

	// Separate leaf tasks from synthesis (synthesis runs in-process, not on workers)
	var leafTasks []TaskSpec
	var synthesisTask *TaskSpec
	for _, t := range tasks {
		if t.AgentType == "synthesis" {
			st := t
			synthesisTask = &st
		} else {
			leafTasks = append(leafTasks, t)
		}
	}
	slog.Info("Leaf tasks to dispatch", "count", len(leafTasks))

	// Phase 2: Dispatch leaf tasks to daemon for worker execution
	slog.Info("Phase 2: Dispatching leaf tasks")
	dispatched := make(map[string]bool)
	for _, t := range leafTasks {
		tid := t.TaskID
		ctxJSON, _ := json.Marshal(t.Context)

		_, err := o.daemon.AddTask(o.ctx, tid, t.AgentType, t.Goal, string(ctxJSON), 3, 300, 2)
		if err != nil {
			return nil, fmt.Errorf("dispatch task %s: %w", tid, err)
		}

		dispatched[tid] = true
		slog.Info("Dispatched task", "id", tid, "type", t.AgentType)
	}

	// Phase 3: Poll for worker completions
	slog.Info("Phase 3: Polling for results", "max_wait_sec", o.cfg.MaxWaitSec)
	start := time.Now()
	pollInterval := time.Duration(o.cfg.PollIntervalSec) * time.Second

	allDone := false
	for time.Since(start) < time.Duration(o.cfg.MaxWaitSec)*time.Second {
		time.Sleep(pollInterval)

		allDone = true
		for tid := range dispatched {
			entry, err := o.daemon.GetTask(o.ctx, tid)
			if err != nil {
				slog.Warn("Failed to get task", "id", tid, "error", err)
				continue
			}
			if entry == nil || entry.Status != "done" {
				allDone = false
			}
		}
		if allDone {
			break
		}
	}

	if !allDone {
		slog.Warn("Timeout waiting for workers", "elapsed_sec", int(time.Since(start).Seconds()))
	}

	// Phase 4: Collect results and synthesize (runs in-process, not on workers)
	slog.Info("Phase 4: Synthesizing results")
	taskResults := o.collectResults(synthesisTask)
	if len(taskResults) == 0 {
		slog.Warn("No results to synthesize — producing fallback report")
	}

	synthesis, err := o.synthesis.Synthesize(taskResults)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}
	slog.Info("Synthesis complete", "title", synthesis.Title)

	// Phase 5: Publish final report to blackboard
	slog.Info("Phase 5: Publishing final report")
	reportKey := fmt.Sprintf("orchestrator_report_%d", time.Now().Unix())

	reportJSON, _ := json.Marshal(synthesis)
	_, err = o.daemon.AddMemory(o.ctx, "memory", "orchestrator_results", reportKey, string(reportJSON), 5, "orchestrator")
	if err != nil {
		slog.Warn("Failed to store report", "key", reportKey, "error", err)
	} else {
		slog.Info("Final report published", "key", reportKey)
	}

	return synthesis, nil
}

// collectResults reads result memory entries for the synthesis task's dependents.
func (o *Orchestrator) collectResults(synth *TaskSpec) map[string]*TaskResult {
	results := make(map[string]*TaskResult)
	if synth == nil {
		return results
	}

	dependents, _ := synth.Context["dependents"].([]string)
	if depsRaw, ok := synth.Context["dependents"].([]interface{}); ok {
		for _, d := range depsRaw {
			if s, ok := d.(string); ok {
				dependents = append(dependents, s)
			}
		}
	}

	for _, tid := range dependents {
		task, err := o.daemon.GetTask(o.ctx, tid)
		if err != nil || task == nil || task.Status != "done" {
			continue
		}

		// FTS5 indexes content + key + category; the key has result_<tid>_<random>
		// so this OR disjunction catches both the key prefix and the task_id in content
		entries, err := o.daemon.SearchMemories(o.ctx, fmt.Sprintf("result_%s*", tid), "memory", 5)
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
