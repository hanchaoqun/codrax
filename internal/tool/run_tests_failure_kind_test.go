package tool

import (
	"path/filepath"
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

func TestQualifyChangeReport_QualifiesBuildErrorPathsFromRunnerRoot(t *testing.T) {
	repo := t.TempDir()
	report := &types.ChangeReport{
		Passed:      false,
		BuildFailed: true,
		TestResults: []types.TestResult{{
			Kind: types.TestResultKindBuildError,
			BuildErrors: []types.BuildError{{
				File:    "src/Foo.java",
				Line:    12,
				Message: "cannot find symbol",
			}},
		}},
	}
	out := qualifyChangeReport(report, runnerPlan{Runner: "java", Root: filepath.Join(repo, "module"), Manifest: "pom.xml"}, repo)
	if got := out.TestResults[0].BuildErrors[0].File; got != "module/src/Foo.java" {
		t.Fatalf("BuildError file = %q, want module/src/Foo.java", got)
	}
}

func TestScopeBuildFailureReportToChangedLinesDowngradesOutsidePatchLine(t *testing.T) {
	ctx := buildErrorScopeContextForTest(t, "src/app.ts")
	report := buildFailureReportForScopeTest("src/app.ts", 2)

	got := scopeBuildFailureReportToChangedLines(ctx, report)

	if got.FailureKind != types.FailureKindPreexistingBuildFailure {
		t.Fatalf("FailureKind = %q, want %q", got.FailureKind, types.FailureKindPreexistingBuildFailure)
	}
	if got.BuildFailed {
		t.Fatal("outside-line build failure should not remain BuildFailed")
	}
	if got.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
		t.Fatalf("VerificationStatus = %q, want unavailable", got.NormalizeVerificationStatus())
	}
	if got.FailureReasonCode != preexistingBuildFailureReasonCode {
		t.Fatalf("FailureReasonCode = %q, want %q", got.FailureReasonCode, preexistingBuildFailureReasonCode)
	}
	if len(got.TestResults) != 1 || len(got.TestResults[0].BuildErrors) != 1 {
		t.Fatalf("BuildErrors should remain as P2 handoff evidence: %+v", got.TestResults)
	}
}

func TestScopeBuildFailureReportToChangedLinesKeepsChangedLineFailure(t *testing.T) {
	ctx := buildErrorScopeContextForTest(t, "src/app.ts")
	report := buildFailureReportForScopeTest("src/app.ts", 10)

	got := scopeBuildFailureReportToChangedLines(ctx, report)

	if got.FailureKind != types.FailureKindBuildFailure {
		t.Fatalf("changed-line build error should remain build_failure, got %q", got.FailureKind)
	}
	if !got.BuildFailed {
		t.Fatal("changed-line build error should remain BuildFailed")
	}
	if got.NormalizeVerificationStatus() != types.VerificationStatusFailed {
		t.Fatalf("VerificationStatus = %q, want failed", got.NormalizeVerificationStatus())
	}
}

func TestScopeBuildFailureReportToChangedLinesFailsClosedWithoutLine(t *testing.T) {
	ctx := buildErrorScopeContextForTest(t, "src/app.ts")
	report := buildFailureReportForScopeTest("src/app.ts", 0)

	got := scopeBuildFailureReportToChangedLines(ctx, report)

	if got.FailureKind != types.FailureKindBuildFailure {
		t.Fatalf("line-less build error should remain build_failure, got %q", got.FailureKind)
	}
	if !got.BuildFailed {
		t.Fatal("line-less build error should remain BuildFailed")
	}
}

func buildErrorScopeContextForTest(t *testing.T, path string) *types.BusContext {
	t.Helper()
	mu := types.NewMutableState("build error changed-line scope")
	mu.SetChangePlan(&types.ChangePlan{
		ID: "plan-build-scope",
		Changes: []types.FileChange{{
			Path:  path,
			Kind:  "patch",
			Patch: "@@ -10,1 +10,1 @@\n-old\n+new\n",
		}},
	})
	return &types.BusContext{
		RepoRoot: t.TempDir(),
		Mutable:  mu,
	}
}

func buildFailureReportForScopeTest(path string, line int) *types.ChangeReport {
	return &types.ChangeReport{
		Passed:      false,
		BuildFailed: true,
		FailureKind: types.FailureKindBuildFailure,
		TestResults: []types.TestResult{{
			Kind:        types.TestResultKindBuildError,
			AssertionID: "build",
			Suite:       "build",
			Passed:      false,
			BuildErrors: []types.BuildError{{
				File:    path,
				Line:    line,
				Column:  3,
				Symbol:  "TS2304",
				Message: "cannot find name",
			}},
		}},
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

func TestMergeChangeReports_PrimaryReasonFollowsPrimaryFailureKind(t *testing.T) {
	build := &types.ChangeReport{
		Passed:            false,
		BuildFailed:       true,
		FailureKind:       types.FailureKindBuildFailure,
		FailureReasonCode: "typescript_compile_check_failed",
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindBuildError, AssertionID: "tsc", Suite: "typescript", Passed: false},
		},
	}
	tests := &types.ChangeReport{
		Passed:            false,
		FailureKind:       types.FailureKindTestsFailed,
		FailureReasonCode: "verification_probe_exception",
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "TestA", Suite: "pkg/a", Passed: false},
		},
	}
	merged := mergeChangeReports([]*types.ChangeReport{tests, build})
	if merged.FailureKind != types.FailureKindBuildFailure {
		t.Fatalf("FailureKind = %q, want %q", merged.FailureKind, types.FailureKindBuildFailure)
	}
	if merged.FailureReasonCode != "typescript_compile_check_failed" {
		t.Fatalf("FailureReasonCode = %q, want primary build reason only", merged.FailureReasonCode)
	}
}

func TestMergeChangeReports_BackfillsUnavailableProbeReasonForUnreasonedBuildFailure(t *testing.T) {
	build := &types.ChangeReport{
		Passed:      false,
		BuildFailed: true,
		FailureKind: types.FailureKindBuildFailure,
		TestResults: []types.TestResult{{
			Kind:        types.TestResultKindBuildError,
			AssertionID: "build",
			Suite:       "build",
			Passed:      false,
		}},
	}
	probe := &types.ChangeReport{
		Passed:            false,
		FailureKind:       types.FailureKindParserError,
		FailureReasonCode: "verification_probe_module_not_found",
		TestResults: []types.TestResult{{
			Kind:        types.TestResultKindBuildError,
			AssertionID: "verification_probe",
			Suite:       "verification_probe",
			Passed:      false,
		}},
	}
	merged := mergeChangeReports([]*types.ChangeReport{build, probe})
	if merged.FailureKind != types.FailureKindBuildFailure {
		t.Fatalf("FailureKind = %q, want %q", merged.FailureKind, types.FailureKindBuildFailure)
	}
	if merged.FailureReasonCode != "verification_probe_module_not_found" {
		t.Fatalf("FailureReasonCode = %q, want verification_probe_module_not_found", merged.FailureReasonCode)
	}
	if got := merged.NormalizeVerificationStatus(); got != types.VerificationStatusUnavailable {
		t.Fatalf("VerificationStatus = %q, want unavailable", got)
	}
}

func TestMergeChangeReports_DoesNotBackfillUnavailableReasonWhenMixedWithCodeFailure(t *testing.T) {
	build := &types.ChangeReport{
		Passed:      false,
		BuildFailed: true,
		FailureKind: types.FailureKindBuildFailure,
		TestResults: []types.TestResult{{
			Kind:        types.TestResultKindBuildError,
			AssertionID: "build",
			Suite:       "build",
			Passed:      false,
		}},
	}
	probe := &types.ChangeReport{
		Passed:            false,
		FailureKind:       types.FailureKindParserError,
		FailureReasonCode: "verification_probe_module_not_found",
	}
	runtime := &types.ChangeReport{
		Passed:            false,
		FailureKind:       types.FailureKindTestsFailed,
		FailureReasonCode: "verification_probe_exception",
	}
	merged := mergeChangeReports([]*types.ChangeReport{build, probe, runtime})
	if merged.FailureKind != types.FailureKindBuildFailure {
		t.Fatalf("FailureKind = %q, want %q", merged.FailureKind, types.FailureKindBuildFailure)
	}
	if merged.FailureReasonCode != "" {
		t.Fatalf("FailureReasonCode = %q, want no fallback when unavailable and code-failure reasons are mixed", merged.FailureReasonCode)
	}
	if got := merged.NormalizeVerificationStatus(); got != types.VerificationStatusFailed {
		t.Fatalf("VerificationStatus = %q, want failed", got)
	}
}

func TestMergeChangeReports_DoesNotPromoteUnavailableReasonOverRedTests(t *testing.T) {
	tests := &types.ChangeReport{
		Passed:         false,
		FailureKind:    types.FailureKindTestsFailed,
		FailureSummary: "python unittest failed",
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "test_widget", Suite: "python", Passed: false},
		},
	}
	makeUnavailable := &types.ChangeReport{
		Passed:            false,
		FailureKind:       types.FailureKindParserError,
		FailureReasonCode: "make_target_missing",
		FailureSummary:    "make target unavailable: No rule to make target `check'",
		NoTestsRunners:    []string{"make"},
		TestResults: []types.TestResult{{
			Kind:        types.TestResultKindBuildError,
			AssertionID: "make_target_missing",
			Suite:       "make",
			Passed:      false,
		}},
	}
	merged := mergeChangeReports([]*types.ChangeReport{tests, makeUnavailable})
	if merged.FailureKind != types.FailureKindTestsFailed {
		t.Fatalf("FailureKind = %q, want %q", merged.FailureKind, types.FailureKindTestsFailed)
	}
	if merged.FailureReasonCode != "" {
		t.Fatalf("FailureReasonCode = %q, want no secondary unavailable reason as primary", merged.FailureReasonCode)
	}
	if len(merged.NoTestsRunners) != 1 || merged.NoTestsRunners[0] != "make" {
		t.Fatalf("secondary unavailable evidence should remain in NoTestsRunners, got %+v", merged.NoTestsRunners)
	}
	if !strings.Contains(merged.FailureSummary, "make target unavailable") {
		t.Fatalf("secondary unavailable evidence should remain in summary, got %q", merged.FailureSummary)
	}
}

func TestMergeChangeReports_CommandBackfillKeepsUnavailableReasonOutOfRedTests(t *testing.T) {
	report := &types.ChangeReport{
		Passed:         false,
		FailureKind:    types.FailureKindTestsFailed,
		FailureSummary: "python unittest failed | make target unavailable",
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "test_widget", Suite: "python", Passed: false},
			{Kind: types.TestResultKindBuildError, AssertionID: "make_target_missing", Suite: "make", Passed: false},
		},
		ExecutedCommands: []types.ExecutedCommand{
			{Runner: "python", Outcome: "executed"},
			{Runner: "make", Outcome: "parser_error", ReasonCode: "make_target_missing"},
		},
		NoTestsRunners: []string{"make"},
	}
	merged := mergeChangeReports([]*types.ChangeReport{report})
	if merged.FailureKind != types.FailureKindTestsFailed {
		t.Fatalf("FailureKind = %q, want %q", merged.FailureKind, types.FailureKindTestsFailed)
	}
	if merged.FailureReasonCode != "" {
		t.Fatalf("FailureReasonCode = %q, want secondary unavailable command reason excluded", merged.FailureReasonCode)
	}
	if len(merged.ExecutedCommands) != 0 {
		t.Fatalf("mergeChangeReports should not copy command rows directly; finishReport owns command evidence")
	}
}

func TestMergeChangeReports_BackfillsFailureReasonFromCommandEvidence(t *testing.T) {
	report := &types.ChangeReport{
		Passed:      false,
		FailureKind: types.FailureKindParserError,
		ExecutedCommands: []types.ExecutedCommand{{
			Runner:     "verification_probe",
			Outcome:    "parser_error",
			ReasonCode: "verification_probe_exception",
		}},
	}
	merged := mergeChangeReports([]*types.ChangeReport{report})
	if merged.FailureReasonCode != "verification_probe_exception" {
		t.Fatalf("FailureReasonCode = %q, want verification_probe_exception", merged.FailureReasonCode)
	}
}

func TestFailureReasonCodeFromExecutedCommandsForKindKeepsProbeRuntimeFailure(t *testing.T) {
	got := failureReasonCodeFromExecutedCommandsForKind([]types.ExecutedCommand{{
		Runner:     "verification_probe",
		Outcome:    "executed",
		ReasonCode: "verification_probe_exception",
	}}, types.FailureKindTestsFailed)
	if got != "verification_probe_exception" {
		t.Fatalf("reason = %q, want verification_probe_exception", got)
	}
}

func TestRenderTestSummary_UnavailableDoesNotRenderAsFailedAssertions(t *testing.T) {
	report := &types.ChangeReport{
		Passed:      false,
		FailureKind: types.FailureKindParserError,
		TestResults: []types.TestResult{
			{
				Kind:          types.TestResultKindUnit,
				AssertionID:   "requests",
				Suite:         "unittest.loader._FailedTest",
				Passed:        false,
				FailureDetail: "ImportError: cannot import name Mapping",
			},
		},
		FailureSummary: "unittest collection/import failed before executing real test cases",
	}
	report.EnsureVerificationStatus()

	summary := renderTestSummary("python", report)
	if !strings.Contains(summary, "verdict=UNAVAILABLE") {
		t.Fatalf("summary should expose typed unavailable verdict, got %q", summary)
	}
	if strings.Contains(summary, "Failed tests:") {
		t.Fatalf("summary should not present infrastructure diagnostics as failed tests: %q", summary)
	}
	if strings.Contains(summary, "1 failed") {
		t.Fatalf("summary should not count parser diagnostics as failed assertions: %q", summary)
	}
}

func TestRenderAggregateTestSummary_UnavailableUsesDiagnosticCounts(t *testing.T) {
	plans := []runnerPlan{
		{Runner: "python", Root: "/repo"},
		{Runner: "python", Framework: pythonFrameworkUnittest, Root: "/repo"},
	}
	reports := []*types.ChangeReport{
		{Passed: true, NoTestsRunners: []string{"python"}},
		{
			Passed:         false,
			FailureKind:    types.FailureKindParserError,
			FailureSummary: "unittest collection/import failed before executing real test cases",
			TestResults: []types.TestResult{{
				Kind:        types.TestResultKindUnit,
				AssertionID: "requests",
				Suite:       "unittest.loader._FailedTest",
				Passed:      false,
			}},
		},
	}
	aggregate := mergeChangeReports(reports)
	summary := renderAggregateTestSummary("/repo", plans, reports, aggregate)
	if !strings.Contains(summary, "verdict=UNAVAILABLE") {
		t.Fatalf("aggregate summary should expose unavailable verdict, got %q", summary)
	}
	if strings.Contains(summary, "assertion(s), 1 failed") {
		t.Fatalf("aggregate summary should not render parser diagnostics as failed assertions: %q", summary)
	}
	if !strings.Contains(summary, "diagnostic row(s)") {
		t.Fatalf("aggregate summary should mention diagnostic rows, got %q", summary)
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
