package orchestrator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBuildRetryHint_NilReport verifies that a retry dispatched
// before any ChangeReport was produced still yields a non-empty
// hint so the planner knows "something broke, start over".
func TestBuildRetryHint_NilReport(t *testing.T) {
	got := buildRetryHint(nil, nil, 1)
	if got == "" {
		t.Fatal("nil report should still yield a hint")
	}
	if !strings.Contains(got, "attempt 1") {
		t.Errorf("hint should name the attempt; got %q", got)
	}
	if !strings.Contains(got, "without producing a ChangeReport") {
		t.Errorf("hint should explain missing report; got %q", got)
	}
}

// TestBuildRetryHint_WithFailingTests checks that failing test
// names + their Suite + the multi-line failure signal surface so the
// reflector / planner sees the actual assertion (not just the first
// line, which on real runners is fixture noise). List capped at 3.
func TestBuildRetryHint_WithFailingTests(t *testing.T) {
	report := &types.ChangeReport{
		FailureSummary: "3 of 5 tests failed in handler module",
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Suite: "pkg", Passed: false, FailureDetail: "expected 42, got 7\nmore stack info"},
			{AssertionID: "TestB", Passed: true},
			{AssertionID: "TestC", Suite: "pkg", Passed: false, FailureDetail: "nil pointer in handler.go:88"},
			{AssertionID: "TestD", Passed: false},
			{AssertionID: "TestE", Passed: false},
			{AssertionID: "TestF", Passed: false},
		},
	}
	got := buildRetryHint(report, nil, 2)
	if !strings.Contains(got, "attempt 2") {
		t.Errorf("hint should name the attempt; got %q", got)
	}
	if !strings.Contains(got, "3 of 5 tests failed") {
		t.Errorf("hint should include FailureSummary; got %q", got)
	}
	for _, name := range []string{"TestA", "TestC", "TestD"} {
		if !strings.Contains(got, name) {
			t.Errorf("hint should include failing test %s; got %q", name, got)
		}
	}
	if strings.Contains(got, "TestF") {
		t.Errorf("hint should truncate at 3 failing tests; got %q", got)
	}
	// New behavior: the multi-line signal extractor returns BOTH the
	// error line AND any stack frame that fits under the cap. The
	// Batch E robot-name failure proved that returning only the first
	// line surfaces fixture noise and hides the actual assertion.
	if !strings.Contains(got, "expected 42, got 7") {
		t.Errorf("hint should include FailureDetail's primary error line for TestA; got %q", got)
	}
	if !strings.Contains(got, "nil pointer in handler.go:88") {
		t.Errorf("hint should include FailureDetail for TestC; got %q", got)
	}
	if !strings.Contains(got, "TestA (pkg)") {
		t.Errorf("hint should render Suite suffix; got %q", got)
	}
}

// TestBuildRetryHint_WithSuspectFileList checks that when a plan is
// passed, its TargetPaths render as a "suspect list" so the planner
// knows which files to re-examine.
func TestBuildRetryHint_WithSuspectFileList(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-x",
		TargetPaths: []string{"a.go", "b.go", "c.go"},
	}
	report := &types.ChangeReport{
		TestResults: []types.TestResult{
			{AssertionID: "TestBad", Passed: false, FailureDetail: "boom"},
		},
	}
	got := buildRetryHint(report, plan, 1)
	if !strings.Contains(got, "suspect list") {
		t.Errorf("hint should flag paths as suspects; got %q", got)
	}
	for _, p := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(got, p) {
			t.Errorf("hint should include path %s; got %q", p, got)
		}
	}
}

// TestBuildRetryHint_SuspectFileListCap caps the path list at 10.
func TestBuildRetryHint_SuspectFileListCap(t *testing.T) {
	paths := make([]string, 15)
	for i := range paths {
		paths[i] = fmt.Sprintf("file%02d.go", i)
	}
	plan := &types.ChangePlan{TargetPaths: paths}
	got := buildRetryHint(nil, plan, 1)
	// First 10 render, last 5 collapse into "+N more".
	if !strings.Contains(got, "file00.go") || !strings.Contains(got, "file09.go") {
		t.Errorf("hint should include first 10 paths; got %q", got)
	}
	if strings.Contains(got, "file10.go") {
		t.Errorf("hint should cap before file10.go; got %q", got)
	}
	if !strings.Contains(got, "(+5 more)") {
		t.Errorf("hint should surface overflow count; got %q", got)
	}
}

// TestBuildRetryHint_PlanWithoutReport exercises the apply-failure
// case: the plan landed but verify never produced a report.
func TestBuildRetryHint_PlanWithoutReport(t *testing.T) {
	plan := &types.ChangePlan{TargetPaths: []string{"a.go"}}
	got := buildRetryHint(nil, plan, 1)
	if !strings.Contains(got, "without producing a ChangeReport") {
		t.Errorf("hint should explain missing report; got %q", got)
	}
	if !strings.Contains(got, "a.go") {
		t.Errorf("hint should still show suspect files; got %q", got)
	}
}

// TestBuildRetryHint_LongSummaryTruncated verifies the 300-char
// clamp on FailureSummary so one verbose verifier doesn't blow up
// the planner prompt.
func TestBuildRetryHint_LongSummaryTruncated(t *testing.T) {
	long := strings.Repeat("x", 500)
	report := &types.ChangeReport{FailureSummary: long}
	got := buildRetryHint(report, nil, 1)
	if !strings.Contains(got, "…") {
		t.Errorf("oversized summary should be truncated with ellipsis; got %q", got)
	}
	// Total hint stays bounded (new enriched cap: 1500 chars).
	if len(got) > 1500 {
		t.Errorf("hint length %d exceeds safety cap; got %q", len(got), got)
	}
}

// TestBuildRetryHint_LongDetailTruncated caps each failing test's
// FailureDetail at 600 chars (upgraded from 140 for the multi-line
// signal extractor) so a verbose stack doesn't swamp the hint.
func TestBuildRetryHint_LongDetailTruncated(t *testing.T) {
	long := strings.Repeat("z", 1000)
	report := &types.ChangeReport{
		TestResults: []types.TestResult{
			{AssertionID: "TestBig", Passed: false, FailureDetail: long},
		},
	}
	got := buildRetryHint(report, nil, 1)
	if !strings.Contains(got, "…") {
		t.Errorf("oversized detail should be ellipsis-truncated; got %q", got)
	}
	if len(got) > 1500 {
		t.Errorf("hint length %d exceeds safety cap", len(got))
	}
}

// TestSetWriteRetryBudget_Clamping locks the [0,5] clamp on the
// setter so misconfigured yaml cannot burn LLM tokens unbounded.
func TestSetWriteRetryBudget_Clamping(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.SetWriteRetryBudget(-3)
	if got := o.WriteRetryBudget(); got != 0 {
		t.Errorf("negative input should clamp to 0; got %d", got)
	}
	o.SetWriteRetryBudget(10)
	if got := o.WriteRetryBudget(); got != 5 {
		t.Errorf("above-cap input should clamp to 5; got %d", got)
	}
	o.SetWriteRetryBudget(3)
	if got := o.WriteRetryBudget(); got != 3 {
		t.Errorf("legal input 3 should stay 3; got %d", got)
	}
}

// TestClearForReplan_ClearsStateAndSeedsHint drives clearForReplan
// directly: seed a "prior attempt" ChangeReport with a failure, a
// ChangePlan, a WriteClosure.AppliedSet, and a WorktreePath, then
// verify all four are reset and the PlanningHint is populated. T4
// renamed prepareVerifyRetry → clearForReplan + moved it into the
// stage_hooks.go file when write modes were folded into the
// scheduler.
func TestClearForReplan_ClearsStateAndSeedsHint(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	// Seed the bus ourselves since we bypass Run().
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main",
		RepoRoot:     "/tmp/worktree", // swapped during apply
		Mutable:      types.NewMutableState("probe"),
		WorktreePath: "/tmp/worktree",
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:          "plan-1",
		TargetPaths: []string{"a.go"},
		Changes:     []types.FileChange{{Path: "a.go", Kind: "modify"}},
	})
	o.busCtx.Mutable.SetChangeReport(&types.ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureSummary: "TestA failed",
		TestResults:    []types.TestResult{{AssertionID: "TestA", Passed: false}},
	})
	o.busCtx.Mutable.WriteClosure().MarkApplied("a.go")
	o.planPath = "/tmp/plan.json" // user-supplied plan file

	// clearForReplan calls worktree.DiscardByPath which is a no-op
	// for non-existent paths — passing /tmp/worktree is safe (it
	// doesn't actually exist) and DiscardByPath is idempotent.
	clearForReplan(o, 1)

	if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
		t.Error("ChangePlan should be reset on retry prep")
	}
	if report := o.busCtx.Mutable.ChangeReport(); report != nil {
		t.Error("ChangeReport should be reset on retry prep")
	}
	if applied := o.busCtx.Mutable.WriteClosure().AppliedSet(); len(applied) != 0 {
		t.Errorf("WriteClosure.AppliedSet should be reset; got %v", applied)
	}
	if o.busCtx.WorktreePath != "" {
		t.Errorf("WorktreePath should be cleared; got %q", o.busCtx.WorktreePath)
	}
	if o.busCtx.RepoRoot != "/tmp/main" {
		t.Errorf("RepoRoot should swap back to MainRepoRoot; got %q", o.busCtx.RepoRoot)
	}
	if o.planPath != "" {
		t.Error("user-supplied planPath should be cleared so retry re-plans")
	}
	hint := o.busCtx.Mutable.PlanningHint()
	if hint == "" {
		t.Fatal("PlanningHint should be seeded")
	}
	if !strings.Contains(hint, "TestA") {
		t.Errorf("hint should carry the failing test; got %q", hint)
	}
	if !strings.Contains(hint, "attempt") {
		t.Errorf("hint should name the prior attempt; got %q", hint)
	}
	// ChangePlan was populated before prepareVerifyRetry ran, so the
	// enriched hint must render the suspect-file list — proves the
	// plan was read BEFORE the subsequent reset cleared it.
	if !strings.Contains(hint, "a.go") {
		t.Errorf("hint should include prior plan's TargetPaths as suspect list; got %q", hint)
	}
}

// TestPlanningHintRoundTrip confirms the MutableState getter/setter
// pair works atomically — a belt-and-suspenders check before we
// rely on it from the planner's BuildInitialInstruction.
func TestPlanningHintRoundTrip(t *testing.T) {
	mu := types.NewMutableState("probe")
	if got := mu.PlanningHint(); got != "" {
		t.Errorf("zero-value hint should be empty; got %q", got)
	}
	mu.SetPlanningHint("prior failure")
	if got := mu.PlanningHint(); got != "prior failure" {
		t.Errorf("hint round-trip failed; got %q", got)
	}
	mu.ResetPlanningHint()
	if got := mu.PlanningHint(); got != "" {
		t.Errorf("reset should clear hint; got %q", got)
	}
}

// TestBaselineReportRoundTrip locks the MutableState slot behaviour
// so CritNoRegression can read what runApplyPhase writes.
func TestBaselineReportRoundTrip(t *testing.T) {
	mu := types.NewMutableState("probe")
	if got := mu.BaselineReport(); got != nil {
		t.Errorf("zero-value baseline should be nil; got %v", got)
	}
	baseline := &types.ChangeReport{PlanID: "baseline-1", Passed: true}
	mu.SetBaselineReport(baseline)
	if got := mu.BaselineReport(); got != baseline {
		t.Error("baseline round-trip failed")
	}
	mu.ResetBaselineReport()
	if got := mu.BaselineReport(); got != nil {
		t.Errorf("reset should clear baseline; got %v", got)
	}
}
