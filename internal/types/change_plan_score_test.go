package types

import "testing"

// TestChangeReportScore_CountsOnlyUnitTests verifies Score() ignores
// build_error rows so a synthetic compile-failure entry cannot be
// misread as a passing test. Total counts only TestResultKindUnit (or
// blank Kind, which defaults to Unit).
func TestChangeReportScore_CountsOnlyUnitTests(t *testing.T) {
	r := &ChangeReport{
		TestResults: []TestResult{
			{Kind: TestResultKindUnit, Passed: true},
			{Kind: TestResultKindUnit, Passed: true},
			{Kind: TestResultKindUnit, Passed: false},
			{Kind: "", Passed: true}, // blank Kind defaults to Unit
			{Kind: TestResultKindBuildError, Passed: false},
		},
	}
	passed, total := r.Score()
	if passed != 3 || total != 4 {
		t.Errorf("Score() = (%d, %d), want (3, 4)", passed, total)
	}
}

// TestChangeReportScore_NilReceiverReturnsSentinel keeps the public
// contract: a nil receiver returns (-1, -1) so callers don't have to
// add a separate nil guard at every call site.
func TestChangeReportScore_NilReceiverReturnsSentinel(t *testing.T) {
	var r *ChangeReport
	passed, total := r.Score()
	if passed != -1 || total != -1 {
		t.Errorf("nil Score() = (%d, %d), want (-1, -1)", passed, total)
	}
}

func TestChangeReportNormalizeVerificationStatus(t *testing.T) {
	cases := []struct {
		name   string
		report *ChangeReport
		want   VerificationStatus
	}{
		{
			name:   "assertions passed",
			report: &ChangeReport{Passed: true, TestResults: []TestResult{{AssertionID: "TestOK", Passed: true}}},
			want:   VerificationStatusPassed,
		},
		{
			name:   "zero tests unavailable even when legacy passed flag is true",
			report: &ChangeReport{Passed: true, NoTestsRunners: []string{"python"}},
			want:   VerificationStatusUnavailable,
		},
		{
			name:   "parser error unavailable",
			report: &ChangeReport{Passed: false, FailureKind: FailureKindParserError},
			want:   VerificationStatusUnavailable,
		},
		{
			name:   "preexisting build failure unavailable",
			report: &ChangeReport{Passed: false, FailureKind: FailureKindPreexistingBuildFailure},
			want:   VerificationStatusUnavailable,
		},
		{
			name: "unreasoned build surface with verifier unavailable reason is unavailable",
			report: &ChangeReport{
				Passed:            false,
				BuildFailed:       true,
				FailureKind:       FailureKindBuildFailure,
				FailureReasonCode: "verification_probe_module_not_found",
			},
			want: VerificationStatusUnavailable,
		},
		{
			name: "real build failure reason stays failed",
			report: &ChangeReport{
				Passed:            false,
				BuildFailed:       true,
				FailureKind:       FailureKindBuildFailure,
				FailureReasonCode: "typescript_compile_check_failed",
			},
			want: VerificationStatusFailed,
		},
		{
			name:   "red assertion failed",
			report: &ChangeReport{Passed: false, TestResults: []TestResult{{AssertionID: "TestBad", Passed: false}}},
			want:   VerificationStatusFailed,
		},
		{
			name: "probe import unavailable rows are unavailable",
			report: &ChangeReport{
				Passed:      false,
				FailureKind: FailureKindTestsFailed,
				TestResults: []TestResult{{
					Kind:        TestResultKindUnit,
					AssertionID: "probe-import-boundary",
					Suite:       "verification_probe/python",
					Passed:      false,
				}},
				ExecutedCommands: []ExecutedCommand{{
					Runner:     "verification_probe",
					Framework:  "python",
					Outcome:    "parser_error",
					ReasonCode: "verification_probe_import_error",
					ExitCode:   1,
				}},
				VerificationConfidence: []VerificationConfidenceRecord{{
					Source:     "pre_suite_verification_probe",
					Category:   "probe_execution",
					Status:     "unavailable",
					ReasonCode: "verification_probe_import_error",
				}},
			},
			want: VerificationStatusUnavailable,
		},
		{
			name: "nested probe import unavailable rows are unavailable",
			report: &ChangeReport{
				Passed:      false,
				FailureKind: FailureKindTestsFailed,
				TestResults: []TestResult{{
					Kind:        TestResultKindUnit,
					AssertionID: "python/pytest@examples/javascript::blueprint_name_dot_error",
					Suite:       "python/pytest@examples/javascript::verification_probe/python",
					Passed:      false,
				}},
				ExecutedCommands: []ExecutedCommand{{
					Runner:     "verification_probe",
					Framework:  "python",
					Outcome:    "parser_error",
					ReasonCode: "verification_probe_import_error",
					ExitCode:   1,
					Source:     "parser_error_verification_probe",
				}},
				VerificationConfidence: []VerificationConfidenceRecord{{
					Source:     "parser_error_verification_probe",
					Category:   "probe_execution",
					Status:     "unavailable",
					ReasonCode: "verification_probe_import_error",
				}},
			},
			want: VerificationStatusUnavailable,
		},
		{
			name: "make unavailable command explains qualified make row",
			report: &ChangeReport{
				Passed:      false,
				FailureKind: FailureKindTestsFailed,
				TestResults: []TestResult{{
					Kind:        TestResultKindUnit,
					AssertionID: "make@cextern/wcslib::make-test",
					Suite:       "make@cextern/wcslib::make",
					Passed:      false,
				}},
				ExecutedCommands: []ExecutedCommand{{
					Runner:     "make",
					WorkingDir: "cextern/wcslib",
					Suite:      "make",
					Outcome:    "parser_error",
					ReasonCode: "make_dependency_file_missing",
					ExitCode:   2,
				}},
			},
			want: VerificationStatusUnavailable,
		},
		{
			name: "real red assertion outranks secondary unavailable probe",
			report: &ChangeReport{
				Passed:      false,
				FailureKind: FailureKindTestsFailed,
				TestResults: []TestResult{{
					Kind:        TestResultKindUnit,
					AssertionID: "tests/test_widget.py::test_widget",
					Suite:       "pytest",
					Passed:      false,
				}},
				ExecutedCommands: []ExecutedCommand{{
					Runner:     "verification_probe",
					Framework:  "python",
					Outcome:    "parser_error",
					ReasonCode: "verification_probe_import_error",
					ExitCode:   1,
				}},
				VerificationConfidence: []VerificationConfidenceRecord{{
					Source:     "pre_suite_verification_probe",
					Category:   "probe_execution",
					Status:     "unavailable",
					ReasonCode: "verification_probe_import_error",
				}},
			},
			want: VerificationStatusFailed,
		},
		{
			name: "unreasoned make execution stays failed",
			report: &ChangeReport{
				Passed:      false,
				FailureKind: FailureKindTestsFailed,
				TestResults: []TestResult{{
					Kind:        TestResultKindUnit,
					AssertionID: "make@pkg::make-test",
					Suite:       "make@pkg::make",
					Passed:      false,
				}},
				ExecutedCommands: []ExecutedCommand{{
					Runner:     "make",
					WorkingDir: "pkg",
					Suite:      "make",
					Outcome:    "executed",
					ExitCode:   2,
				}},
			},
			want: VerificationStatusFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.report.NormalizeVerificationStatus(); got != tc.want {
				t.Errorf("NormalizeVerificationStatus() = %q, want %q", got, tc.want)
			}
			tc.report.EnsureVerificationStatus()
			if tc.report.VerificationStatus != tc.want {
				t.Errorf("EnsureVerificationStatus() stored %q, want %q", tc.report.VerificationStatus, tc.want)
			}
		})
	}
}

func TestChangeReportEnsureVerificationStatusBackfillsNoTestsFailureKind(t *testing.T) {
	report := &ChangeReport{Passed: true, NoTestsRunners: []string{"python"}}
	report.EnsureVerificationStatus()
	if report.VerificationStatus != VerificationStatusUnavailable {
		t.Fatalf("VerificationStatus = %q, want unavailable", report.VerificationStatus)
	}
	if report.FailureKind != FailureKindNoTests {
		t.Fatalf("FailureKind = %q, want %q", report.FailureKind, FailureKindNoTests)
	}
}

// TestChangeReportIsBetterThan covers the strict-improvement contract:
// a tie keeps the existing best (returns false) so the latch never
// thrashes between equivalent plans; nil-vs-non-nil follows obvious
// dominance rules.
func TestChangeReportIsBetterThan(t *testing.T) {
	mk := func(passed, fail int) *ChangeReport {
		results := make([]TestResult, 0, passed+fail)
		for i := 0; i < passed; i++ {
			results = append(results, TestResult{Kind: TestResultKindUnit, Passed: true})
		}
		for i := 0; i < fail; i++ {
			results = append(results, TestResult{Kind: TestResultKindUnit, Passed: false})
		}
		return &ChangeReport{TestResults: results}
	}

	cases := []struct {
		name string
		a, b *ChangeReport
		want bool
	}{
		{"strictly_more_passes", mk(49, 5), mk(46, 8), true},
		{"strictly_fewer_passes", mk(46, 8), mk(49, 5), false},
		{"tie_on_passes_more_total_wins", mk(5, 5), mk(5, 0), true},
		{"identical_score_ties", mk(5, 5), mk(5, 5), false},
		{"nil_loses_to_non_nil", nil, mk(0, 0), false},
		{"non_nil_beats_nil", mk(0, 0), nil, true},
		{"nil_vs_nil_false", nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.IsBetterThan(tc.b); got != tc.want {
				t.Errorf("IsBetterThan = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMutableState_BestPlanReportRoundTrip pins the storage contract:
// SetBestPlanReport overwrites unconditionally (caller owns the score
// comparison), Reset clears, and BestPlanReport on a nil receiver is
// safe and returns (nil, nil).
func TestMutableState_BestPlanReportRoundTrip(t *testing.T) {
	m := NewMutableState("test")
	plan := &ChangePlan{ID: "plan-1"}
	report := &ChangeReport{PlanID: "plan-1", Passed: true}

	gotPlan, gotReport := m.BestPlanReport()
	if gotPlan != nil || gotReport != nil {
		t.Errorf("zero state should return (nil, nil); got (%v, %v)", gotPlan, gotReport)
	}

	m.SetBestPlanReport(plan, report)
	gotPlan, gotReport = m.BestPlanReport()
	if gotPlan != plan || gotReport != report {
		t.Errorf("after Set, got (%v, %v); want (%v, %v)", gotPlan, gotReport, plan, report)
	}

	// Storage is dumb — second Set unconditionally overwrites even
	// with a "worse" report. The score check belongs to the caller.
	worsePlan := &ChangePlan{ID: "plan-2"}
	worseReport := &ChangeReport{PlanID: "plan-2", Passed: false}
	m.SetBestPlanReport(worsePlan, worseReport)
	gotPlan, _ = m.BestPlanReport()
	if gotPlan != worsePlan {
		t.Error("SetBestPlanReport should overwrite unconditionally")
	}

	m.ResetBestPlanReport()
	gotPlan, gotReport = m.BestPlanReport()
	if gotPlan != nil || gotReport != nil {
		t.Errorf("after Reset, got (%v, %v); want (nil, nil)", gotPlan, gotReport)
	}

	// Nil receiver safety.
	var nilM *MutableState
	gotPlan, gotReport = nilM.BestPlanReport()
	if gotPlan != nil || gotReport != nil {
		t.Errorf("nil receiver: got (%v, %v); want (nil, nil)", gotPlan, gotReport)
	}
}
