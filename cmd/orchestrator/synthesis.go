// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"encoding/json"
	"fmt"

	"github.com/aj-nt/vassago/internal/ollama"
)

// TaskResult mirrors the result stored by workers after executing a task.
type TaskResult struct {
	TaskID    string                 `json:"task_id"`
	AgentType string                 `json:"agent_type"`
	Status    string                 `json:"status"`
	Summary   string                 `json:"summary"`
	Error     string                 `json:"error,omitempty"`
	Findings  map[string]interface{} `json:"findings,omitempty"`
}

// Finding represents a single finding in a synthesis report.
type Finding struct {
	Title    string   `json:"title"`
	Location string   `json:"location,omitempty"`
	Sources  []string `json:"sources"`
	Detail   string   `json:"detail,omitempty"`
}

// CrossRefInsight represents a pattern discovered across multiple agent results.
type CrossRefInsight struct {
	Pattern     string   `json:"pattern"`
	Locations   []string `json:"affected_locations"`
	RootCause   string   `json:"root_cause"`
	Recommend   string   `json:"recommendation"`
}

// Recommendation is an actionable step prioritized by impact and effort.
type Recommendation struct {
	Priority int    `json:"priority"`
	Action   string `json:"action"`
	Effort   string `json:"effort"`
	Impact   string `json:"impact"`
	Rationale string `json:"rationale,omitempty"`
}

// SynthesisResult is the final output of the synthesis engine.
type SynthesisResult struct {
	Title            string            `json:"title"`
	ExecutiveSummary string            `json:"executive_summary"`
	HighFindings     []Finding         `json:"high_findings"`
	MediumFindings   []Finding         `json:"medium_findings"`
	LowFindings      []Finding         `json:"low_findings"`
	CrossRefInsights []CrossRefInsight `json:"cross_reference_insights"`
	CodebaseHealth   map[string]string `json:"codebase_health"`
	Recommendations  []Recommendation  `json:"top_recommendations"`
}

// SynthesisStrategy defines how to synthesize task results into a report.
type SynthesisStrategy interface {
	Synthesize(results map[string]*TaskResult) (*SynthesisResult, error)
}

// SynthesisEngine tries LLM synthesis first, falling back to basic cross-reference.
type SynthesisEngine struct {
	llm      *LLMSynthesizer
	fallback *FallbackSynthesizer
}

// Synthesize runs the synthesis pipeline.
func (e *SynthesisEngine) Synthesize(results map[string]*TaskResult) (*SynthesisResult, error) {
	if e.llm != nil {
		result, err := e.llm.Synthesize(results)
		if err == nil && result != nil {
			return result, nil
		}
		// LLM failed — fall through to fallback
	}
	return e.fallback.Synthesize(results), nil
}

// LLMSynthesizer calls an LLM to cross-reference and synthesize findings.
type LLMSynthesizer struct {
	client *ollama.Client
}

// NewLLMSynthesizer creates an LLM-powered synthesizer.
func NewLLMSynthesizer(client *ollama.Client) *LLMSynthesizer {
	return &LLMSynthesizer{client: client}
}

var synthesisPrompt = `You are a synthesis agent that combines findings from multiple code analysis agents into a unified audit report.

Below are results from %d independent analysis agents. Your job is to:

1. Cross-reference findings — identify issues that appear in multiple reports (e.g., a high-severity lint finding in the same file as a god object from the quality scan).

2. Prioritize findings — rank by severity: CRITICAL > HIGH > MEDIUM > LOW > INFO. Use context from cross-references to escalate: a MEDIUM lint issue in a file that's 800+ lines with bare panics becomes HIGH.

3. Identify root causes — some findings are symptoms of the same underlying problem. Group them.

4. Produce actionable recommendations — concrete steps, ordered by impact.

FORMAT YOUR RESPONSE AS JSON:
{
  "title": "Unified Audit Report",
  "executive_summary": "2-3 sentence overview of the codebase health",
  "high_findings": [
    {"title": "...", "location": "...", "sources": ["task_id_1", "task_id_2"], "detail": "..."}
  ],
  "medium_findings": [...],
  "low_findings": [...],
  "cross_reference_insights": [
    {"pattern": "...", "affected_locations": [...], "root_cause": "...", "recommendation": "..."}
  ],
  "codebase_health": {
    "overall_score": "N/100",
    "security_score": "N/100",
    "maintainability_score": "N/100",
    "test_coverage_assessment": "..."
  },
  "top_recommendations": [
    {"priority": 1, "action": "...", "effort": "low|medium|high", "impact": "low|medium|high", "rationale": "..."}
  ]
}

ANALYSIS RESULTS:
%s

Return ONLY the JSON object. No explanation, no markdown fences.`

// Synthesize calls the LLM and parses the synthesized report.
func (s *LLMSynthesizer) Synthesize(results map[string]*TaskResult) (*SynthesisResult, error) {
	summaries := make(map[string]interface{})
	for tid, r := range results {
		summaries[tid] = map[string]interface{}{
			"agent_type": r.AgentType,
			"summary":    r.Summary,
			"status":     r.Status,
			"findings":   r.Findings,
		}
	}

	resultsJSON, _ := json.MarshalIndent(summaries, "", "  ")
	prompt := fmt.Sprintf(synthesisPrompt, len(results), string(resultsJSON))

	messages := []ollama.Message{{
		Role:    "user",
		Content: prompt,
	}}

	raw, err := s.client.Chat(messages)
	if err != nil {
		return nil, fmt.Errorf("LLM synthesis failed: %w", err)
	}

	raw = cleanJSONResponse(raw)

	var result SynthesisResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse synthesis JSON: %w", err)
	}

	return &result, nil
}

// FallbackSynthesizer performs basic cross-referencing without an LLM.
// Port of _basic_cross_reference() from orchestrator_v3.py.
type FallbackSynthesizer struct{}

// Synthesize runs the fallback cross-reference.
func (f *FallbackSynthesizer) Synthesize(results map[string]*TaskResult) *SynthesisResult {
	return basicCrossReference(results)
}

// basicCrossReference performs mechanical cross-referencing of task results.
// It doesn't use an LLM — just scans for non-zero exit codes, bare panics,
// and large files across all task results.
func basicCrossReference(results map[string]*TaskResult) *SynthesisResult {
	if len(results) == 0 {
		return &SynthesisResult{
			Title:            "Basic Cross-Reference (no results)",
			ExecutiveSummary: "No task results were available for synthesis.",
			Recommendations: []Recommendation{
				{Priority: 1, Action: "Verify workers completed their tasks", Effort: "low", Impact: "medium"},
			},
		}
	}

	var highFindings []Finding
	for tid, r := range results {
		if r == nil {
			continue
		}
		f := r.Findings
		if f == nil {
			continue
		}

		// Non-zero exit codes
		for k, v := range f {
			if k == "exit_code" {
				if ec, ok := v.(float64); ok && ec != 0 {
					highFindings = append(highFindings, Finding{
						Title:   fmt.Sprintf("Tool exited with code %.0f", ec),
						Sources: []string{tid},
						Detail:  fmt.Sprintf("%s: exit code %.0f", k, ec),
					})
				}
			}
		}

		// Bare panics
		if bp, ok := f["bare_panics"].(float64); ok && bp > 0 {
			highFindings = append(highFindings, Finding{
				Title:   fmt.Sprintf("%.0f bare panic() calls detected", bp),
				Sources: []string{tid},
				Detail:  fmt.Sprintf("%.0f bare panic() calls — should use structured error handling", bp),
			})
		}

		// Large files
		if lf, ok := f["large_files"].([]interface{}); ok && len(lf) > 0 {
			for _, item := range lf {
				if s, ok := item.(string); ok {
					highFindings = append(highFindings, Finding{
						Title:   fmt.Sprintf("Large file detected: %s", s),
						Sources: []string{tid},
					})
				}
			}
		}
	}

	return &SynthesisResult{
		Title:            "Basic Cross-Reference (LLM unavailable)",
		ExecutiveSummary: fmt.Sprintf("Found %d high-severity items across %d analysis results.", len(highFindings), len(results)),
		HighFindings:     highFindings,
		Recommendations: []Recommendation{
			{Priority: 1, Action: "Address flagged items above", Effort: "medium", Impact: "high"},
		},
	}
}

// jsonMarshal wraps json.Marshal for serializing synthesis results to the blackboard.
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
