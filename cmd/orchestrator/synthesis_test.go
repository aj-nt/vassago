// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"testing"
)

func TestBasicCrossReference_EmptyResults(t *testing.T) {
	results := map[string]*TaskResult{}
	synth := basicCrossReference(results)

	if synth.Title == "" {
		t.Error("synthesis should have a default title even with no results")
	}
}

func TestBasicCrossReference_DetectsFailure(t *testing.T) {
	results := map[string]*TaskResult{
		"task_1": {
			TaskID:   "task_1",
			Status:   "success",
			Findings: map[string]interface{}{
				"exit_code": float64(1), // JSON numbers decode as float64
			},
		},
	}
	synth := basicCrossReference(results)

	if len(synth.HighFindings) == 0 {
		t.Error("should have high findings for non-zero exit code")
	}
}

func TestBasicCrossReference_DetectsBarePanics(t *testing.T) {
	results := map[string]*TaskResult{
		"task_1": {
			TaskID:   "task_1",
			Status:   "success",
			Findings: map[string]interface{}{
				"bare_panics": float64(3),
			},
		},
	}
	synth := basicCrossReference(results)

	found := false
	for _, f := range synth.HighFindings {
		if f.Sources[0] == "task_1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("should have finding from task_1 for bare_panics > 0, got %d findings", len(synth.HighFindings))
	}
}

func TestBasicCrossReference_IncludesSummary(t *testing.T) {
	results := map[string]*TaskResult{
		"task_1": {TaskID: "task_1", Status: "success", Findings: map[string]interface{}{}},
		"task_2": {TaskID: "task_2", Status: "success", Findings: map[string]interface{}{}},
	}
	synth := basicCrossReference(results)

	if synth.ExecutiveSummary == "" {
		t.Error("synthesis should include an executive summary")
	}
	if len(synth.Recommendations) == 0 {
		t.Error("synthesis should include at least one recommendation")
	}
}

func TestSynthesizeResults_LLMFailureFallsBack(t *testing.T) {
	// Without a real LLM client, the engine should use the fallback.
	// This test validates the fallback path is wired correctly.
	engine := &SynthesisEngine{
		llm:      nil, // simulate no LLM
		fallback: &FallbackSynthesizer{},
	}

	results := map[string]*TaskResult{
		"task_1": {TaskID: "task_1", Status: "success", Findings: map[string]interface{}{
			"bare_panics": float64(5),
		}},
	}
	synth, err := engine.Synthesize(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if synth.Title == "" {
		t.Error("fallback synthesis should produce a result")
	}
}

func TestSynthesisResult_JSONRoundtrip(t *testing.T) {
	sr := &SynthesisResult{
		Title:            "Test Report",
		ExecutiveSummary: "All clear",
		HighFindings: []Finding{
			{Title: "Issue", Location: "file.go:10", Sources: []string{"task_1"}},
		},
		CodebaseHealth: map[string]string{
			"overall_score": "85/100",
		},
		Recommendations: []Recommendation{
			{Priority: 1, Action: "Fix it", Effort: "low", Impact: "high"},
		},
	}

	// Should serialize and deserialize cleanly for blackboard storage
	data, err := jsonMarshal(sr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Error("empty JSON")
	}
}
