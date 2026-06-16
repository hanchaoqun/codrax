package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// planner_probe_test.go pins Batch 4 (Module E): the plan-stage
// dry-run probe channel + planner-side rendering of probe results.

// TestPlannerBuildInitialInstruction_ProbeHistorySection verifies
// that when the planner has run dry-run probes earlier in the Run,
// their results are rendered into the next dispatch's initial prompt
// as a "## Probe results" section — verbatim, no
// system pre-classification.
func TestPlannerBuildInitialInstruction_ProbeHistorySection(t *testing.T) {
	mu := types.NewMutableState("plan probe test")
	mu.AppendPlanStageProbeReport(&types.ChangeReport{
		Passed: false,
		TestResults: []types.TestResult{
			{AssertionID: "TestExistingA", Passed: true},
			{AssertionID: "TestExistingB", Passed: false, FailureDetail: "raw stderr"},
		},
		FailureSummary: "1 of 2 existing tests failed",
	})
	e := &plannerEvaluator{}
	ctx := &types.AgentContext{Mutable: mu}
	got := e.BuildInitialInstruction(ctx, nil)

	if !strings.Contains(got, "## Probe results") {
		t.Errorf("expected probe history section; got:\n%s", got)
	}
	if !strings.Contains(got, "1 passed, 1 failed") {
		t.Errorf("probe section should show counts; got:\n%s", got)
	}
	if !strings.Contains(got, "1 of 2 existing tests failed") {
		t.Errorf("probe section should carry verbatim FailureSummary; got:\n%s", got)
	}
	if strings.Contains(got, "no_change_required") {
		t.Errorf("plain probe history must not advertise the replan no-change sentinel; got:\n%s", got)
	}
	// Forbidden — must not classify what the failure means.
	for _, banned := range []string{"Most common", "you should", "Revise to"} {
		if strings.Contains(got, banned) {
			t.Errorf("probe section must not contain prescriptive prose %q; got:\n%s", banned, got)
		}
	}
}

func TestPlannerBuildInitialInstruction_ProbeHistoryNoChangeSentinel(t *testing.T) {
	mu := types.NewMutableState("plan probe test")
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		PlanID:  "plan-applied",
		BatchID: "batch-1",
		Attempt: 1,
	})
	mu.AppendPlanStageProbeReport(&types.ChangeReport{
		Channel:            types.ChangeReportChannelPlannerProbe,
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{
			{AssertionID: "tests/test_routes.py::test_route_registration", Passed: true},
		},
	})
	e := &plannerEvaluator{}
	ctx := &types.AgentContext{Mutable: mu}
	got := e.BuildInitialInstruction(ctx, nil)

	for _, want := range []string{"## Probe results", "No-change sentinel available", "changes: []", "no_change_required"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in no-change sentinel probe section; got:\n%s", want, got)
		}
	}
	for _, banned := range []string{"Most common", "you should", "Revise to"} {
		if strings.Contains(got, banned) {
			t.Errorf("probe section must not contain prescriptive prose %q; got:\n%s", banned, got)
		}
	}
}

// TestPlannerBuildInitialInstruction_NoProbeNoSection verifies the
// degraded path: zero probes → no section rendered.
func TestPlannerBuildInitialInstruction_NoProbeNoSection(t *testing.T) {
	mu := types.NewMutableState("test")
	e := &plannerEvaluator{}
	ctx := &types.AgentContext{Mutable: mu}
	got := e.BuildInitialInstruction(ctx, nil)
	if strings.Contains(got, "## Probe results") {
		t.Errorf("no probe should produce no section; got:\n%s", got)
	}
}
