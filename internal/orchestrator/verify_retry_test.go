package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
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
	if !strings.Contains(got, "Repair scope") {
		t.Errorf("hint should tell planner to keep repair scoped to the failing batch; got %q", got)
	}
	if !strings.Contains(got, "state the new path and reason") {
		t.Errorf("hint should require explicit scope expansion rationale; got %q", got)
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

// TestBuildRetryHint_PassesFullSummary locks the post-2026-04-30
// red-line refactor: FailureSummary is propagated VERBATIM to the
// LLM-facing retry hint, no truncation. Truncating the summary
// drops the actual error-bearing line (often 10-15 lines deep into
// pytest fixture noise) and forces the model to guess the cause
// from header preamble — the principle "feed the complete error to
// the model, no system-side editing" requires the full payload.
// Operator-facing log lines have their own caps; this path is
// LLM-facing context.
func TestBuildRetryHint_PassesFullSummary(t *testing.T) {
	long := strings.Repeat("x", 500)
	report := &types.ChangeReport{FailureSummary: long}
	got := buildRetryHint(report, nil, 1)
	if !strings.Contains(got, long) {
		t.Errorf("retry hint must propagate the full FailureSummary verbatim; got %q", got)
	}
	// Ellipsis truncation marker must NOT appear — the model needs
	// the complete error to reason about.
	if strings.Contains(got, "x…") || strings.Contains(strings.TrimSpace(got), long+"…") {
		t.Errorf("retry hint must not truncate the FailureSummary; got %q", got)
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

// TestClearForReplan_PreservesPhaseContext pins commit 23's
// stage II fix: when an intra-phase verify→plan retry fires
// inside multi-phase, the orchestrator's phaseContextPrefix
// header gets re-prepended onto the retry hint so the planner
// dispatching the retry still sees "## Phase 2 of 3: <goal>".
// Without this prepend, the consume-once PlanningHint had
// already been drained by the first dispatch, and the retry
// hint would replace it without any phase boundary.
func TestClearForReplan_PreservesPhaseContext(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main",
		RepoRoot:     "/tmp/worktree",
		Mutable:      types.NewMutableState("probe"),
		WorktreePath: "/tmp/worktree",
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:          "plan-phase2",
		TargetPaths: []string{"x.go"},
	})
	o.busCtx.Mutable.SetChangeReport(&types.ChangeReport{
		PlanID:      "plan-phase2",
		Passed:      false,
		TestResults: []types.TestResult{{AssertionID: "TestX", Passed: false}},
	})
	o.phaseContextPrefix = "## Phase 2 of 3: update ORM\n\n"

	clearForReplan(o, 1)

	hint := o.busCtx.Mutable.PlanningHint()
	if !strings.HasPrefix(hint, "## Phase 2 of 3:") {
		t.Errorf("retry hint should start with the phase header; got %q", hint)
	}
	if !strings.Contains(hint, "TestX") {
		t.Errorf("retry hint should still carry the failure context after the phase prefix; got %q", hint)
	}
}

// TestClearForReplan_NoPhasePrefixOnSinglePhase pins the
// back-compat invariant: single-phase Runs never set
// phaseContextPrefix, so the retry hint stays byte-identical
// to its pre-stage-II shape.
func TestClearForReplan_NoPhasePrefixOnSinglePhase(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main",
		RepoRoot:     "/tmp/worktree",
		Mutable:      types.NewMutableState("probe"),
		WorktreePath: "/tmp/worktree",
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{ID: "plan-1", TargetPaths: []string{"a.go"}})
	o.busCtx.Mutable.SetChangeReport(&types.ChangeReport{
		PlanID:      "plan-1",
		Passed:      false,
		TestResults: []types.TestResult{{AssertionID: "TestA", Passed: false}},
	})

	clearForReplan(o, 1)

	hint := o.busCtx.Mutable.PlanningHint()
	if strings.HasPrefix(hint, "## Phase ") {
		t.Errorf("single-phase Run should not get a phase prefix; got %q", hint)
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

// TestBuildRetryHintWithBest_NoRegressionEqualsPlainHint verifies
// that when no earlier iteration outscored the current one, the
// retry hint is byte-for-byte identical to buildRetryHint's output.
// This preserves the prior behaviour on monotonic-improvement
// trajectories — the new "Regression detected" section only fires
// when there is actual ground to lose.
func TestBuildRetryHintWithBest_NoRegressionEqualsPlainHint(t *testing.T) {
	cur := &types.ChangeReport{
		FailureSummary: "1 of 10 tests failed",
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Passed: true},
			{AssertionID: "TestB", Passed: false, FailureDetail: "boom"},
		},
	}
	curPlan := &types.ChangePlan{TargetPaths: []string{"a.go"}}

	// No best at all.
	plain := buildRetryHint(cur, curPlan, 1)
	withNilBest := buildRetryHintWithBest(cur, curPlan, nil, nil, 1)
	if plain != withNilBest {
		t.Errorf("nil best should equal plain hint;\nplain:\n%s\nwithNilBest:\n%s", plain, withNilBest)
	}

	// Best equal to current — IsBetterThan returns false on tie.
	withTieBest := buildRetryHintWithBest(cur, curPlan, cur, curPlan, 1)
	if plain != withTieBest {
		t.Errorf("equal-score best should not annotate;\nplain:\n%s\nwithTieBest:\n%s", plain, withTieBest)
	}

	// Best strictly worse — also no annotation.
	worseBest := &types.ChangeReport{
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Passed: false},
		},
	}
	withWorseBest := buildRetryHintWithBest(cur, curPlan, worseBest, curPlan, 1)
	if plain != withWorseBest {
		t.Errorf("worse best should not annotate;\nplain:\n%s\nwithWorseBest:\n%s", plain, withWorseBest)
	}
}

// TestBuildRetryHintWithBest_RegressionAnnotated verifies that when
// the best-known-good is strictly better than the current iteration,
// the hint adds a "Regression detected" section with the score delta
// and the best plan's TargetPaths so the planner can see what edits
// to preserve. This is the load-bearing case behind the latch.
func TestBuildRetryHintWithBest_RegressionAnnotated(t *testing.T) {
	cur := &types.ChangeReport{
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Passed: true},
			{AssertionID: "TestB", Passed: false, FailureDetail: "regression"},
			{AssertionID: "TestC", Passed: false, FailureDetail: "regression"},
		},
	}
	best := &types.ChangeReport{
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Passed: true},
			{AssertionID: "TestB", Passed: true},
			{AssertionID: "TestC", Passed: false, FailureDetail: "expected"},
		},
	}
	curPlan := &types.ChangePlan{TargetPaths: []string{"now.go"}}
	bestPlan := &types.ChangePlan{TargetPaths: []string{"original.go", "shared.go"}}

	got := buildRetryHintWithBest(cur, curPlan, best, bestPlan, 2)
	for _, marker := range []string{
		"Regression detected",
		"1/3", // current passed/total
		"2/3", // best passed/total
		"original.go",
		"shared.go",
	} {
		if !strings.Contains(got, marker) {
			t.Errorf("regression hint missing marker %q in:\n%s", marker, got)
		}
	}
}

// TestBuildPlanContentDiff_ShowsLineDelta locks the core regression-
// hint enrichment: when best and current plans modify the same path
// with different NewContent, the diff shows the lines that changed.
// This is what closes the information gap that left forth-py's
// planner reconstructing from memory.
func TestBuildPlanContentDiff_ShowsLineDelta(t *testing.T) {
	best := &types.ChangePlan{Changes: []types.FileChange{{
		Path:       "forth.py",
		Kind:       "modify",
		NewContent: "def lookup(name):\n    return user_words.get(name)\n",
	}}}
	current := &types.ChangePlan{Changes: []types.FileChange{{
		Path:       "forth.py",
		Kind:       "modify",
		NewContent: "def lookup(name):\n    return builtins.get(name)\n",
	}}}
	got := buildPlanContentDiff(best, current, 4096)
	if got == "" {
		t.Fatal("diff should be non-empty when content differs")
	}
	for _, marker := range []string{"forth.py", "-", "+", "user_words", "builtins"} {
		if !strings.Contains(got, marker) {
			t.Errorf("diff should include %q; got:\n%s", marker, got)
		}
	}
}

// TestBuildPlanContentDiff_NoOverlapReturnsEmpty: plans touching
// disjoint paths produce no diff (no overlap = no comparison
// possible).
func TestBuildPlanContentDiff_NoOverlapReturnsEmpty(t *testing.T) {
	best := &types.ChangePlan{Changes: []types.FileChange{{Path: "a.go", Kind: "create", NewContent: "package a"}}}
	current := &types.ChangePlan{Changes: []types.FileChange{{Path: "b.go", Kind: "create", NewContent: "package b"}}}
	if got := buildPlanContentDiff(best, current, 4096); got != "" {
		t.Errorf("disjoint paths should yield empty diff; got:\n%s", got)
	}
}

// TestBuildPlanContentDiff_IdenticalContentReturnsEmpty: when
// best and current have the SAME NewContent for the same path,
// no diff is produced (no signal to surface).
func TestBuildPlanContentDiff_IdenticalContentReturnsEmpty(t *testing.T) {
	same := types.FileChange{Path: "x.py", Kind: "modify", NewContent: "x = 1\n"}
	best := &types.ChangePlan{Changes: []types.FileChange{same}}
	current := &types.ChangePlan{Changes: []types.FileChange{same}}
	if got := buildPlanContentDiff(best, current, 4096); got != "" {
		t.Errorf("identical content should yield empty diff; got:\n%s", got)
	}
}

// TestBuildPlanContentDiff_TotalCapTruncation: oversize diffs are
// truncated to maxBytes with an explicit marker.
func TestBuildPlanContentDiff_TotalCapTruncation(t *testing.T) {
	huge := strings.Repeat("line\n", 2000) // ~10 KB
	best := &types.ChangePlan{Changes: []types.FileChange{{
		Path:       "big.txt",
		Kind:       "modify",
		NewContent: huge,
	}}}
	current := &types.ChangePlan{Changes: []types.FileChange{{
		Path:       "big.txt",
		Kind:       "modify",
		NewContent: huge + "trailing\n",
	}}}
	got := buildPlanContentDiff(best, current, 512)
	if got == "" {
		t.Fatal("oversize diff should still produce output before truncation")
	}
	// Total stays well under 1 KB (cap=512 + small overhead).
	if len(got) > 1024 {
		t.Errorf("diff length %d exceeds cap with margin; got:\n%s", len(got), got)
	}
}

// TestBuildPlanContentDiff_NilPlansReturnsEmpty: defensive — nil
// best or nil current both short-circuit to empty so callers don't
// have to nil-check.
func TestBuildPlanContentDiff_NilPlansReturnsEmpty(t *testing.T) {
	plan := &types.ChangePlan{Changes: []types.FileChange{{Path: "a", Kind: "create", NewContent: "x"}}}
	if got := buildPlanContentDiff(nil, plan, 4096); got != "" {
		t.Errorf("nil best should yield empty; got %s", got)
	}
	if got := buildPlanContentDiff(plan, nil, 4096); got != "" {
		t.Errorf("nil current should yield empty; got %s", got)
	}
}

// TestBuildRetryHintWithBest_IncludesDiffOnRegression: end-to-end
// integration — the full retry hint includes the unified diff
// section when regression is detected, and the diff carries the
// content delta the planner should review.
func TestBuildRetryHintWithBest_IncludesDiffOnRegression(t *testing.T) {
	bestReport := &types.ChangeReport{
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, Passed: true},
			{Kind: types.TestResultKindUnit, Passed: true},
			{Kind: types.TestResultKindUnit, Passed: false},
		},
	}
	curReport := &types.ChangeReport{
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, Passed: true},
			{Kind: types.TestResultKindUnit, Passed: false},
			{Kind: types.TestResultKindUnit, Passed: false},
		},
	}
	bestPlan := &types.ChangePlan{
		TargetPaths: []string{"forth.py"},
		Changes: []types.FileChange{{
			Path:       "forth.py",
			Kind:       "modify",
			NewContent: "BEST_VERSION = True\n",
		}},
	}
	curPlan := &types.ChangePlan{
		TargetPaths: []string{"forth.py"},
		Changes: []types.FileChange{{
			Path:       "forth.py",
			Kind:       "modify",
			NewContent: "REGRESSED_VERSION = True\n",
		}},
	}
	got := buildRetryHintWithBest(curReport, curPlan, bestReport, bestPlan, 2)
	for _, marker := range []string{
		"Regression detected",
		"```diff",
		"BEST_VERSION",
		"REGRESSED_VERSION",
	} {
		if !strings.Contains(got, marker) {
			t.Errorf("hint missing %q in:\n%s", marker, got)
		}
	}
}

// TestClearForReplan_WarmRewindWhenBestSHAAvailable verifies the
// load-bearing warm-worktree behavior: when a best-known-good SHA
// is set on the orchestrator and the worktree exists, clearForReplan
// MUST `git reset --hard <bestSHA>` the existing worktree (not
// discard + recreate) so the next iteration's planner reads BEST
// code as its working baseline.
//
// The test seeds two commits in a real worktree (best=BEST, regressed
// =REGRESSED) and confirms clearForReplan rewinds the file content
// back to BEST without destroying the worktree.
func TestClearForReplan_WarmRewindWhenBestSHAAvailable(t *testing.T) {
	// Build a fresh main repo + worktree.
	mainRoot := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	gitInit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = mainRoot
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitInit("init", "-q")
	gitInit("config", "user.email", "test@local")
	gitInit("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(mainRoot, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInit("add", ".")
	gitInit("commit", "-q", "-m", "seed")

	wtBase := filepath.Join(t.TempDir(), "worktrees")
	sess, err := worktree.Create(wtBase, mainRoot, "trace-warmretry-test")
	if err != nil {
		t.Fatalf("worktree.Create: %v", err)
	}
	wtPath := sess.Path()

	target := filepath.Join(wtPath, "out.txt")
	// Best iteration (commit A).
	if err := os.WriteFile(target, []byte("BEST\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bestSHA, err := worktree.CommitChanges(wtPath, "iter best")
	if err != nil {
		t.Fatal(err)
	}
	// Regressed iteration (commit B).
	if err := os.WriteFile(target, []byte("REGRESSED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.CommitChanges(wtPath, "iter regressed"); err != nil {
		t.Fatal(err)
	}

	// Stand up an orchestrator with the warm-retry slot pre-seeded.
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: mainRoot,
		RepoRoot:     wtPath,
		WorktreePath: wtPath,
		Mutable:      types.NewMutableState("probe"),
	}
	o.bestAppliedCommitSHA = bestSHA

	clearForReplan(o, 1)

	// Worktree should still exist (not discarded).
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree should be preserved across warm rewind; got stat err: %v", err)
	}
	// Working-tree file should now match BEST iteration.
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if strings.TrimSpace(string(data)) != "BEST" {
		t.Errorf("after clearForReplan, file should hold BEST content; got %q", data)
	}
	// busCtx.WorktreePath stays populated; RepoRoot stays at worktree
	// (NOT reset back to MainRepoRoot).
	if o.busCtx.WorktreePath != wtPath {
		t.Errorf("WorktreePath should be preserved; got %q", o.busCtx.WorktreePath)
	}
	if o.busCtx.RepoRoot != wtPath {
		t.Errorf("RepoRoot should stay at worktree (not reset to main); got %q", o.busCtx.RepoRoot)
	}

	_ = worktree.DiscardByPath(wtPath, mainRoot)
}

// TestClearForReplan_ColdDiscardWhenNoBestSHA verifies the fallback:
// when bestAppliedCommitSHA is empty (iter 0 hasn't produced any
// committed checkpoint yet), clearForReplan keeps its historical
// behavior — discard worktree and reset RepoRoot to MainRepoRoot.
func TestClearForReplan_ColdDiscardWhenNoBestSHA(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main-cold",
		RepoRoot:     "/tmp/worktree-cold",
		WorktreePath: "/tmp/worktree-cold", // path doesn't exist; DiscardByPath is idempotent
		Mutable:      types.NewMutableState("probe"),
	}
	// bestAppliedCommitSHA empty — fallback path

	clearForReplan(o, 1)

	if o.busCtx.WorktreePath != "" {
		t.Errorf("cold path: WorktreePath should be cleared; got %q", o.busCtx.WorktreePath)
	}
	if o.busCtx.RepoRoot != "/tmp/main-cold" {
		t.Errorf("cold path: RepoRoot should reset to MainRepoRoot; got %q", o.busCtx.RepoRoot)
	}
}

// TestRestoreBestIfRegressed_PersistsAfterPlanPathCleared verifies
// the followup-bug fix: clearForReplan wipes busCtx.PlanPath between
// retry iterations, but saveChangeReport must still be able to
// persist the restored ChangeReport because o.reportDir was captured
// at Run entry. Without this fallback the restored 51/54 (from the
// best iteration) never reaches disk and resummarize tools see only
// the first iteration's stale report.
func TestRestoreBestIfRegressed_PersistsAfterPlanPathCleared(t *testing.T) {
	tmp := t.TempDir()
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main",
		RepoRoot:     "/tmp/main",
		Mutable:      types.NewMutableState("probe"),
		// PlanPath is intentionally empty — simulates the post-clearForReplan
		// state where the retry loop is mid-flight and busCtx.PlanPath
		// has been wiped.
		PlanPath: "",
	}
	o.reportDir = tmp // captured at Run entry, survives retries

	// Best (the iteration we want preserved on disk).
	bestPlan := &types.ChangePlan{ID: "plan-best", TargetPaths: []string{"forth.py"}}
	bestReport := &types.ChangeReport{
		PlanID: "plan-best",
		Passed: false, // 51/54 — better than current but still failing
		TestResults: func() []types.TestResult {
			out := make([]types.TestResult, 0, 54)
			for i := 0; i < 51; i++ {
				out = append(out, types.TestResult{Kind: types.TestResultKindUnit, Passed: true})
			}
			for i := 0; i < 3; i++ {
				out = append(out, types.TestResult{Kind: types.TestResultKindUnit, Passed: false})
			}
			return out
		}(),
	}
	o.busCtx.Mutable.SetBestPlanReport(bestPlan, bestReport)

	// Current — last iteration regressed to 0/54.
	curPlan := &types.ChangePlan{ID: "plan-current", TargetPaths: []string{"forth.py"}}
	curReport := &types.ChangeReport{
		PlanID: "plan-current",
		Passed: false,
		TestResults: func() []types.TestResult {
			out := make([]types.TestResult, 0, 54)
			for i := 0; i < 54; i++ {
				out = append(out, types.TestResult{Kind: types.TestResultKindUnit, Passed: false})
			}
			return out
		}(),
	}
	o.busCtx.Mutable.SetChangePlan(curPlan)
	o.busCtx.Mutable.SetChangeReport(curReport)

	restoreBestIfRegressed(o)

	// Mutable now holds the best.
	gotPlan := o.busCtx.Mutable.ChangePlan()
	if gotPlan == nil || gotPlan.ID != "plan-best" {
		t.Errorf("Mutable.ChangePlan should be restored to best; got %v", gotPlan)
	}
	gotReport := o.busCtx.Mutable.ChangeReport()
	if gotReport == nil || gotReport.PlanID != "plan-best" {
		t.Errorf("Mutable.ChangeReport should be restored to best; got %v", gotReport)
	}

	// The disk artifact MUST exist under the captured reportDir.
	wantPath := filepath.Join(tmp, "plan-best.report.json")
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("restored report not persisted to disk: %v (expected at %s)", err, wantPath)
	}

	// Mutable.Result reflects the restored (still-failing) verdict, not
	// a stale "0/54" rendering.
	result := o.busCtx.Mutable.Result()
	// Match either localised heading; the verify failure renderer
	// follows BusContext.Language (zh default, en flips on
	// --lang=en).
	// Header wording was de-jargonised post-2026-04-30 (no "Verify"
	// internal stage name; see renderVerifyFailure docblock). Match
	// the new user-language headers in either locale.
	if !strings.Contains(result, "Tests did not pass") && !strings.Contains(result, "测试未通过") {
		t.Errorf("Mutable.Result should be re-rendered for restored report; got %q", result)
	}
}

// TestRestoreBestIfRegressed_NoOpWhenCurrentIsBest verifies the
// happy path: when the current ChangeReport is the best (or no
// retry latch fired), restoreBestIfRegressed is a no-op — Mutable
// is untouched and no spurious report.json is written.
func TestRestoreBestIfRegressed_NoOpWhenCurrentIsBest(t *testing.T) {
	tmp := t.TempDir()
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("probe"),
	}
	o.reportDir = tmp

	curPlan := &types.ChangePlan{ID: "plan-current"}
	curReport := &types.ChangeReport{
		PlanID: "plan-current",
		Passed: true,
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, Passed: true},
		},
	}
	o.busCtx.Mutable.SetChangePlan(curPlan)
	o.busCtx.Mutable.SetChangeReport(curReport)

	restoreBestIfRegressed(o)

	if o.busCtx.Mutable.ChangePlan().ID != "plan-current" {
		t.Error("happy-path no-op: ChangePlan should be untouched")
	}
	if o.busCtx.Mutable.ChangeReport().PlanID != "plan-current" {
		t.Error("happy-path no-op: ChangeReport should be untouched")
	}
	// No stray report file should have been written.
	matches, _ := filepath.Glob(filepath.Join(tmp, "*.report.json"))
	if len(matches) != 0 {
		t.Errorf("happy-path no-op: no report file should be written; got %v", matches)
	}
}

// TestSaveChangeReport_ReportDirFallback verifies the fallback chain:
// when busCtx.PlanPath is empty (typical post-retry state) but
// o.reportDir is set, saveChangeReport persists under reportDir
// instead of skipping with a "no plan dir" warning.
func TestSaveChangeReport_ReportDirFallback(t *testing.T) {
	tmp := t.TempDir()
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("probe"),
	}
	o.reportDir = tmp

	report := &types.ChangeReport{
		PlanID: "plan-fallback",
		Passed: true,
	}
	o.saveChangeReport(report)

	wantPath := filepath.Join(tmp, "plan-fallback.report.json")
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("reportDir fallback failed: %v (expected at %s)", err, wantPath)
	}
}

func TestSaveChangeReport_BackfillsPlanIDFromMutablePlan(t *testing.T) {
	tmp := t.TempDir()
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("probe"),
	}
	o.reportDir = tmp
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{ID: "plan-current"})

	report := &types.ChangeReport{
		Passed: true,
	}
	o.saveChangeReport(report)

	if report.PlanID != "plan-current" {
		t.Fatalf("report PlanID should be backfilled from current ChangePlan; got %q", report.PlanID)
	}
	wantPath := filepath.Join(tmp, "plan-current.report.json")
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("backfilled report not persisted: %v (expected at %s)", err, wantPath)
	}
}

func TestSaveChangeReport_PersistsPlanThroughSaverWhenNoPlanPath(t *testing.T) {
	tmp := t.TempDir()
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.SetPlanSaver(&testPlanSaver{dir: tmp})
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("probe"),
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{ID: "plan-durable"})

	report := &types.ChangeReport{Passed: true}
	o.saveChangeReport(report)

	wantPlan := filepath.Join(tmp, "plan-durable.json")
	if o.busCtx.PlanPath != wantPlan {
		t.Fatalf("PlanPath should be rebound to saved plan path; got %q want %q", o.busCtx.PlanPath, wantPlan)
	}
	if _, err := os.Stat(wantPlan); err != nil {
		t.Fatalf("plan saver did not persist plan: %v", err)
	}
	wantReport := filepath.Join(tmp, "plan-durable.report.json")
	if _, err := os.Stat(wantReport); err != nil {
		t.Errorf("report not persisted next to saved plan: %v (expected at %s)", err, wantReport)
	}
}

func TestPersistCurrentChangePlanSnapshot_PreservesApplyRecord(t *testing.T) {
	tmp := t.TempDir()
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.SetPlanSaver(&testPlanSaver{dir: tmp})
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("probe"),
	}
	plan := &types.ChangePlan{
		ID:     "plan-apply-record",
		Status: types.PlanStatusPending,
		Changes: []types.FileChange{{
			Path: "file.txt",
			Kind: "patch",
			Apply: &types.FileChangeApplyRecord{
				Status: "applied",
				Source: "structured_builder",
				Engine: "git_apply",
			},
		}},
		TargetPaths: []string{"file.txt"},
	}
	o.busCtx.Mutable.SetChangePlan(plan)

	o.persistCurrentChangePlanSnapshot()

	loaded, err := types.LoadChangePlanFromFile(filepath.Join(tmp, "plan-apply-record.json"))
	if err != nil {
		t.Fatalf("load persisted plan: %v", err)
	}
	if loaded.Changes[0].Apply == nil {
		t.Fatal("apply record should persist with full plan snapshot")
	}
	if loaded.Changes[0].Apply.Source != "structured_builder" || loaded.Changes[0].Apply.Engine != "git_apply" {
		t.Fatalf("unexpected persisted apply record: %+v", loaded.Changes[0].Apply)
	}
}

func TestSaveChangeReport_WorkDirFallbackWhenNoPlanStore(t *testing.T) {
	tmp := t.TempDir()
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		WorkDir: tmp,
		Mutable: types.NewMutableState("probe"),
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{ID: "plan-workdir"})

	report := &types.ChangeReport{Passed: true}
	o.saveChangeReport(report)

	wantPath := filepath.Join(tmp, "plans", "plan-workdir.report.json")
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("workdir report fallback failed: %v (expected at %s)", err, wantPath)
	}
}

func TestWriteWorkflowReportID_UsesArtifactSafeStem(t *testing.T) {
	report := &types.ChangeReport{PlanID: "../plan unsafe"}
	if got := writeWorkflowReportID(report); got != "_plan_unsafe.report.json" {
		t.Fatalf("report id should use artifact-safe stem; got %q", got)
	}
}

type testPlanSaver struct {
	dir string
}

func (s *testPlanSaver) Save(plan *types.ChangePlan) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil testPlanSaver")
	}
	path := filepath.Join(s.dir, plan.ID+".json")
	if err := types.WritePlanToFile(plan, path); err != nil {
		return "", err
	}
	return path, nil
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

// TestClearForReplan_AppendsIterationLedger pins Module C's
// load-bearing wiring: clearForReplan must capture verbatim plan +
// report into Mutable.iterationLedger BEFORE the resets fire, so the
// next planner dispatch reads the FULL prior attempt as raw data
// (not pre-classified into "regression" or "plateau" labels).
func TestClearForReplan_AppendsIterationLedger(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		MainRepoRoot: "/tmp/main",
		RepoRoot:     "/tmp/main",
		Mutable:      types.NewMutableState("probe"),
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:          "plan-iter1",
		TargetPaths: []string{"a.go", "b.go"},
		Summary:     "Add foo, refactor bar.",
	})
	o.busCtx.Mutable.SetChangeReport(&types.ChangeReport{
		PlanID:                "plan-iter1",
		Passed:                false,
		FailureSummary:        "TestFoo failed: expected 5 got 4",
		FailureSummaryBlobRef: "/tmp/codrax/blob/run_tests-xyz.txt",
		TestResults: []types.TestResult{
			{AssertionID: "TestFoo", Passed: false},
			{AssertionID: "TestBar", Passed: true},
		},
	})

	clearForReplan(o, 1)

	ledger := o.busCtx.Mutable.IterationLedger()
	if len(ledger) != 1 {
		t.Fatalf("ledger should have 1 entry after clearForReplan; got %d", len(ledger))
	}
	got := ledger[0]
	if got.Attempt != 1 {
		t.Errorf("Attempt = %d; want 1", got.Attempt)
	}
	if got.PlanID != "plan-iter1" {
		t.Errorf("PlanID = %q; want plan-iter1", got.PlanID)
	}
	if got.PlanSummary != "Add foo, refactor bar." {
		t.Errorf("PlanSummary should be verbatim; got %q", got.PlanSummary)
	}
	if got.PassedCount != 1 || got.FailedCount != 1 {
		t.Errorf("counts wrong: passed=%d failed=%d", got.PassedCount, got.FailedCount)
	}
	if got.FailureSummary != "TestFoo failed: expected 5 got 4" {
		t.Errorf("FailureSummary should be verbatim; got %q", got.FailureSummary)
	}
	if got.FailureSummaryBlobRef != "/tmp/codrax/blob/run_tests-xyz.txt" {
		t.Errorf("FailureSummaryBlobRef should propagate; got %q", got.FailureSummaryBlobRef)
	}
	if len(got.ChangedFiles) != 2 || got.ChangedFiles[0] != "a.go" || got.ChangedFiles[1] != "b.go" {
		t.Errorf("ChangedFiles wrong; got %v", got.ChangedFiles)
	}
}
