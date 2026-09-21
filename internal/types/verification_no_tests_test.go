package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNoTestsReportAuthorityMatrix(t *testing.T) {
	native := TestResult{Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAssertion, AssertionID: "test_value", Suite: "tests.Widget", Passed: true}
	for _, tc := range []struct {
		name string
		edit func(*ChangeReport)
		want VerificationStatus
	}{
		{"legacy_empty_pass", func(r *ChangeReport) {}, VerificationStatusUnavailable},
		{"legacy_empty_false", func(r *ChangeReport) { r.Passed = false }, VerificationStatusUnavailable},
		{"legacy_empty_tests_failed", func(r *ChangeReport) { r.Passed = false; r.FailureKind = FailureKindTestsFailed }, VerificationStatusUnavailable},
		{"native_assertion", func(r *ChangeReport) { r.TestResults = []TestResult{native} }, VerificationStatusPassed},
		{"native_default_unit_kind", func(r *ChangeReport) { row := native; row.Kind = ""; r.TestResults = []TestResult{row} }, VerificationStatusPassed},
		{"probe", func(r *ChangeReport) {
			r.TestResults = []TestResult{{Kind: TestResultKindUnit, AssertionID: "probe", Suite: "verification_probe/python", Passed: true}}
		}, VerificationStatusUnavailable},
		{"aggregate", func(r *ChangeReport) {
			row := native
			row.ObservationScope = TestObservationScopeAggregate
			r.TestResults = []TestResult{row}
		}, VerificationStatusUnavailable},
		{"non_asserting", func(r *ChangeReport) {
			row := native
			row.ObservationScope = TestObservationScopeNonAsserting
			r.TestResults = []TestResult{row}
		}, VerificationStatusUnavailable},
		{"unknown_scope", func(r *ChangeReport) { row := native; row.ObservationScope = ""; r.TestResults = []TestResult{row} }, VerificationStatusUnavailable},
		{"missing_identity", func(r *ChangeReport) { row := native; row.AssertionID = ""; r.TestResults = []TestResult{row} }, VerificationStatusUnavailable},
		{"missing_suite", func(r *ChangeReport) { row := native; row.Suite = ""; r.TestResults = []TestResult{row} }, VerificationStatusUnavailable},
		{"syntax_only", func(r *ChangeReport) {
			r.ExecutedCommands = []ExecutedCommand{{Runner: "python", Outcome: ExecutedCommandOutcomeSyntaxPreflight, ExitCode: 0}}
		}, VerificationStatusUnavailable},
		{"build_row_not_assertion", func(r *ChangeReport) {
			row := native
			row.Kind = TestResultKindBuildError
			r.TestResults = []TestResult{row}
		}, VerificationStatusUnavailable},
		{"legacy_failed_row", func(r *ChangeReport) {
			r.Passed = false
			r.TestResults = []TestResult{{AssertionID: "TestFail", Suite: "project", Passed: false}}
		}, VerificationStatusFailed},
		{"native_failed_row", func(r *ChangeReport) {
			r.Passed = false
			row := native
			row.Passed = false
			r.TestResults = []TestResult{row}
		}, VerificationStatusFailed},
		{"build_failed", func(r *ChangeReport) { r.Passed = false; r.BuildFailed = true }, VerificationStatusFailed},
		{"build_failure_kind", func(r *ChangeReport) { r.Passed = false; r.FailureKind = FailureKindBuildFailure }, VerificationStatusFailed},
		{"timeout", func(r *ChangeReport) { r.Passed = false; r.FailureKind = FailureKindTimeout }, VerificationStatusFailed},
		{"failed_command", func(r *ChangeReport) {
			r.Passed = false
			r.ExecutedCommands = []ExecutedCommand{{Runner: "make", Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 2}}
		}, VerificationStatusFailed},
		{"missing_runner_after_native", func(r *ChangeReport) { r.TestResults = []TestResult{native}; r.FailureKind = FailureKindRunnerMissing }, VerificationStatusUnavailable},
		{"uncovered_path_after_native", func(r *ChangeReport) {
			r.TestResults = []TestResult{native}
			r.FailureKind = FailureKindVerificationIncomplete
			r.FailureReasonCode = "changed_path_verification_uncovered"
		}, VerificationStatusUnavailable},
		{"explicit_no_tests_stays_unavailable", func(r *ChangeReport) { r.TestResults = []TestResult{native}; r.FailureKind = FailureKindNoTests }, VerificationStatusUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &ChangeReport{Passed: true, NoTestsRunners: []string{"python"}}
			tc.edit(r)
			before, _ := json.Marshal(r)
			if got := r.NormalizeVerificationStatus(); got != tc.want {
				t.Fatalf("status=%s want=%s report=%s", got, tc.want, before)
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) {
				t.Fatal("classification mutated report")
			}
			r.EnsureVerificationStatus()
			if r.VerificationStatus != tc.want || r.NormalizeVerificationStatus() != tc.want {
				t.Fatalf("ensure disagrees: %+v", r)
			}
			wire, _ := json.Marshal(r)
			var restored ChangeReport
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.NormalizeVerificationStatus() != tc.want || !reflect.DeepEqual(restored.NoTestsRunners, []string{"python"}) {
				t.Fatalf("roundtrip lost status/census: %+v", restored)
			}
		})
	}
}

func TestNoTestsInvocationLabelsKeepOnlyObservedScope(t *testing.T) {
	r := &ChangeReport{NoTestsRunners: []string{"python", "ruby"}, ExecutedCommands: []ExecutedCommand{
		{Runner: "python", Framework: "unittest", WorkingDir: "packages/widget", Outcome: ExecutedCommandOutcomeExecuted},
		{Runner: "python", Framework: "unittest", WorkingDir: ".", Outcome: ExecutedCommandOutcomeZeroTests},
		{Runner: "python", Framework: "unittest", WorkingDir: ".", Outcome: ExecutedCommandOutcomeZeroTests},
		{Runner: "python", Framework: "unittest", WorkingDir: "packages/empty", Outcome: ExecutedCommandOutcomeSyntheticNoTests},
	}}
	want := []string{"python/unittest@.", "python/unittest@packages/empty", "ruby (invocation scope unavailable)"}
	if got := r.NoTestsInvocationLabels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels=%v want=%v", got, want)
	}
	r.ExecutedCommands = []ExecutedCommand{{Runner: "python", Outcome: ExecutedCommandOutcomeZeroTests}}
	if got := r.NoTestsInvocationLabels(); got[0] != "python (working directory unavailable)" {
		t.Fatalf("invented root directory: %v", got)
	}
}
