// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

// Package eval provides a lightweight evaluation framework for testing
// agent behavior across tool usage, response quality, and routing decisions.
//
// Phase 35: replaces ad-hoc manual testing with reproducible, automatable
// test cases that score agent responses against expectations.
package eval

import (
	"fmt"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// EvalCase defines a test case for agent evaluation.
// It specifies the user input, expected behavior, and scoring rules.
type EvalCase struct {
	// Name is a human-readable identifier for this case (e.g., "memory-search").
	Name string

	// Description explains what this case tests.
	Description string

	// Turns is the sequence of user messages to send. Each turn can have
	// its own expectations. A case passes if ALL turns pass.
	Turns []Turn
}

// Turn is a single interaction turn in an eval case.
type Turn struct {
	// UserMessage is the message sent to the agent.
	UserMessage string

	// Expect defines what the agent must do for this turn to pass.
	// If nil, the turn always passes (sanity check only).
	Expect *Expectation
}

// Expectation defines what a turn's result must satisfy.
// All non-zero/non-empty fields are AND-ed together — every
// specified expectation must be met.
type Expectation struct {
	// ToolCalled requires that this exact tool was invoked during the turn.
	// Case-insensitive match. Empty = don't check.
	ToolCalled string

	// ContainsKeyword requires that the agent's response contains this
	// substring. Case-insensitive. Empty = don't check.
	ContainsKeyword string

	// MinToolsCalled requires at least this many tool calls.
	// Zero = don't check.
	MinToolsCalled int

	// MaxToolsCalled is the upper bound on tool calls (anti-abuse check).
	// Zero = don't check.
	MaxToolsCalled int
}

// Case is a validated eval case ready for execution.
type Case struct {
	EvalCase EvalCase
}

// NewCase validates and creates a Case.
func NewCase(ec EvalCase) (*Case, error) {
	if strings.TrimSpace(ec.Name) == "" {
		return nil, fmt.Errorf("eval case name must not be empty")
	}
	if len(ec.Turns) == 0 {
		return nil, fmt.Errorf("eval case %q must have at least one turn", ec.Name)
	}
	for i, turn := range ec.Turns {
		if strings.TrimSpace(turn.UserMessage) == "" {
			return nil, fmt.Errorf("eval case %q: turn %d user message must not be empty", ec.Name, i)
		}
	}
	return &Case{EvalCase: ec}, nil
}

// Score evaluates a pipeline result against this case's expectations.
// The result is the cumulative output after processing ALL turns.
func (c *Case) Score(result *agentctx.PipelineResult) Score {
	s := Score{CaseName: c.EvalCase.Name}

	// Collect tool calls from the parsed response
	var toolCalls []agentctx.ToolCall
	var responseText string
	if result.Response != nil {
		toolCalls = result.Response.ToolCalls
		responseText = result.Response.Text
	}

	calledTools := make(map[string]bool)
	for _, tc := range toolCalls {
		calledTools[strings.ToLower(tc.Name)] = true
	}

	// Evaluate each turn's expectations
	for i, turn := range c.EvalCase.Turns {
		if turn.Expect == nil {
			continue
		}
		exp := turn.Expect

		// Check tool called
		if exp.ToolCalled != "" {
			if !calledTools[strings.ToLower(exp.ToolCalled)] {
				s.Errors = append(s.Errors, fmt.Sprintf(
					"turn %d: expected tool %q to be called, got %v",
					i, exp.ToolCalled, toolNames(toolCalls),
				))
			}
		}

		// Check keyword
		if exp.ContainsKeyword != "" {
			if !strings.Contains(
				strings.ToLower(responseText),
				strings.ToLower(exp.ContainsKeyword),
			) {
				s.Errors = append(s.Errors, fmt.Sprintf(
					"turn %d: expected keyword %q in response, got %q",
					i, exp.ContainsKeyword, truncate(responseText, 100),
				))
			}
		}

		// Check tool count bounds
		if exp.MinToolsCalled > 0 && len(toolCalls) < exp.MinToolsCalled {
			s.Errors = append(s.Errors, fmt.Sprintf(
				"turn %d: expected >= %d tool calls, got %d",
				i, exp.MinToolsCalled, len(toolCalls),
			))
		}
		if exp.MaxToolsCalled > 0 && len(toolCalls) > exp.MaxToolsCalled {
			s.Errors = append(s.Errors, fmt.Sprintf(
				"turn %d: expected <= %d tool calls, got %d",
				i, exp.MaxToolsCalled, len(toolCalls),
			))
		}
	}

	s.Passed = len(s.Errors) == 0
	return s
}

func toolNames(calls []agentctx.ToolCall) []string {
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Name
	}
	return names
}

// Score is the result of evaluating a single case.
type Score struct {
	CaseName string
	Passed   bool
	Errors   []string
}

func (s Score) String() string {
	if s.Passed {
		return fmt.Sprintf("PASS: %s", s.CaseName)
	}
	return fmt.Sprintf("FAIL: %s — %v", s.CaseName, s.Errors)
}

// Suite is a collection of eval cases that are run together.
type Suite struct {
	Name  string
	Cases []*Case
}

// NewSuite creates an eval suite.
func NewSuite(name string, cases []*Case) *Suite {
	return &Suite{Name: name, Cases: cases}
}

// SuiteResult aggregates results from running a full suite.
type SuiteResult struct {
	SuiteName string
	Passed    int
	Failed    int
	Scores    []Score
}

// Run executes all cases in the suite. The runner function receives
// each case and must return the agent's pipeline result. This decouples
// eval logic from agent setup (provider, tools, config).
func (s *Suite) Run(runner func(*Case) *agentctx.PipelineResult) *SuiteResult {
	res := &SuiteResult{SuiteName: s.Name}

	for _, tc := range s.Cases {
		result := runner(tc)
		score := tc.Score(result)
		res.Scores = append(res.Scores, score)

		if score.Passed {
			res.Passed++
		} else {
			res.Failed++
		}
	}

	return res
}

// Summary returns a one-line summary of suite results.
func (r *SuiteResult) Summary() string {
	total := r.Passed + r.Failed
	return fmt.Sprintf("%s: %d/%d passed", r.SuiteName, r.Passed, total)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
