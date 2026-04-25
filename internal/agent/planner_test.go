package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// newPlannerEvaluatorForTest builds a plannerEvaluator with the
// resolved AgentSettings caps so the test stays in sync with whatever
// defaults DefaultAgentSettings declares.
func newPlannerEvaluatorForTest(t *testing.T) *plannerEvaluator {
	t.Helper()
	s := types.ResolvedAgentSettings(types.AgentSettings{})
	mu := types.NewMutableState("obj")
	return &plannerEvaluator{
		mu:          mu,
		softIterCap: s.PlannerSoftIterCap,
		hardIterCap: s.PlannerHardIterCap,
	}
}

// TestPlannerShouldStop_PlanInstalledStops pins the happy-path exit:
// the moment emit_change_plan installs a plan on Mutable, the loop
// terminates regardless of iteration count.
func TestPlannerShouldStop_PlanInstalledStops(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	e.mu.SetChangePlan(&types.ChangePlan{ID: "plan-x"})

	if !e.ShouldStop(llm.Response{}, 0) {
		t.Fatalf("expected immediate stop when ChangePlan is installed")
	}
}

// TestPlannerShouldStop_BelowSoftCapContinues confirms the soft-cap
// gate: under the soft cap, ShouldStop never terminates regardless of
// what the LLM is calling.
func TestPlannerShouldStop_BelowSoftCapContinues(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)

	for i := 0; i < e.softIterCap; i++ {
		if e.ShouldStop(llm.Response{}, i) {
			t.Fatalf("expected continue at iter=%d (below soft cap %d)", i, e.softIterCap)
		}
	}
}

// TestPlannerShouldStop_SoftCapStopsWhenIdle pins the default
// soft-cap behaviour: at the soft cap, with no emit_change_plan in
// flight, the loop terminates.
func TestPlannerShouldStop_SoftCapStopsWhenIdle(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)

	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: "read_file"},
	}}
	if !e.ShouldStop(resp, e.softIterCap) {
		t.Fatalf("expected stop at soft cap when LLM is not retrying emit_change_plan")
	}
}

// TestPlannerShouldStop_SoftCapAllowsEmitRetry locks in the recovery
// behaviour for streaming-truncation: at the soft cap, a clean retry
// of emit_change_plan must be allowed to execute. Without this the
// LLM's recovery emit is discarded and the truncation error surfaces
// to the user.
func TestPlannerShouldStop_SoftCapAllowsEmitRetry(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)

	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: emitChangePlanToolName},
	}}
	if e.ShouldStop(resp, e.softIterCap) {
		t.Fatalf("expected continue at soft cap when LLM is retrying emit_change_plan")
	}
}

// TestPlannerShouldStop_HardCapAlwaysStops bounds the recovery window:
// even when the LLM keeps spamming emit_change_plan, the hard cap
// ensures the loop cannot run indefinitely.
func TestPlannerShouldStop_HardCapAlwaysStops(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)

	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: emitChangePlanToolName},
	}}
	if !e.ShouldStop(resp, e.hardIterCap) {
		t.Fatalf("expected stop at hard cap regardless of tool calls")
	}
}

// TestAgentSettings_IterCapInvariants is the structural invariant
// for every two-stage cap: hard MUST be > soft, otherwise the
// recovery window collapses to zero. Resolved defaults must satisfy
// this.
func TestAgentSettings_IterCapInvariants(t *testing.T) {
	s := types.ResolvedAgentSettings(types.AgentSettings{})
	cases := []struct {
		name string
		soft int
		hard int
	}{
		{"planner", s.PlannerSoftIterCap, s.PlannerHardIterCap},
		{"verifier", s.VerifierSoftIterCap, s.VerifierHardIterCap},
		{"extractor", s.ExtractorSoftIterCap, s.ExtractorHardIterCap},
	}
	for _, c := range cases {
		if c.hard <= c.soft {
			t.Errorf("%s: hard cap (%d) must be > soft cap (%d)", c.name, c.hard, c.soft)
		}
	}
	// Coder uses len(plan.TargetPaths)+slack as soft cap and
	// soft+recovery as hard cap; recovery > 0 is the invariant.
	if s.CoderHardIterRecovery <= 0 {
		t.Errorf("coder: recovery window (%d) must be > 0", s.CoderHardIterRecovery)
	}
}

// TestAgentSettings_RejectsCollapsedRecoveryWindow pins the resolver
// safety net: a misconfigured yaml that sets hard <= soft must fall
// back to defaults rather than producing a useless zero-width window.
func TestAgentSettings_RejectsCollapsedRecoveryWindow(t *testing.T) {
	d := types.DefaultAgentSettings()

	// Caller deliberately sets hard == soft.
	bad := types.AgentSettings{
		PlannerSoftIterCap: 8,
		PlannerHardIterCap: 8,
	}
	got := types.ResolvedAgentSettings(bad)
	if got.PlannerSoftIterCap != d.PlannerSoftIterCap || got.PlannerHardIterCap != d.PlannerHardIterCap {
		t.Errorf("expected fallback to defaults on collapsed window, got soft=%d hard=%d",
			got.PlannerSoftIterCap, got.PlannerHardIterCap)
	}
}
