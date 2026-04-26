package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestQualifyChangeReport_BackfillsTestsFailedKind verifies the
// parser-output path picks up FailureKind=tests_failed when none was
// classified. Without this backfill the verify→plan retry hint would
// see an empty FailureKind on every red-test run and never route the
// kind-specific narrative.
func TestQualifyChangeReport_BackfillsTestsFailedKind(t *testing.T) {
	report := &types.ChangeReport{
		Passed: false,
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "TestFoo", Suite: "pkg/foo", Passed: false},
		},
	}
	out := qualifyChangeReport(report, runnerPlan{Runner: "go", Root: "/tmp/repo", Manifest: "go.mod"}, "/tmp/repo")
	if out.FailureKind != types.FailureKindTestsFailed {
		t.Errorf("expected FailureKindTestsFailed, got %q", out.FailureKind)
	}
}

// TestQualifyChangeReport_BackfillsBuildFailureKind verifies build-
// stage failures get FailureKindBuildFailure (not the generic
// tests_failed) so the planner sees "your code didn't compile" rather
// than "your tests are red".
func TestQualifyChangeReport_BackfillsBuildFailureKind(t *testing.T) {
	report := &types.ChangeReport{
		Passed:      false,
		BuildFailed: true,
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindBuildError, AssertionID: "build", Suite: "build", Passed: false},
		},
	}
	out := qualifyChangeReport(report, runnerPlan{Runner: "go", Root: "/tmp/repo", Manifest: "go.mod"}, "/tmp/repo")
	if out.FailureKind != types.FailureKindBuildFailure {
		t.Errorf("expected FailureKindBuildFailure, got %q", out.FailureKind)
	}
}

// TestQualifyChangeReport_PreservesExplicitFailureKind verifies a
// resource-exhaustion FailureKind set by makeResourceExhaustionReport
// survives qualifyChangeReport (which does NOT overwrite when the
// kind is already non-empty).
func TestQualifyChangeReport_PreservesExplicitFailureKind(t *testing.T) {
	report := makeResourceExhaustionReport("oom", "killed by memory cap")
	out := qualifyChangeReport(report, runnerPlan{Runner: "python", Root: "/tmp/repo", Manifest: "pyproject.toml"}, "/tmp/repo")
	if out.FailureKind != types.FailureKindOOM {
		t.Errorf("expected explicit FailureKindOOM to survive qualify, got %q", out.FailureKind)
	}
}

// TestQualifyChangeReport_PassedReportClearsKind verifies a passing
// run never gets a FailureKind backfilled. Without this clamp, a
// retry that turned green would still carry stale "tests_failed"
// state through the merge.
func TestQualifyChangeReport_PassedReportClearsKind(t *testing.T) {
	report := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "TestFoo", Suite: "pkg/foo", Passed: true},
		},
	}
	out := qualifyChangeReport(report, runnerPlan{Runner: "go", Root: "/tmp/repo", Manifest: "go.mod"}, "/tmp/repo")
	if out.FailureKind != "" {
		t.Errorf("passing report should not carry FailureKind, got %q", out.FailureKind)
	}
}

// TestMergeChangeReports_PromotesMostSevereKind verifies the merge
// rule: when one project produced an OOM and another only produced
// red tests, the aggregate FailureKind must surface OOM (the
// architecturally-actionable signal) — not the chronologically-first
// or alphabetically-first kind. Without this the planner would see
// "tests_failed" and re-derive the wrong corrective direction.
func TestMergeChangeReports_PromotesMostSevereKind(t *testing.T) {
	tests := &types.ChangeReport{
		Passed:      false,
		FailureKind: types.FailureKindTestsFailed,
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "TestA", Suite: "pkg/a", Passed: false},
		},
	}
	oom := makeResourceExhaustionReport("oom", "killed by memory cap")
	merged := mergeChangeReports([]*types.ChangeReport{tests, oom})
	if merged.FailureKind != types.FailureKindOOM {
		t.Errorf("expected aggregate FailureKindOOM (most severe), got %q", merged.FailureKind)
	}
}

// TestMakeResourceExhaustionReport_KindsExposed pins the
// kind→FailureKind mapping so callers (run_tests Execute) can rely on
// the surface contract. Catches a regression where a new kind ever
// gets added without updating the switch.
func TestMakeResourceExhaustionReport_KindsExposed(t *testing.T) {
	cases := []struct {
		kind     string
		want     types.FailureKind
		mustHave string
	}{
		{"timeout", types.FailureKindTimeout, "timeout"},
		{"oom", types.FailureKindOOM, "memory"},
		{"cpu_limit", types.FailureKindCPULimit, "CPU"},
		{"unknown", types.FailureKindCrash, "crash"},
	}
	for _, tc := range cases {
		report := makeResourceExhaustionReport(tc.kind, "explanatory text mentioning "+tc.mustHave)
		if report.FailureKind != tc.want {
			t.Errorf("kind=%q: expected FailureKind=%q, got %q", tc.kind, tc.want, report.FailureKind)
		}
		if !strings.Contains(report.FailureSummary, tc.mustHave) {
			t.Errorf("kind=%q: FailureSummary must echo the detail: %q", tc.kind, report.FailureSummary)
		}
	}
}
