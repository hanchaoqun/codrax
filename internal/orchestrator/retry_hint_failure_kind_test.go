package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBuildRetryHint_OOMNarrative verifies the OOM-classified
// FailureKind produces a planner-readable narrative that names the
// failure mode + suggests corrective directions, not the generic
// "tests failed" string. Without this surfacing the retry loop
// repeats the OOM forever (the planner re-derives the same wrong
// fix from the same buried stderr).
//
// Provenance: 2026-04-26 OOM event — the supervisor caps now
// catch the kill in seconds; this test pins the planner-facing
// half of the closed-loop fix.
func TestBuildRetryHint_OOMNarrative(t *testing.T) {
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

	musts := []string{
		"out-of-memory",          // failure-mode header
		"Unbounded allocation",   // corrective direction
		"DO NOT raise the memory cap", // anti-anti-pattern
	}
	for _, m := range musts {
		if !strings.Contains(hint, m) {
			t.Errorf("OOM hint missing required text %q\nhint=%s", m, hint)
		}
	}
}

// TestBuildRetryHint_CPULimitNarrative pins the CPU-limit narrative
// is distinct from OOM and timeout — the corrective direction
// (audit loops, audit recursion bases) is different.
func TestBuildRetryHint_CPULimitNarrative(t *testing.T) {
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
	musts := []string{
		"CPU-time limit exceeded",
		"Infinite loop",
		"Recursive call without a base case",
	}
	for _, m := range musts {
		if !strings.Contains(hint, m) {
			t.Errorf("CPU-limit hint missing %q\nhint=%s", m, hint)
		}
	}
	// Must NOT confuse the planner by including the OOM narrative.
	if strings.Contains(hint, "out-of-memory") {
		t.Errorf("CPU-limit hint must not include OOM section\nhint=%s", hint)
	}
}

// TestBuildRetryHint_TimeoutNarrative pins the timeout narrative.
// The OOM event was an OOM, not a timeout, but timeout is the third
// resource-exhaustion kind and deserves its own corrective direction
// (audit blocking calls, not allocations).
func TestBuildRetryHint_TimeoutNarrative(t *testing.T) {
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
	musts := []string{
		"wall-clock timeout",
		"blocking call",
		"finite timeout",
	}
	for _, m := range musts {
		if !strings.Contains(hint, m) {
			t.Errorf("timeout hint missing %q\nhint=%s", m, hint)
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
