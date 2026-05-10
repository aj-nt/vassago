// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package eval

import (
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// ============================================================================
// Phase 35: Evaluation Framework — TDD tests
// ============================================================================

func makeResult(text string, tools ...string) *agentctx.PipelineResult {
	calls := make([]agentctx.ToolCall, len(tools))
	for i, t := range tools {
		calls[i] = agentctx.ToolCall{Name: t}
	}
	return &agentctx.PipelineResult{
		Response: &agentctx.ParsedResponse{
			Text:      text,
			ToolCalls: calls,
		},
	}
}

func TestCase_ValidatesInput(t *testing.T) {
	_, err := NewCase(EvalCase{Name: "", Turns: []Turn{{UserMessage: "hello"}}})
	if err == nil {
		t.Error("empty name should return error")
	}

	_, err = NewCase(EvalCase{Name: "test", Turns: nil})
	if err == nil {
		t.Error("empty turns should return error")
	}

	_, err = NewCase(EvalCase{Name: "simple-greeting", Turns: []Turn{{UserMessage: "hi"}}})
	if err != nil {
		t.Fatalf("valid case: %v", err)
	}
}

func TestCase_PassesWhenExpectedToolCalled(t *testing.T) {
	tc := mustCase(t, EvalCase{
		Name: "tool-call-test",
		Turns: []Turn{
			{UserMessage: "search for foo", Expect: &Expectation{ToolCalled: "SearchMemories"}},
		},
	})

	score := tc.Score(makeResult("searching...", "SearchMemories"))
	if !score.Passed {
		t.Errorf("should pass: %v", score.Errors)
	}
}

func TestCase_FailsWhenExpectedToolNotCalled(t *testing.T) {
	tc := mustCase(t, EvalCase{
		Name: "tool-call-fail",
		Turns: []Turn{
			{UserMessage: "search", Expect: &Expectation{ToolCalled: "SearchMemories"}},
		},
	})

	score := tc.Score(makeResult("nope", "FileRead"))
	if score.Passed {
		t.Error("should fail — wrong tool called")
	}
}

func TestCase_PassesWhenKeywordFound(t *testing.T) {
	tc := mustCase(t, EvalCase{
		Name: "keyword-test",
		Turns: []Turn{
			{UserMessage: "what time", Expect: &Expectation{ContainsKeyword: "time"}},
		},
	})

	score := tc.Score(makeResult("The current time is 3:00 PM UTC."))
	if !score.Passed {
		t.Errorf("should pass: %v", score.Errors)
	}
}

func TestCase_MultipleExpectationsAND(t *testing.T) {
	tc := mustCase(t, EvalCase{
		Name: "multi-expect",
		Turns: []Turn{
			{UserMessage: "search", Expect: &Expectation{
				ToolCalled:      "SearchMemories",
				ContainsKeyword: "found",
			}},
		},
	})

	// Both met
	score := tc.Score(makeResult("I found 3 results.", "SearchMemories"))
	if !score.Passed {
		t.Errorf("should pass: %v", score.Errors)
	}

	// Only one met
	score2 := tc.Score(makeResult("I found 3 results.")) // no tool
	if score2.Passed {
		t.Error("should fail")
	}
}

func TestCase_NoExpectationsPasses(t *testing.T) {
	tc := mustCase(t, EvalCase{
		Name:  "no-expect",
		Turns: []Turn{{UserMessage: "anything"}},
	})
	if !tc.Score(makeResult("")).Passed {
		t.Error("no expectations = always pass")
	}
}

func TestSuite_RunsAllCases(t *testing.T) {
	c1 := mustCase(t, EvalCase{
		Name: "c1", Turns: []Turn{{UserMessage: "hi", Expect: &Expectation{ContainsKeyword: "hello"}}},
	})
	c2 := mustCase(t, EvalCase{
		Name: "c2", Turns: []Turn{{UserMessage: "bye", Expect: &Expectation{ContainsKeyword: "goodbye"}}},
	})

	suite := NewSuite("test-suite", []*Case{c1, c2})
	results := suite.Run(func(tc *Case) *agentctx.PipelineResult {
		if tc.EvalCase.Name == "c1" {
			return makeResult("hello there")
		}
		return makeResult("goodbye now")
	})

	if results.Passed != 2 || results.Failed != 0 {
		t.Errorf("suite: %d passed, %d failed", results.Passed, results.Failed)
	}
}

func TestSuite_TracksFailures(t *testing.T) {
	c1 := mustCase(t, EvalCase{
		Name: "c1", Turns: []Turn{{UserMessage: "hi", Expect: &Expectation{ContainsKeyword: "there"}}},
	})
	c2 := mustCase(t, EvalCase{
		Name: "c2", Turns: []Turn{{UserMessage: "bye", Expect: &Expectation{ContainsKeyword: "goodbye"}}},
	})

	suite := NewSuite("test-suite", []*Case{c1, c2})
	results := suite.Run(func(tc *Case) *agentctx.PipelineResult {
		return makeResult("hi there")  // c1 passes ("there"), c2 fails (no "goodbye")
	})

	if results.Passed != 1 || results.Failed != 1 {
		t.Errorf("suite: %d passed, %d failed (want 1/1)", results.Passed, results.Failed)
	}
}

func TestCase_MinMaxToolsCalled(t *testing.T) {
	tc := mustCase(t, EvalCase{
		Name: "tool-count",
		Turns: []Turn{
			{UserMessage: "do stuff", Expect: &Expectation{
				MinToolsCalled: 1,
				MaxToolsCalled: 3,
			}},
		},
	})

	// Exactly 2 tools — passes
	if !tc.Score(makeResult("done", "A", "B")).Passed {
		t.Error("2 tools should pass")
	}

	// Zero tools — fails
	if tc.Score(makeResult("done")).Passed {
		t.Error("0 tools should fail MinToolsCalled=1")
	}

	// Four tools — fails
	if tc.Score(makeResult("done", "A", "B", "C", "D")).Passed {
		t.Error("4 tools should fail MaxToolsCalled=3")
	}
}

func mustCase(t *testing.T, ec EvalCase) *Case {
	t.Helper()
	tc, err := NewCase(ec)
	if err != nil {
		t.Fatalf("NewCase: %v", err)
	}
	return tc
}
