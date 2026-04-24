package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBuildRetryHint_NilReport verifies that a retry dispatched
// before any ChangeReport was produced still yields a non-empty
// hint so the planner knows "something broke, start over".
func TestBuildRetryHint_NilReport(t *testing.T) {
	got := buildRetryHint(nil, 1)
	if got == "" {
		t.Fatal("nil report should still yield a hint")
	}
	if !strings.Contains(got, "attempt 1") {
		t.Errorf("hint should name the attempt; got %q", got)
	}
	if !strings.Contains(got, "from scratch") {
		t.Errorf("hint should direct a scratch restart; got %q", got)
	}
}

// TestBuildRetryHint_WithFailingTests checks that failing test
// names show up (bounded) and a truncation marker appears when
// the list exceeds the cap.
func TestBuildRetryHint_WithFailingTests(t *testing.T) {
	report := &types.ChangeReport{
		FailureSummary: "3 of 5 tests failed in handler module",
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Passed: false},
			{AssertionID: "TestB", Passed: true},
			{AssertionID: "TestC", Passed: false},
			{AssertionID: "TestD", Passed: false},
			{AssertionID: "TestE", Passed: false},
			{AssertionID: "TestF", Passed: false},
		},
	}
	got := buildRetryHint(report, 2)
	if !strings.Contains(got, "attempt 2") {
		t.Errorf("hint should name the attempt; got %q", got)
	}
	if !strings.Contains(got, "3 of 5 tests failed") {
		t.Errorf("hint should include FailureSummary; got %q", got)
	}
	// First 3 failing tests should appear.
	for _, name := range []string{"TestA", "TestC", "TestD"} {
		if !strings.Contains(got, name) {
			t.Errorf("hint should include failing test %s; got %q", name, got)
		}
	}
	// TestE / TestF should NOT be named (cap is 3).
	if strings.Contains(got, "TestF") {
		t.Errorf("hint should truncate at 3 failing tests; got %q", got)
	}
}

// TestBuildRetryHint_LongSummaryTruncated verifies the 300-char
// clamp on FailureSummary so one verbose verifier doesn't blow up
// the planner prompt.
func TestBuildRetryHint_LongSummaryTruncated(t *testing.T) {
	long := strings.Repeat("x", 500)
	report := &types.ChangeReport{FailureSummary: long}
	got := buildRetryHint(report, 1)
	if !strings.Contains(got, "…") {
		t.Errorf("oversized summary should be truncated with ellipsis; got %q", got)
	}
	// Total hint stays bounded.
	if len(got) > 700 {
		t.Errorf("hint length %d exceeds safety cap; got %q", len(got), got)
	}
}

// TestSetMaxVerifyRetries_Clamping locks the [0,5] clamp on the
// setter so misconfigured yaml cannot burn LLM tokens unbounded.
func TestSetMaxVerifyRetries_Clamping(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.SetMaxVerifyRetries(-3)
	if got := o.MaxVerifyRetries(); got != 0 {
		t.Errorf("negative input should clamp to 0; got %d", got)
	}
	o.SetMaxVerifyRetries(10)
	if got := o.MaxVerifyRetries(); got != 5 {
		t.Errorf("above-cap input should clamp to 5; got %d", got)
	}
	o.SetMaxVerifyRetries(3)
	if got := o.MaxVerifyRetries(); got != 3 {
		t.Errorf("legal input 3 should stay 3; got %d", got)
	}
}

// TestPrepareVerifyRetry_ClearsStateAndSeedsHint drives
// prepareVerifyRetry directly: seed a "prior attempt" ChangeReport
// with a failure, a ChangePlan, a WriteClosure.AppliedSet, and a
// WorktreePath, then verify all four are reset and the PlanningHint
// is populated.
func TestPrepareVerifyRetry_ClearsStateAndSeedsHint(t *testing.T) {
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

	// prepareVerifyRetry depends on worktree.DiscardByPath which is
	// a no-op for non-existent paths — passing /tmp/worktree is safe
	// (it doesn't actually exist) and DiscardByPath is idempotent.
	o.prepareVerifyRetry(2)

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
	if !strings.Contains(hint, "attempt 1") {
		t.Errorf("hint should name the prior attempt; got %q", hint)
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
