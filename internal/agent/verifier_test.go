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

// verifierFixtureCtxWithBaseline is like verifierFixtureCtx but
// additionally pre-populates Mutable.BaselineReport so tests can
// exercise the pre-existing-failures prompt path.
func verifierFixtureCtxWithBaseline(report, baseline *types.ChangeReport, plan *types.ChangePlan) *types.AgentContext {
	ctx := verifierFixtureCtx(report, plan)
	if baseline != nil {
		ctx.Mutable.SetBaselineReport(baseline)
	}
	return ctx
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
	s := types.ResolvedAgentSettings(types.AgentSettings{})
	ev := &verifierEvaluator{
		softIterCap: s.VerifierSoftIterCap,
		hardIterCap: s.VerifierHardIterCap,
	}
	ev.BuildInitialInstruction(ctx, &skill.Config{})
	// Below soft cap: no stop when no report.
	for i := 0; i < s.VerifierSoftIterCap; i++ {
		if ev.ShouldStop(llm.Response{}, i) {
			t.Errorf("iter %d: should not stop (no report, below soft cap %d)", i, s.VerifierSoftIterCap)
		}
	}
	// Soft cap idle → stop.
	if !ev.ShouldStop(llm.Response{}, s.VerifierSoftIterCap) {
		t.Errorf("iter %d: should stop at soft cap when LLM idle", s.VerifierSoftIterCap)
	}
	// Hard cap stops unconditionally even with emit_test_results in flight.
	if !ev.ShouldStop(
		llm.Response{ToolCalls: []llm.ToolCall{{Name: emitTestResultsToolName}}},
		s.VerifierHardIterCap,
	) {
		t.Errorf("iter %d: should stop at hard cap regardless of tool calls", s.VerifierHardIterCap)
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

// TestVerifier_ParseOutput_NoTestsRunners verifies that a clean run
// with zero discovered tests (NoTestsRunners populated, Passed=true)
// is treated as success — not a verify failure that triggers the
// verify→plan retry loop. Provenance: pytest exit 5 on a Python
// script in a Go-only repo previously surfaced as "verify failed:
// pytest exitcode=5 — 0 of 0 total" and the planner hallucinated a
// test_*.py to "fix" the absent suite.
func TestVerifier_ParseOutput_NoTestsRunners(t *testing.T) {
	report := &types.ChangeReport{
		Passed:         true,
		NoTestsRunners: []string{"python"},
		TestResults:    nil,
	}
	ctx := verifierFixtureCtx(report, nil)
	ev := &verifierEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})

	out, err := ev.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.MissingPiece != types.MissingNone {
		t.Errorf("MissingPiece = %q, want MissingNone for NoTestsRunners pass", out.MissingPiece)
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
		t.Fatal("ParseOutput should error on verify failure — fail-loud structured outcome")
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
// the session-35 invariant: even without acceptance tests or baseline,
// the supplement MUST carry a stage-stating directive so the LLM has
// an unambiguous anchor. Prior to session-35 this case returned empty
// string, leaving the verifier leaning on the raw User Request (which
// in write mode is plan-shaped phrasing) and getting confused into
// trying emit_change_plan — the exact regression the e2e run caught.
func TestVerifier_BuildInitialInstruction_EmptyAcceptanceTests(t *testing.T) {
	plan := &types.ChangePlan{
		ID:      "plan",
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
		// AcceptanceTests empty; no baseline set.
	}
	ctx := verifierFixtureCtx(nil, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if inst == "" {
		t.Fatal("empty acceptance_tests + no baseline must still yield a stage-stating directive")
	}
	// Must pin the verifier's role and the authoritative tool.
	for _, want := range []string{"## Verify phase", "run_tests", "Do NOT emit_change_plan"} {
		if !strings.Contains(inst, want) {
			t.Errorf("instruction missing %q anchor; got %q", want, inst)
		}
	}
	// No acceptance / baseline sections when both are empty — the
	// stage-stating directive is the whole content.
	for _, unwanted := range []string{"## Plan acceptance criteria", "Pre-existing baseline failures"} {
		if strings.Contains(inst, unwanted) {
			t.Errorf("instruction should not render %q when both slots are empty; got %q", unwanted, inst)
		}
	}
}

// TestVerifier_BuildInitialInstruction_TargetPathsAndLanguages locks
// the runner-selection contract: when the plan's TargetPaths span
// languages the worktree manifest doesn't represent (e.g. a single
// .py file in a Go-manifest repo), the verifier prompt MUST surface
// those paths AND their detected languages so the LLM picks the
// runner whose language matches the change, not whichever language
// the manifest scan trips over first. Pre-fix bug: the prompt
// rendered nothing about TargetPaths and the verifier defaulted to
// runner=go on go.mod presence, running the entire codrax Go suite
// against a Python-only plan.
func TestVerifier_BuildInitialInstruction_TargetPathsAndLanguages(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-runner-hint",
		TargetPaths: []string{"guess_number.py"},
		Changes:     []types.FileChange{{Path: "guess_number.py", Kind: "create"}},
	}
	ctx := verifierFixtureCtx(nil, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	for _, want := range []string{
		"## Plan-touched paths",
		"guess_number.py",
		"Languages touched:",
		"python",
		"Pick the runner whose language matches FIRST",
	} {
		if !strings.Contains(inst, want) {
			t.Errorf("instruction missing %q anchor; got:\n%s", want, inst)
		}
	}
}

// TestVerifier_BuildInitialInstruction_BaselineFailuresRendered checks
// that when the orchestrator captured a pre-apply baseline that
// already had failing tests, the verifier prompt lists them so the
// LLM can classify regression vs pre-existing.
func TestVerifier_BuildInitialInstruction_BaselineFailuresRendered(t *testing.T) {
	plan := &types.ChangePlan{
		ID:      "plan-baseline",
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}
	baseline := &types.ChangeReport{
		Passed: false,
		TestResults: []types.TestResult{
			{AssertionID: "TestPreExistingBroken", Suite: "legacy", Passed: false, FailureDetail: "was broken before"},
			{AssertionID: "TestFine", Suite: "legacy", Passed: true},
			{AssertionID: "TestAlsoBroken", Suite: "legacy", Passed: false},
		},
	}
	ctx := verifierFixtureCtxWithBaseline(nil, baseline, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "Pre-existing baseline failures") {
		t.Errorf("instruction should include baseline section; got %q", inst)
	}
	if !strings.Contains(inst, "TestPreExistingBroken") {
		t.Errorf("instruction should list pre-existing failure; got %q", inst)
	}
	if !strings.Contains(inst, "TestAlsoBroken") {
		t.Errorf("instruction should list second failure; got %q", inst)
	}
	if strings.Contains(inst, "TestFine") {
		t.Errorf("passing baseline tests should not render; got %q", inst)
	}
	for _, keyword := range []string{"regression_assertions", "preexisting_assertions", "fixed_assertions"} {
		if !strings.Contains(inst, keyword) {
			t.Errorf("instruction should teach the LLM about the structured field %q; got %q", keyword, inst)
		}
	}
	if !strings.Contains(inst, "## Output requirement") {
		t.Errorf("structured-classification contract section missing; got %q", inst)
	}
}

// TestVerifier_BuildInitialInstruction_BaselinePassing verifies we
// DON'T render the "Pre-existing baseline failures" section when the
// baseline was captured but all tests passed. Post session-35: the
// stage-stating directive is always present, but the conditional
// baseline/acceptance blocks stay gated by their slot's content.
func TestVerifier_BuildInitialInstruction_BaselinePassing(t *testing.T) {
	plan := &types.ChangePlan{
		ID:      "plan-baseline-clean",
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}
	baseline := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{
			{AssertionID: "TestFine", Passed: true},
		},
	}
	ctx := verifierFixtureCtxWithBaseline(nil, baseline, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if strings.Contains(inst, "Pre-existing baseline failures") {
		t.Errorf("clean baseline should NOT render the baseline-failures section; got %q", inst)
	}
	// Stage-stating directive must still be present (session-35 invariant).
	if !strings.Contains(inst, "## Verify phase") || !strings.Contains(inst, "run_tests") {
		t.Errorf("stage-stating directive missing; got %q", inst)
	}
}

// TestVerifier_BuildInitialInstruction_BaselineFailureCap caps the
// rendered list at 15 so a catastrophic baseline doesn't blow up
// the prompt.
func TestVerifier_BuildInitialInstruction_BaselineFailureCap(t *testing.T) {
	plan := &types.ChangePlan{
		ID:      "plan-baseline-many",
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}
	var results []types.TestResult
	for i := 0; i < 25; i++ {
		results = append(results, types.TestResult{
			AssertionID: "TestBroken" + string(rune('A'+i%26)),
			Passed:      false,
		})
	}
	baseline := &types.ChangeReport{TestResults: results}
	ctx := verifierFixtureCtxWithBaseline(nil, baseline, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "+10 more") {
		t.Errorf("cap at 15 should yield '+10 more' overflow marker; got %q", inst)
	}
}

// TestVerifier_BuildInitialInstruction_AcceptanceAndBaselineBoth
// verifies both sections render together when both inputs are
// present.
func TestVerifier_BuildInitialInstruction_AcceptanceAndBaselineBoth(t *testing.T) {
	plan := &types.ChangePlan{
		ID:              "plan-both",
		AcceptanceTests: []string{"no regressions"},
		Changes:         []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}
	baseline := &types.ChangeReport{
		TestResults: []types.TestResult{
			{AssertionID: "TestOldBug", Passed: false},
		},
	}
	ctx := verifierFixtureCtxWithBaseline(nil, baseline, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "Plan acceptance criteria") {
		t.Errorf("acceptance section missing; got %q", inst)
	}
	if !strings.Contains(inst, "Pre-existing baseline failures") {
		t.Errorf("baseline section missing; got %q", inst)
	}
	if !strings.Contains(inst, "no regressions") {
		t.Errorf("acceptance criterion missing; got %q", inst)
	}
	if !strings.Contains(inst, "TestOldBug") {
		t.Errorf("baseline failure name missing; got %q", inst)
	}
}

// TestVerifier_BuildInitialInstruction_BuildFailureSurfaced verifies
// the new "Build failures (this run)" section: when ChangeReport
// carries BuildFailed=true with structured BuildErrors[], the prompt
// renders concrete file:line targets for the LLM to act on.
func TestVerifier_BuildInitialInstruction_BuildFailureSurfaced(t *testing.T) {
	plan := &types.ChangePlan{
		ID:      "plan-build-fail",
		Changes: []types.FileChange{{Path: "src/Foo.java", Kind: "modify"}},
	}
	report := &types.ChangeReport{
		Passed:         false,
		BuildFailed:    true,
		FailureSummary: "Java build failed: 2 compile error(s)",
		TestResults: []types.TestResult{{
			Kind:        types.TestResultKindBuildError,
			AssertionID: "/src/Foo.java:42",
			Suite:       "build",
			Passed:      false,
			BuildErrors: []types.BuildError{
				{File: "/src/Foo.java", Line: 42, Column: 5, Message: "cannot find symbol"},
				{File: "/src/Foo.java", Line: 99, Message: "missing return"},
			},
		}},
	}
	ctx := verifierFixtureCtx(report, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "## Build failures (this run)") {
		t.Errorf("Build failures section missing; got %q", inst)
	}
	if !strings.Contains(inst, "/src/Foo.java:42:5") {
		t.Errorf("first error file:line:col missing; got %q", inst)
	}
	if !strings.Contains(inst, "cannot find symbol") {
		t.Errorf("first error message missing; got %q", inst)
	}
	if !strings.Contains(inst, "/src/Foo.java:99") {
		t.Errorf("second error missing; got %q", inst)
	}
}

// When BuildFailed is true but BuildErrors[] is empty (markerless
// build output), the prompt should still render the section header
// + a fallback note pointing at FailureDetail. No silent omission.
func TestVerifier_BuildInitialInstruction_BuildFailureNoStructured(t *testing.T) {
	plan := &types.ChangePlan{
		ID:      "plan-build-fail-mu",
		Changes: []types.FileChange{{Path: "x", Kind: "modify"}},
	}
	report := &types.ChangeReport{
		BuildFailed: true,
		Passed:      false,
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindBuildError,
			Suite:         "build",
			Passed:        false,
			FailureDetail: "Could not resolve dependency foo:bar:1.0",
		}},
	}
	ctx := verifierFixtureCtx(report, plan)
	ev := &verifierEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "## Build failures (this run)") {
		t.Errorf("Build failures section should render even without structured errors; got %q", inst)
	}
	if !strings.Contains(inst, "No structured compile errors parsed") {
		t.Errorf("fallback note missing; got %q", inst)
	}
}
