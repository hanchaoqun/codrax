package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBuildRetryHint_OOMTagPropagated locks the post-2026-04-30
// red-line refactor: the OOM-classified FailureKind surfaces ONE
// neutral structural tag ("out-of-memory") in the LLM-facing hint
// and the FailureSummary verbatim — NO prescribed-cause list, no
// "DO NOT raise the cap" anti-pattern paragraph. The model reads
// the raw stderr below and decides the fix; pre-classified prose
// telling it "Unbounded allocation is the most common cause" is the
// red-line violation we removed (system pre-classifies → LLM gets
// pushed toward a specific fix instead of reasoning from data).
func TestBuildRetryHint_OOMTagPropagated(t *testing.T) {
	report := &types.ChangeReport{
		Passed:         false,
		FailureKind:    types.FailureKindOOM,
		FailureSummary: "[python] command killed by memory cap (limit=2048 MiB)",
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindBuildError, AssertionID: "oom", Suite: "oom", Passed: false},
		},
	}
	plan := &types.ChangePlan{ID: "plan-x", TargetPaths: []string{"src/runaway.py"}}
	hint := buildRetryHint(report, plan, 1)

	if !strings.Contains(hint, "out-of-memory") {
		t.Errorf("OOM hint must surface the structural tag; got: %s", hint)
	}
	// Raw failure summary must propagate verbatim so the model sees
	// the actual error without truncation.
	if !strings.Contains(hint, report.FailureSummary) {
		t.Errorf("OOM hint must carry the raw FailureSummary verbatim; got: %s", hint)
	}
	// Forbidden: any prescribed-cause / corrective-direction prose.
	// These were the system-side keyword-classification leaks the
	// red-line refactor removed.
	forbidden := []string{
		"Unbounded allocation",
		"Most common cause",
		"DO NOT raise",
		"Revise the plan to:",
	}
	for _, f := range forbidden {
		if strings.Contains(hint, f) {
			t.Errorf("OOM hint must not contain prescribed-cause prose %q; got: %s", f, hint)
		}
	}
}

// TestBuildRetryHint_CPULimitTagPropagated mirrors the OOM test for
// CPU-limit failures — neutral tag + raw summary, no prescription.
func TestBuildRetryHint_CPULimitTagPropagated(t *testing.T) {
	report := &types.ChangeReport{
		Passed:         false,
		FailureKind:    types.FailureKindCPULimit,
		FailureSummary: "[python] command killed by CPU-time cap (limit=600s)",
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindBuildError, AssertionID: "cpu_limit", Suite: "cpu_limit", Passed: false},
		},
	}
	plan := &types.ChangePlan{ID: "plan-x", TargetPaths: []string{"src/spin.py"}}
	hint := buildRetryHint(report, plan, 1)
	if !strings.Contains(hint, "CPU-time limit exceeded") {
		t.Errorf("CPU-limit hint must surface the structural tag; got: %s", hint)
	}
	if !strings.Contains(hint, report.FailureSummary) {
		t.Errorf("CPU-limit hint must carry raw FailureSummary; got: %s", hint)
	}
	forbidden := []string{"Infinite loop", "Recursive call without a base case", "DO NOT raise"}
	for _, f := range forbidden {
		if strings.Contains(hint, f) {
			t.Errorf("CPU-limit hint must not contain prescribed-cause prose %q; got: %s", f, hint)
		}
	}
	// Must not bleed OOM tag into a CPU-limit hint.
	if strings.Contains(hint, "out-of-memory") {
		t.Errorf("CPU-limit hint must not include OOM tag; got: %s", hint)
	}
}

// TestBuildRetryHint_TimeoutTagPropagated mirrors the above for
// wall-clock timeouts.
func TestBuildRetryHint_TimeoutTagPropagated(t *testing.T) {
	report := &types.ChangeReport{
		Passed:         false,
		FailureKind:    types.FailureKindTimeout,
		FailureSummary: "[python] command timed out after 5m0s",
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindBuildError, AssertionID: "timeout", Suite: "timeout", Passed: false},
		},
	}
	plan := &types.ChangePlan{ID: "plan-x", TargetPaths: []string{"src/wait.py"}}
	hint := buildRetryHint(report, plan, 1)
	if !strings.Contains(hint, "wall-clock timeout") {
		t.Errorf("timeout hint must surface the structural tag; got: %s", hint)
	}
	if !strings.Contains(hint, report.FailureSummary) {
		t.Errorf("timeout hint must carry raw FailureSummary; got: %s", hint)
	}
	forbidden := []string{"blocking call", "finite timeout", "deadlock", "Revise the plan to:"}
	for _, f := range forbidden {
		if strings.Contains(hint, f) {
			t.Errorf("timeout hint must not contain prescribed-cause prose %q; got: %s", f, hint)
		}
	}
}

// TestBuildRetryHint_OrdinaryFailureKindNoSpecialNarrative verifies
// FailureKindTestsFailed (the default) does NOT trigger a resource-
// exhaustion narrative. The planner should see only the existing
// per-test breakdown for ordinary red tests; padding the hint with
// OOM/CPU/timeout guidance for a normal failure would confuse it.
func TestBuildRetryHint_OrdinaryFailureKindNoSpecialNarrative(t *testing.T) {
	report := &types.ChangeReport{
		Passed:         false,
		FailureKind:    types.FailureKindTestsFailed,
		FailureSummary: "1 of 3 tests failed",
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "TestFoo", Suite: "pkg/foo", Passed: false, FailureDetail: "expected 1, got 2"},
		},
	}
	plan := &types.ChangePlan{ID: "plan-x", TargetPaths: []string{"src/foo.py"}}
	hint := buildRetryHint(report, plan, 1)
	forbidden := []string{
		"out-of-memory",
		"CPU-time limit exceeded",
		"wall-clock timeout",
	}
	for _, f := range forbidden {
		if strings.Contains(hint, f) {
			t.Errorf("ordinary tests_failed hint should not include %q\nhint=%s", f, hint)
		}
	}
}
