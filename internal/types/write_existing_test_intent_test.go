package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// These are consumer protocol fixtures, not evidence of a native test run.
// The orchestrator public test separately obtains receipts from real runners.
func existingIntentConsumerFixture(targets ...string) (*ChangePlan, *ChangeReport) {
	if len(targets) == 0 {
		targets = []string{"packages/widget/tests/test_widget.py"}
	}
	plan := &ChangePlan{
		ID: "current-plan", AppliedCommitSHA: "current-apply",
		PatchEffect:     &PatchEffectRecord{PlanID: "current-plan", RecordID: "current-effect", DiffFingerprint: "current-diff"},
		WriteAnalysisIR: &WriteAnalysisIR{},
	}
	report := &ChangeReport{
		PlanID: plan.ID, Channel: ChangeReportChannelPostApplyVerify,
		Passed: true, VerificationStatus: VerificationStatusPassed,
		VerificationConfidence: []VerificationConfidenceRecord{{Source: "other-owner", Category: "unrelated", Status: "observed", Detail: "retain this independent record"}},
		TestSurface:            &TestSurface{Candidates: []TestSurfaceCandidate{{ID: "python/unittest@packages/widget", Runner: "python", Framework: "unittest", WorkingDir: "packages/widget", HasTestSignal: true}}},
	}
	for i, target := range targets {
		plan.WriteAnalysisIR.Request.Constraints = append(plan.WriteAnalysisIR.Request.Constraints, WriteConstraint{Kind: WriteConstraintRunExistingTest, Target: target})
		suite := strings.TrimPrefix(target, "packages/widget/")
		report.ExecutedCommands = append(report.ExecutedCommands, ExecutedCommand{Runner: "python", Framework: "unittest", WorkingDir: "packages/widget", Suite: suite, Command: "display-only command", Outcome: ExecutedCommandOutcomeExecuted})
		report.TestResults = append(report.TestResults, TestResult{Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAssertion, Suite: strings.TrimSuffix(strings.ReplaceAll(suite, "/", "."), ".py") + ".WidgetTest", AssertionID: "test_value", Passed: true})
		report.ExistingTestExecutions = append(report.ExistingTestExecutions, ExistingTestExecutionReceipt{
			PlanID: plan.ID, AppliedCommitSHA: plan.AppliedCommitSHA, PatchEffectID: plan.PatchEffect.RecordID, DiffFingerprint: plan.PatchEffect.DiffFingerprint,
			TestPath: target, CandidateID: report.TestSurface.Candidates[0].ID, Runner: "python", Framework: "unittest", WorkingDir: "packages/widget", Suite: suite,
			CommandIndex: i, AssertionCount: 1,
			TestFileSHA256: ExistingTestExecutionDigest("fixture source bytes"), CommandSHA256: ExistingTestExecutionDigest(report.ExecutedCommands[i].Command),
			AssertionDigests: []string{ExistingTestAssertionDigest(report.TestResults[i])},
		})
	}
	return plan, report
}

func existingIntentConsumerBytes(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertExistingIntentConsumerStatuses(t *testing.T, plan *ChangePlan, report *ChangeReport, want ...string) {
	t.Helper()
	before := existingIntentConsumerBytes(t, []any{plan, report})
	records := ExistingTestExecutionConfidence(plan, report)
	if len(records) != len(want) {
		t.Fatalf("execution confidence count=%d, want=%d: %+v", len(records), len(want), records)
	}
	for i, rec := range records {
		wantSource := WriteConstraintRunExistingTest + ":" + ExistingTestExecutionDigest(RequiredExistingTestPaths(plan)[i])
		if rec.Status != want[i] || rec.Source != wantSource || rec.Category != ExistingTestExecutionCategory {
			t.Errorf("execution confidence[%d]=%+v, want status %s", i, rec, want[i])
		}
		if len(rec.ContractRefs) != 0 || len(rec.ChangedSymbolRefs) != 0 || rec.WitnessKind != "" {
			t.Errorf("file execution became behavior/changed-source authority: %+v", rec)
		}
	}
	if after := existingIntentConsumerBytes(t, []any{plan, report}); after != before {
		t.Fatal("confidence projection mutated its inputs")
	}
}

func TestExistingTestIntentConsumerIdentityMutations(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangePlan, *ChangeReport)
	}{
		{"plan_id", func(p *ChangePlan, _ *ChangeReport) { p.ID = "other-plan" }},
		{"report_plan", func(_ *ChangePlan, r *ChangeReport) { r.PlanID = "other-plan" }},
		{"report_channel", func(_ *ChangePlan, r *ChangeReport) { r.Channel = "" }},
		{"missing_patch", func(p *ChangePlan, _ *ChangeReport) { p.PatchEffect = nil }},
		{"patch_owner", func(p *ChangePlan, _ *ChangeReport) { p.PatchEffect.PlanID = "other-plan" }},
		{"apply_commit", func(p *ChangePlan, _ *ChangeReport) { p.AppliedCommitSHA = "next-apply" }},
		{"patch_record", func(p *ChangePlan, _ *ChangeReport) { p.PatchEffect.RecordID = "next-effect" }},
		{"diff_fingerprint", func(p *ChangePlan, _ *ChangeReport) { p.PatchEffect.DiffFingerprint = "next-diff" }},
		{"receipt_plan", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].PlanID = "other-plan" }},
		{"receipt_apply", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].AppliedCommitSHA = "" }},
		{"receipt_patch", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].PatchEffectID = "" }},
		{"receipt_diff", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].DiffFingerprint = "" }},
		{"receipt_file_hash_absent", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].TestFileSHA256 = "" }},
		{"receipt_file_hash_not_hex", func(_ *ChangePlan, r *ChangeReport) {
			r.ExistingTestExecutions[0].TestFileSHA256 = strings.Repeat("z", 64)
		}},
		{"receipt_command_digest", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].CommandSHA256 = "" }},
		{"receipt_assertions_absent", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].AssertionDigests = nil }},
		{"receipt_assertion_digest", func(_ *ChangePlan, r *ChangeReport) {
			r.ExistingTestExecutions[0].AssertionDigests[0] = ExistingTestExecutionDigest("unrelated result")
		}},
		{"receipt_target", func(_ *ChangePlan, r *ChangeReport) {
			r.ExistingTestExecutions[0].TestPath = "packages/widget/tests/test_other.py"
		}},
		{"receipt_candidate", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].CandidateID = "other-candidate" }},
		{"receipt_runner", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].Runner = "ruby" }},
		{"receipt_framework", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].Framework = "pytest" }},
		{"receipt_cwd", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].WorkingDir = "." }},
		{"receipt_selector", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].Suite = "tests" }},
		{"command_negative", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].CommandIndex = -1 }},
		{"command_absent", func(_ *ChangePlan, r *ChangeReport) { r.ExecutedCommands = nil }},
		{"command_runner", func(_ *ChangePlan, r *ChangeReport) { r.ExecutedCommands[0].Runner = "node" }},
		{"command_framework", func(_ *ChangePlan, r *ChangeReport) { r.ExecutedCommands[0].Framework = "pytest" }},
		{"command_cwd", func(_ *ChangePlan, r *ChangeReport) { r.ExecutedCommands[0].WorkingDir = "other" }},
		{"command_selector", func(_ *ChangePlan, r *ChangeReport) { r.ExecutedCommands[0].Suite = "tests/test_other.py" }},
		{"command_skip", func(_ *ChangePlan, r *ChangeReport) {
			r.ExecutedCommands[0].Outcome = ExecutedCommandOutcomeSuiteSkipped
		}},
		{"command_zero", func(_ *ChangePlan, r *ChangeReport) { r.ExecutedCommands[0].Outcome = ExecutedCommandOutcomeZeroTests }},
		{"command_bytes", func(_ *ChangePlan, r *ChangeReport) { r.ExecutedCommands[0].Command += " changed" }},
		{"command_failed_without_failed_assertion", func(_ *ChangePlan, r *ChangeReport) { r.ExecutedCommands[0].ExitCode = 1 }},
		{"probe_not_native", func(_ *ChangePlan, r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution = &VerificationProbeExecutionReceipt{}
		}},
		{"source_check_not_native", func(_ *ChangePlan, r *ChangeReport) {
			r.ExecutedCommands[0].SourceCheckExecution = &SourceCheckExecutionReceipt{}
		}},
		{"candidate_absent", func(_ *ChangePlan, r *ChangeReport) { r.TestSurface = nil }},
		{"candidate_no_tests", func(_ *ChangePlan, r *ChangeReport) { r.TestSurface.Candidates[0].HasTestSignal = false }},
		{"candidate_runner", func(_ *ChangePlan, r *ChangeReport) { r.TestSurface.Candidates[0].Runner = "node" }},
		{"candidate_framework", func(_ *ChangePlan, r *ChangeReport) { r.TestSurface.Candidates[0].Framework = "pytest" }},
		{"candidate_cwd", func(_ *ChangePlan, r *ChangeReport) { r.TestSurface.Candidates[0].WorkingDir = "other" }},
		{"candidate_ambiguous", func(_ *ChangePlan, r *ChangeReport) {
			r.TestSurface.Candidates = append(r.TestSurface.Candidates, r.TestSurface.Candidates[0])
		}},
		{"all_skipped", func(_ *ChangePlan, r *ChangeReport) {
			r.TestResults[0].ObservationScope = TestObservationScopeNonAsserting
		}},
		{"results_absent", func(_ *ChangePlan, r *ChangeReport) { r.TestResults = nil }},
		{"result_identity", func(_ *ChangePlan, r *ChangeReport) { r.TestResults[0].AssertionID = "another_test" }},
		{"result_file", func(_ *ChangePlan, r *ChangeReport) { r.TestResults[0].Suite = "other.Test" }},
		{"result_verdict", func(_ *ChangePlan, r *ChangeReport) { r.TestResults[0].Passed = false }},
		{"reused_assertion_digest", func(_ *ChangePlan, r *ChangeReport) {
			r.ExistingTestExecutions[0].AssertionCount = 2
			r.ExistingTestExecutions[0].AssertionDigests = append(r.ExistingTestExecutions[0].AssertionDigests, r.ExistingTestExecutions[0].AssertionDigests[0])
		}},
		{"assertion_count_cap", func(_ *ChangePlan, r *ChangeReport) {
			row, digest := r.TestResults[0], r.ExistingTestExecutions[0].AssertionDigests[0]
			for len(r.TestResults) <= MaxExistingTestExecutionAssertions {
				r.TestResults = append(r.TestResults, row)
				r.ExistingTestExecutions[0].AssertionDigests = append(r.ExistingTestExecutions[0].AssertionDigests, digest)
			}
			r.ExistingTestExecutions[0].AssertionCount = len(r.TestResults)
		}},
		{"zero_assertions", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].AssertionCount = 0 }},
		{"negative_failures", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].FailedAssertionCount = -1 }},
		{"impossible_failures", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].FailedAssertionCount = 2 }},
		{"too_many_receipts", func(_ *ChangePlan, r *ChangeReport) {
			receipt := r.ExistingTestExecutions[0]
			for len(r.ExistingTestExecutions) <= MaxExistingTestExecutionReceipts {
				r.ExistingTestExecutions = append(r.ExistingTestExecutions, receipt)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, report := existingIntentConsumerFixture()
			assertExistingIntentConsumerStatuses(t, plan, report, "satisfied")
			tc.edit(plan, report)
			assertExistingIntentConsumerStatuses(t, plan, report, "missing")
			before := existingIntentConsumerBytes(t, []any{plan, report})
			got := EffectiveExistingTestExecutionReport(plan, report)
			if got.Passed || got.VerificationStatus != VerificationStatusUnavailable || got.FailureKind != FailureKindVerificationIncomplete {
				t.Errorf("unbound execution must not keep a green report: %+v", got)
			}
			if existingIntentConsumerBytes(t, []any{plan, report}) != before {
				t.Fatal("effective report rewrote history")
			}
		})
	}
}

func TestExistingTestIntentConsumerJSONAndIndependentTargets(t *testing.T) {
	plan, report := existingIntentConsumerFixture("packages/widget/tests/test_a.py", "packages/widget/tests/test_b.py")
	var loadedPlan ChangePlan
	var loadedReport ChangeReport
	if err := json.Unmarshal([]byte(existingIntentConsumerBytes(t, plan)), &loadedPlan); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(existingIntentConsumerBytes(t, report)), &loadedReport); err != nil {
		t.Fatal(err)
	}
	assertExistingIntentConsumerStatuses(t, &loadedPlan, &loadedReport, "satisfied", "satisfied")
	loadedReport.ExistingTestExecutions = loadedReport.ExistingTestExecutions[:1]
	assertExistingIntentConsumerStatuses(t, &loadedPlan, &loadedReport, "satisfied", "missing")
	before := existingIntentConsumerBytes(t, []any{loadedPlan, loadedReport})
	projected := EffectiveVerificationProbeReport(&loadedPlan, &loadedReport)
	if projected.Passed || projected.VerificationStatus != VerificationStatusUnavailable {
		t.Fatal("first file lent its receipt to the second requirement")
	}
	if !reflect.DeepEqual(projected.TestResults, loadedReport.TestResults) || !reflect.DeepEqual(projected.ExecutedCommands, loadedReport.ExecutedCommands) {
		t.Fatal("projection dropped independent native result history")
	}
	if !reflect.DeepEqual(projected.VerificationConfidence[0], loadedReport.VerificationConfidence[0]) {
		t.Fatal("projection changed another confidence owner's record")
	}
	if projected.VerificationConfidence[1].Source == projected.VerificationConfidence[2].Source {
		t.Fatal("distinct file requirements lost their independent stable identity")
	}
	if existingIntentConsumerBytes(t, []any{loadedPlan, loadedReport}) != before {
		t.Fatal("combined effective projection changed JSON-loaded history")
	}
}

func TestExistingTestIntentConsumerRequiredPathsPreserveDebt(t *testing.T) {
	plan, report := existingIntentConsumerFixture()
	plan.WriteAnalysisIR.Request.Constraints = append(plan.WriteAnalysisIR.Request.Constraints,
		plan.WriteAnalysisIR.Request.Constraints[0],
		WriteConstraint{Kind: "preserve_regression_test", Target: "other_test.py", Note: "run existing test"})
	if got := RequiredExistingTestPaths(plan); !reflect.DeepEqual(got, []string{"packages/widget/tests/test_widget.py"}) {
		t.Fatalf("duplicate/preservation prose changed exact execution requirements: %v", got)
	}
	plan.WriteAnalysisIR.Request.Constraints = nil
	for i := 0; i <= MaxRequiredExistingTests; i++ {
		plan.WriteAnalysisIR.Request.Constraints = append(plan.WriteAnalysisIR.Request.Constraints, WriteConstraint{Kind: WriteConstraintRunExistingTest, Target: fmt.Sprintf("tests/test_%02d.py", i)})
	}
	paths := RequiredExistingTestPaths(plan)
	if len(paths) != MaxRequiredExistingTests+1 || paths[0] != "" {
		t.Fatalf("over-limit persisted requirement disappeared instead of remaining debt: %v", paths)
	}
	if got := EffectiveExistingTestExecutionReport(plan, report); got.Passed {
		t.Fatal("over-limit requirement was silently closed")
	}
	if RequiredExistingTestPaths(nil) != nil || ExistingTestExecutionConfidence(nil, report) != nil || EffectiveExistingTestExecutionReport(plan, nil) != nil {
		t.Fatal("missing plan/report acquired execution authority")
	}
}

func TestExistingTestIntentConsumerFailureAndLegacyProjection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   FailureKind
		status VerificationStatus
		native bool
	}{
		{"native_failure", FailureKindTestsFailed, VerificationStatusFailed, true},
		{"timeout", FailureKindTimeout, VerificationStatusFailed, false},
		{"missing_runner", FailureKindRunnerMissing, VerificationStatusUnavailable, false},
		{"parser_error", FailureKindParserError, VerificationStatusUnavailable, false},
		{"timeout_with_native_failure", FailureKindTimeout, VerificationStatusFailed, true},
		{"parser_error_with_native_failure", FailureKindParserError, VerificationStatusUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, report := existingIntentConsumerFixture()
			report.Passed, report.VerificationStatus, report.FailureKind = false, tc.status, tc.kind
			report.FailureReasonCode, report.FailureSummary, report.FailureSummaryBlobRef = "original_reason", "original precise fault", "original-blob"
			report.TestResults[0].Passed, report.TestResults[0].FailureDetail = false, "original assertion traceback"
			if tc.native {
				report.ExistingTestExecutions[0].FailedAssertionCount = 1
				report.ExistingTestExecutions[0].AssertionDigests = []string{ExistingTestAssertionDigest(report.TestResults[0])}
				report.ExecutedCommands[0].ExitCode = 1
				assertExistingIntentConsumerStatuses(t, plan, report, "failed")
			} else {
				report.ExistingTestExecutions = nil
				assertExistingIntentConsumerStatuses(t, plan, report, "missing")
			}
			before := existingIntentConsumerBytes(t, []any{plan, report})
			got := EffectiveVerificationProbeReport(plan, report)
			if got.Passed || got.VerificationStatus != tc.status || got.FailureKind != tc.kind || got.FailureReasonCode != report.FailureReasonCode || got.FailureSummary != report.FailureSummary || got.FailureSummaryBlobRef != report.FailureSummaryBlobRef {
				t.Fatalf("execution debt replaced the original stronger failure: %+v", got)
			}
			if !reflect.DeepEqual(got.TestResults, report.TestResults) || !reflect.DeepEqual(got.ExecutedCommands, report.ExecutedCommands) {
				t.Fatal("failure receipt lost")
			}
			if existingIntentConsumerBytes(t, []any{plan, report}) != before {
				t.Fatal("projection mutated persisted failure")
			}
		})
	}
	t.Run("native_failure_overrules_old_green_label", func(t *testing.T) {
		plan, report := existingIntentConsumerFixture()
		report.TestResults[0].Passed = false
		report.TestResults[0].FailureDetail = "native assertion failed"
		report.ExecutedCommands[0].ExitCode = 1
		report.ExistingTestExecutions[0].FailedAssertionCount = 1
		report.ExistingTestExecutions[0].AssertionDigests = []string{ExistingTestAssertionDigest(report.TestResults[0])}
		before := existingIntentConsumerBytes(t, report)
		got := EffectiveExistingTestExecutionReport(plan, report)
		if got.Passed || got.VerificationStatus != VerificationStatusFailed || got.FailureKind != FailureKindTestsFailed || got.FailureReasonCode != "required_existing_test_failed" {
			t.Fatalf("precise native failure degraded into generic missing execution: %+v", got)
		}
		if !reflect.DeepEqual(got.TestResults, report.TestResults) || existingIntentConsumerBytes(t, report) != before {
			t.Fatal("restoring a failed verdict rewrote the native history")
		}
	})
	t.Run("old_satisfied_without_receipt", func(t *testing.T) {
		plan, report := existingIntentConsumerFixture()
		report.ExistingTestExecutions = nil
		report.VerificationConfidence = append(report.VerificationConfidence, VerificationConfidenceRecord{Category: ExistingTestExecutionCategory, Status: "satisfied", ContractRefs: []string{"must-not-authorize"}})
		before := existingIntentConsumerBytes(t, report)
		got := EffectiveExistingTestExecutionReport(plan, report)
		if got.Passed {
			t.Fatal("persisted satisfied label replaced a missing native receipt")
		}
		for _, rec := range got.VerificationConfidence {
			if rec.Category == ExistingTestExecutionCategory && (rec.Status != "missing" || len(rec.ContractRefs) != 0) {
				t.Fatalf("stale authority survived: %+v", rec)
			}
		}
		if existingIntentConsumerBytes(t, report) != before {
			t.Fatal("old report was migrated in place")
		}
	})
	for _, intent := range []string{"", "preserve_regression_test"} {
		t.Run("legacy_"+intent, func(t *testing.T) {
			plan, report := existingIntentConsumerFixture()
			plan.WriteAnalysisIR.Request.Constraints[0].Kind = intent
			report.ExistingTestExecutions = nil
			before := existingIntentConsumerBytes(t, report)
			if got := EffectiveExistingTestExecutionReport(plan, report); got != report || existingIntentConsumerBytes(t, got) != before {
				t.Fatal("legacy/preservation intent gained an execution obligation")
			}
			assertExistingIntentConsumerStatuses(t, plan, report)
		})
	}
}

func TestExistingTestIntentConsumerSelectorBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, runner, framework, workingDir, suite, target string
		want                                               bool
	}{
		{"nested", "python", "unittest", "packages/widget", "tests/test_a.py", "packages/widget/tests/test_a.py", true},
		{"root", "python", "unittest", ".", "test_a.py", "test_a.py", true},
		{"other_file", "python", "unittest", "packages/widget", "tests/test_b.py", "packages/widget/tests/test_a.py", false},
		{"directory", "python", "unittest", "packages/widget", "tests", "packages/widget/tests/test_a.py", false},
		{"module_not_file", "python", "unittest", ".", "tests.test_a", "tests/test_a.py", false},
		{"glob", "python", "unittest", ".", "test_*.py", "test_*.py", false},
		{"parent_path", "python", "unittest", "packages", "../test_a.py", "test_a.py", false},
		{"noncanonical_cwd", "python", "unittest", "packages/../packages", "test_a.py", "packages/test_a.py", false},
		{"absolute", "python", "unittest", "/repo", "test_a.py", "/repo/test_a.py", false},
		{"empty_cwd", "python", "unittest", "", "test_a.py", "test_a.py", false},
		{"windows_uncanonical", "python", "unittest", ".", `tests\test_a.py`, `tests\test_a.py`, false},
		{"newline", "python", "unittest", ".", "test_a\n.py", "test_a\n.py", false},
		{"pytest_not_yet_bound", "python", "pytest", ".", "test_a.py", "test_a.py", false},
		{"node_not_yet_bound", "node", "jest", ".", "test_a.js", "test_a.js", false},
		{"ruby_not_yet_bound", "ruby", "rspec", ".", "test_a.rb", "test_a.rb", false},
		{"go_package_not_file", "go", "", ".", "test_a.go", "test_a.go", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExistingTestExactFileSelector(tc.runner, tc.framework, tc.workingDir, tc.suite, tc.target); got != tc.want {
				t.Errorf("exact native selector=%t, want=%t", got, tc.want)
			}
		})
	}
}
