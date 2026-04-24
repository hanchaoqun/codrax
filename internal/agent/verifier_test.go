package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// verifierFixtureCtx builds the AgentContext the verifier expects,
// optionally pre-populating Mutable.ChangeReport so we exercise the
// report-already-installed path without running a real test runner.
func verifierFixtureCtx(report *types.ChangeReport, plan *types.ChangePlan) *types.AgentContext {
	mu := types.NewMutableState("test")
	if report != nil {
		mu.SetChangeReport(report)
	}
	if plan != nil {
		mu.SetChangePlan(plan)
	}
	return &types.AgentContext{
		AgentName: types.AgentVerifier,
		Stage:     types.StageVerify,
		Objective: "verify plan",
		Mutable:   mu,
	}
}

// TestVerifier_ShouldStop_ReportInstalled verifies the evaluator
// exits immediately when Mutable.ChangeReport is non-nil (run_tests
// has populated it synchronously).
func TestVerifier_ShouldStop_ReportInstalled(t *testing.T) {
	report := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{
			{AssertionID: "TestFoo", Suite: "pkg", Passed: true},
		},
	}
	ctx := verifierFixtureCtx(report, nil)
	ev := &verifierEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !ev.ShouldStop(llm.Response{}, 0) {
		t.Error("should stop when ChangeReport installed on iter=0")
	}
}

// TestVerifier_ShouldStop_ReportMissingCap verifies the defensive
// iteration cap fires when run_tests never populated a report.
func TestVerifier_ShouldStop_ReportMissingCap(t *testing.T) {
	ctx := verifierFixtureCtx(nil, nil)
	ev := &verifierEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})
	// Iterations 0-4: no report, below cap.
	for i := 0; i < 5; i++ {
		if ev.ShouldStop(llm.Response{}, i) {
			t.Errorf("iter %d: should not stop (no report, below cap)", i)
		}
	}
	// Iteration 5 hits cap.
	if !ev.ShouldStop(llm.Response{}, 5) {
		t.Error("iter 5: should stop at cap (defense against runaway loop)")
	}
}

// TestVerifier_ParseOutput_Passed verifies the happy path.
func TestVerifier_ParseOutput_Passed(t *testing.T) {
	report := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Passed: true},
			{AssertionID: "TestB", Passed: true},
		},
	}
	ctx := verifierFixtureCtx(report, nil)
	ev := &verifierEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})

	out, err := ev.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.MissingPiece != types.MissingNone {
		t.Errorf("MissingPiece = %q, want MissingNone", out.MissingPiece)
	}
	if out.Error != "" {
		t.Errorf("unexpected error: %q", out.Error)
	}
}

// TestVerifier_ParseOutput_Failed verifies verify-failure surfaces
// as StageOutput.Error with narrative from report.FailureSummary.
func TestVerifier_ParseOutput_Failed(t *testing.T) {
	report := &types.ChangeReport{
		Passed:         false,
		FailureSummary: "3 of 12 tests failed in handler module",
		TestResults: []types.TestResult{
			{AssertionID: "TestOK", Passed: true},
			{AssertionID: "TestBad", Passed: false, FailureDetail: "expected error"},
		},
	}
	ctx := verifierFixtureCtx(report, nil)
	ev := &verifierEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})

	out, err := ev.ParseOutput(ctx, nil, nil, nil)
	if err == nil {
		t.Fatal("ParseOutput should error on verify failure (B1 Q3 fail-loud)")
	}
	if !strings.Contains(out.Error, "verify failed") {
		t.Errorf("error should mention 'verify failed'; got %q", out.Error)
	}
	if !strings.Contains(out.Error, "3 of 12") {
		t.Errorf("error should include FailureSummary narrative; got %q", out.Error)
	}
	if out.MissingPiece != types.MissingFacts {
		t.Errorf("MissingPiece = %q, want MissingFacts", out.MissingPiece)
	}
}

// TestVerifier_ParseOutput_NoReport verifies run_tests failure
// surfaces a clean error rather than a silent pass.
func TestVerifier_ParseOutput_NoReport(t *testing.T) {
	ctx := verifierFixtureCtx(nil, nil)
	ev := &verifierEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})

	out, err := ev.ParseOutput(ctx, nil, nil, nil)
	if err == nil {
		t.Fatal("ParseOutput should error when ChangeReport missing")
	}
	if !strings.Contains(out.Error, "ChangeReport") {
		t.Errorf("error should explain missing ChangeReport; got %q", out.Error)
	}
}

// TestVerifier_BuildInitialInstruction_SurfacesAcceptanceTests
// verifies the prompt supplement lists plan's acceptance criteria.
func TestVerifier_BuildInitialInstruction_SurfacesAcceptanceTests(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-verify-inst",
		AcceptanceTests: []string{
			"all unit tests pass",
			"no new lint errors",
		},
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}
	ctx := verifierFixtureCtx(nil, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "all unit tests pass") {
		t.Errorf("instruction should list acceptance test 1; got %q", inst)
	}
	if !strings.Contains(inst, "no new lint errors") {
		t.Errorf("instruction should list acceptance test 2; got %q", inst)
	}
	if !strings.Contains(inst, "OPTIONALLY") {
		t.Errorf("instruction should mark emit_test_results as optional; got %q", inst)
	}
}

// TestVerifier_BuildInitialInstruction_NoPlan verifies graceful
// message when no plan is installed.
func TestVerifier_BuildInitialInstruction_NoPlan(t *testing.T) {
	ctx := verifierFixtureCtx(nil, nil)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "No ChangePlan") {
		t.Errorf("should explain missing plan; got %q", inst)
	}
}

// TestVerifier_BuildInitialInstruction_EmptyAcceptanceTests verifies
// we don't emit a noise supplement when plan has no explicit tests.
func TestVerifier_BuildInitialInstruction_EmptyAcceptanceTests(t *testing.T) {
	plan := &types.ChangePlan{
		ID:      "plan",
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
		// AcceptanceTests empty.
	}
	ctx := verifierFixtureCtx(nil, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if inst != "" {
		t.Errorf("empty acceptance_tests should yield empty instruction; got %q", inst)
	}
}
