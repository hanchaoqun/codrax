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
