package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAcceptanceChecker_NilAdapterIsDisabled covers the no-op
// path: nil adapter returns (nil, nil). Caller treats as
// auto-accept.
func TestAcceptanceChecker_NilAdapterIsDisabled(t *testing.T) {
	c := NewAcceptanceChecker(nil)
	out, err := c.Check(context.Background(), AcceptanceCheckInput{PhaseGoal: "x"})
	if err != nil {
		t.Errorf("nil adapter must not produce err; got %v", err)
	}
	if out != nil {
		t.Errorf("nil adapter must return nil result; got %+v", out)
	}
}

// TestAcceptanceChecker_HappyPath verifies one Chat call dispatches
// with the right tool, parses the response, returns the verdict.
func TestAcceptanceChecker_HappyPath(t *testing.T) {
	stub := &stubLLM{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name:   acceptanceTool.Name,
				Params: []byte(`{"passed":true,"reasoning":"plan summary describes migration; schema test passes","next_hint":"ORM layer needs to know about users.email"}`),
			}},
		},
	}
	c := &llmAcceptanceChecker{adapter: stub}
	in := AcceptanceCheckInput{
		PhaseGoal:   "add migration file",
		PlanSummary: "create migrations/0001_users.sql with email column",
		TestReport: &types.ChangeReport{
			PlanID: "plan-x",
			Passed: true,
			TestResults: []types.TestResult{{AssertionID: "TestSchema", Passed: true}},
		},
		Outcomes: []string{"users table has email column"},
		Attempt:  1,
	}
	out, err := c.Check(context.Background(), in)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil verdict")
	}
	if !out.Passed {
		t.Error("expected Passed=true")
	}
	if !strings.Contains(out.Reasoning, "migration") {
		t.Errorf("reasoning drift; got %q", out.Reasoning)
	}
	if !strings.Contains(out.NextHint, "ORM") {
		t.Errorf("next_hint drift; got %q", out.NextHint)
	}
	if stub.callCount != 1 {
		t.Errorf("expected 1 Chat call; got %d", stub.callCount)
	}
	if stub.lastOptions.ToolChoice != "required" {
		t.Errorf("expected tool_choice=required; got %q", stub.lastOptions.ToolChoice)
	}
	// Confirm the user message rendered the phase goal + plan summary
	// + outcomes section.
	for _, want := range []string{"Phase goal", "add migration file", "Plan summary", "users table has email column"} {
		if !strings.Contains(stub.lastUser, want) {
			t.Errorf("user message missing %q; got %q", want, stub.lastUser)
		}
	}
}

// TestAcceptanceChecker_RejectionPath: a passed=false verdict
// flows through verbatim.
func TestAcceptanceChecker_RejectionPath(t *testing.T) {
	stub := &stubLLM{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name:   acceptanceTool.Name,
				Params: []byte(`{"passed":false,"reasoning":"plan summary mentions auth.go but the phase was supposed to add a migration; tests passed but in unrelated code"}`),
			}},
		},
	}
	c := &llmAcceptanceChecker{adapter: stub}
	out, err := c.Check(context.Background(), AcceptanceCheckInput{
		PhaseGoal: "add migration",
		Attempt:   1,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if out.Passed {
		t.Error("expected Passed=false")
	}
	if !strings.Contains(out.Reasoning, "supposed to add a migration") {
		t.Errorf("reasoning drift; got %q", out.Reasoning)
	}
}

// TestAcceptanceChecker_ChatErrorPropagates verifies a
// Chat-level error flows up so the caller can degrade.
func TestAcceptanceChecker_ChatErrorPropagates(t *testing.T) {
	stub := &stubLLM{err: errors.New("network blip")}
	c := &llmAcceptanceChecker{adapter: stub}
	out, err := c.Check(context.Background(), AcceptanceCheckInput{PhaseGoal: "x", PlanSummary: "y"})
	if err == nil {
		t.Error("expected err on Chat failure")
	}
	if out != nil {
		t.Errorf("expected nil verdict on err; got %+v", out)
	}
}

// TestAcceptanceChecker_NoToolCallReportsErr covers the
// LLM-misbehave path.
func TestAcceptanceChecker_NoToolCallReportsErr(t *testing.T) {
	stub := &stubLLM{resp: llm.Response{Content: "no tool"}}
	c := &llmAcceptanceChecker{adapter: stub}
	_, err := c.Check(context.Background(), AcceptanceCheckInput{PhaseGoal: "x", PlanSummary: "y"})
	if err == nil || !strings.Contains(err.Error(), "no tool_call") {
		t.Errorf("expected 'no tool_call' err; got %v", err)
	}
}

// TestAcceptanceCheckInput_RendersAllSections pins the user
// message renderer — every populated input field surfaces in
// the user message exactly once.
func TestAcceptanceCheckInput_RendersAllSections(t *testing.T) {
	in := AcceptanceCheckInput{
		PhaseGoal:   "phase goal text",
		PlanSummary: "plan summary text",
		Outcomes:    []string{"outcome A", "outcome B"},
		TestReport: &types.ChangeReport{
			Passed: false,
			TestResults: []types.TestResult{
				{Suite: "pkg", AssertionID: "TestX", Passed: false, FailureDetail: "assert failed"},
				{Suite: "pkg", AssertionID: "TestY", Passed: true},
			},
			FailureSummary: "1 of 2 tests failed",
		},
	}
	got := renderAcceptanceUserMessage(in)
	for _, want := range []string{
		"## Phase goal",
		"phase goal text",
		"## Plan summary (from the planner)",
		"plan summary text",
		"## Overall task expected outcomes",
		"- outcome A",
		"- outcome B",
		"## Test report",
		"Failures (1)",
		"pkg::TestX",
		"assert failed",
		"1 of 2 tests failed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered message missing %q\n--- full message ---\n%s", want, got)
		}
	}
	// Passing test should NOT appear in the failures list.
	if strings.Contains(got, "TestY") {
		t.Errorf("passing test leaked into failure list; got %q", got)
	}
}

// TestAcceptanceCheckInput_EmptyInputReturnsErr pins the
// defensive guard against a fully-empty input.
func TestAcceptanceCheckInput_EmptyInputReturnsErr(t *testing.T) {
	stub := &stubLLM{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name: acceptanceTool.Name, Params: []byte(`{"passed":true,"reasoning":"x"}`),
			}},
		},
	}
	c := &llmAcceptanceChecker{adapter: stub}
	_, err := c.Check(context.Background(), AcceptanceCheckInput{})
	if err == nil {
		t.Error("expected err on empty input")
	}
}
