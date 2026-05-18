// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"testing"
)

func TestHardcodedDecompose_ProducesThreeTasks(t *testing.T) {
	tasks := hardcodedDecompose("audit my codebase", "/tmp/repo", "abc123")

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// First two should be leaf tasks (ready status, no parents)
	for i := 0; i < 2; i++ {
		if tasks[i].Status != "ready" {
			t.Errorf("task[%d] status = %q, want ready", i, tasks[i].Status)
		}
		if len(tasks[i].ParentIDs) != 0 {
			t.Errorf("task[%d] has %d parents, want 0", i, len(tasks[i].ParentIDs))
		}
		if tasks[i].TaskID == "" {
			t.Errorf("task[%d] has empty TaskID", i)
		}
	}

	// Task 0 should be static_analysis
	if tasks[0].AgentType != "static_analysis" {
		t.Errorf("task[0] AgentType = %q, want static_analysis", tasks[0].AgentType)
	}

	// Task 1 should be quality_scan
	if tasks[1].AgentType != "quality_scan" {
		t.Errorf("task[1] AgentType = %q, want quality_scan", tasks[1].AgentType)
	}

	// Third task should be synthesis: blocked, with two parents
	synth := tasks[2]
	if synth.AgentType != "synthesis" {
		t.Errorf("synthesis AgentType = %q, want synthesis", synth.AgentType)
	}
	if synth.Status != "blocked" {
		t.Errorf("synthesis status = %q, want blocked", synth.Status)
	}
	if len(synth.ParentIDs) != 2 {
		t.Errorf("synthesis has %d parents, want 2", len(synth.ParentIDs))
	}

	// Parents should be task[0] and task[1]
	parentSet := make(map[string]bool)
	for _, pid := range synth.ParentIDs {
		parentSet[pid] = true
	}
	if !parentSet[tasks[0].TaskID] {
		t.Error("synthesis parent_ids missing task[0]")
	}
	if !parentSet[tasks[1].TaskID] {
		t.Error("synthesis parent_ids missing task[1]")
	}
}

func TestHardcodedDecompose_UniqueTaskIDs(t *testing.T) {
	tasks1 := hardcodedDecompose("audit", "/tmp/a", "prefA")
	tasks2 := hardcodedDecompose("audit", "/tmp/a", "prefB")

	// Different prefixes produce different task IDs
	for i := range tasks1 {
		if tasks1[i].TaskID == tasks2[i].TaskID {
			t.Errorf("task[%d] has same ID across different calls: %s", i, tasks1[i].TaskID)
		}
	}
}

func TestHardcodedDecompose_IncludesRepoPath(t *testing.T) {
	tasks := hardcodedDecompose("audit", "/my/special/repo", "test")

	for _, task := range tasks {
		repoPath, ok := task.Context["repo_path"].(string)
		if !ok || repoPath != "/my/special/repo" {
			t.Errorf("task %s: repo_path = %v, want /my/special/repo", task.TaskID, task.Context["repo_path"])
		}
	}
}

func TestHardcodedDecompose_SynthesisHasDependents(t *testing.T) {
	tasks := hardcodedDecompose("audit", "/tmp/repo", "t1")

	// The synthesis task must include dependents in its context for blackboard lookup
	synth := tasks[2]
	deps, ok := synth.Context["dependents"].([]string)
	if !ok {
		t.Fatalf("synthesis context missing 'dependents' field or wrong type: %T", synth.Context["dependents"])
	}
	if len(deps) != 2 {
		t.Errorf("synthesis has %d dependents, want 2", len(deps))
	}
}
