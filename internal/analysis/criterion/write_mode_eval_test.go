package criterion

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// wcFromApplied builds a WriteClosure with the given files marked
// as applied — test fixture for evalPatchApplies.
func wcFromApplied(applied ...string) *types.WriteClosure {
	wc := &types.WriteClosure{}
	for _, f := range applied {
		wc.MarkApplied(f)
	}
	return wc
}

// ── evalPatchApplies ───────────────────────────────────────────────

func TestEvalPatchApplies_ReadModeSatisfied(t *testing.T) {
	// No ChangePlan in env → read-mode / pre-plan: trivially satisfied.
	r := evalPatchApplies("", Env{})
	if !r.Satisfied {
		t.Errorf("read mode must short-circuit satisfied; got %+v", r)
	}
	if !strings.Contains(r.Detail, "no ChangePlan") {
		t.Errorf("detail should explain the short-circuit; got %q", r.Detail)
	}
}

func TestEvalPatchApplies_NoWriteClosureSatisfied(t *testing.T) {
	// Plan set but WriteClosure still nil — defensive short-circuit
	// (real pipeline never hits this; buildEnv always attaches WC).
	env := Env{
		ChangePlan: &types.ChangePlan{TargetPaths: []string{"a.go"}},
	}
	r := evalPatchApplies("", env)
	if !r.Satisfied {
		t.Errorf("nil WriteClosure should short-circuit satisfied; got %+v", r)
	}
}

func TestEvalPatchApplies_AllApplied(t *testing.T) {
	env := Env{
		ChangePlan:   &types.ChangePlan{TargetPaths: []string{"a.go", "b.go"}},
		WriteClosure: wcFromApplied("a.go", "b.go"),
	}
	r := evalPatchApplies("", env)
	if !r.Satisfied {
		t.Errorf("all targets applied should be satisfied; got %+v", r)
	}
}

func TestEvalPatchApplies_SomeMissing(t *testing.T) {
	env := Env{
		ChangePlan:   &types.ChangePlan{TargetPaths: []string{"a.go", "b.go", "c.go"}},
		WriteClosure: wcFromApplied("a.go"),
	}
	r := evalPatchApplies("", env)
	if r.Satisfied {
		t.Errorf("missing targets should NOT be satisfied; got %+v", r)
	}
	if !strings.Contains(r.Detail, "b.go") || !strings.Contains(r.Detail, "c.go") {
		t.Errorf("detail must list the missing paths; got %q", r.Detail)
	}
}

func TestEvalPatchApplies_SubstringFilter(t *testing.T) {
	// Filter matches only one target; that one is applied → satisfied
	// even though another (unfiltered) target is missing.
	env := Env{
		ChangePlan: &types.ChangePlan{
			TargetPaths: []string{"internal/a.go", "cmd/b.go"},
		},
		WriteClosure: wcFromApplied("internal/a.go"),
	}
	r := evalPatchApplies("internal/", env)
	if !r.Satisfied {
		t.Errorf("filter-matching target is applied — should be satisfied; got %+v", r)
	}
}

func TestEvalPatchApplies_FilterNoMatch(t *testing.T) {
	// Filter excludes every target — trivially satisfied ("nothing to check").
	env := Env{
		ChangePlan:   &types.ChangePlan{TargetPaths: []string{"a.go"}},
		WriteClosure: wcFromApplied(),
	}
	r := evalPatchApplies("zzz", env)
	if !r.Satisfied {
		t.Errorf("empty filter result should be satisfied; got %+v", r)
	}
	if !strings.Contains(r.Detail, "0/") {
		t.Errorf("detail should indicate empty match; got %q", r.Detail)
	}
}

// ── evalTestsPass ──────────────────────────────────────────────────

func TestEvalTestsPass_NoReportSatisfied(t *testing.T) {
	r := evalTestsPass("", Env{})
	if !r.Satisfied {
		t.Error("nil ChangeReport should short-circuit satisfied")
	}
}

func TestEvalTestsPass_AllPassed(t *testing.T) {
	env := Env{
		ChangeReport: &types.ChangeReport{
			TestResults: []types.TestResult{
				{AssertionID: "TestA", Passed: true},
				{AssertionID: "TestB", Passed: true},
			},
		},
	}
	r := evalTestsPass("", env)
	if !r.Satisfied {
		t.Errorf("all passed should be satisfied; got %+v", r)
	}
}

func TestEvalTestsPass_SomeFailed(t *testing.T) {
	env := Env{
		ChangeReport: &types.ChangeReport{
			TestResults: []types.TestResult{
				{AssertionID: "TestA", Passed: true},
				{AssertionID: "TestBad", Passed: false},
				{AssertionID: "TestOther", Passed: false},
			},
		},
	}
	r := evalTestsPass("", env)
	if r.Satisfied {
		t.Errorf("with failures should NOT be satisfied; got %+v", r)
	}
	if !strings.Contains(r.Detail, "TestBad") || !strings.Contains(r.Detail, "TestOther") {
		t.Errorf("detail must list failed assertion IDs; got %q", r.Detail)
	}
}

func TestEvalTestsPass_FilterIsolatesFailure(t *testing.T) {
	// Two failures exist, filter narrows to only the "Parser"-named
	// one — satisfied when that one is passing.
	env := Env{
		ChangeReport: &types.ChangeReport{
			TestResults: []types.TestResult{
				{AssertionID: "TestParserHappyPath", Passed: true},
				{AssertionID: "TestOtherBad", Passed: false},
			},
		},
	}
	r := evalTestsPass("Parser", env)
	if !r.Satisfied {
		t.Errorf("filter should exclude the unrelated failure; got %+v", r)
	}
}

func TestEvalTestsPass_ManyFailuresTruncatedDetail(t *testing.T) {
	var results []types.TestResult
	for i := 0; i < 20; i++ {
		results = append(results, types.TestResult{
			AssertionID: "TestF" + string(rune('A'+i)),
			Passed:      false,
		})
	}
	env := Env{ChangeReport: &types.ChangeReport{TestResults: results}}
	r := evalTestsPass("", env)
	if r.Satisfied {
		t.Error("should not be satisfied")
	}
	if !strings.Contains(r.Detail, "more") {
		t.Errorf("detail should elide long failure lists with '+N more'; got %q", r.Detail)
	}
}

// ── evalNoRegression ───────────────────────────────────────────────

func TestEvalNoRegression_NoReportSatisfied(t *testing.T) {
	r := evalNoRegression("", Env{})
	if !r.Satisfied {
		t.Error("nil ChangeReport should short-circuit satisfied")
	}
}

func TestEvalNoRegression_NoBaselineSatisfied(t *testing.T) {
	env := Env{
		ChangeReport: &types.ChangeReport{
			TestResults: []types.TestResult{{AssertionID: "X", Passed: false}},
		},
		// BaselineReport nil on purpose
	}
	r := evalNoRegression("", env)
	if !r.Satisfied {
		t.Errorf("missing baseline should short-circuit satisfied (can't compare); got %+v", r)
	}
	if !strings.Contains(r.Detail, "BaselineReport") {
		t.Errorf("detail should name the missing baseline; got %q", r.Detail)
	}
}

func TestEvalNoRegression_TestRegression(t *testing.T) {
	// TestA passed in baseline, fails post-apply → regression.
	env := Env{
		BaselineReport: &types.ChangeReport{
			TestResults: []types.TestResult{
				{AssertionID: "TestA", Passed: true},
				{AssertionID: "TestB", Passed: true},
			},
		},
		ChangeReport: &types.ChangeReport{
			TestResults: []types.TestResult{
				{AssertionID: "TestA", Passed: false},
				{AssertionID: "TestB", Passed: true},
			},
		},
	}
	r := evalNoRegression("", env)
	if r.Satisfied {
		t.Errorf("regression should not be satisfied; got %+v", r)
	}
	if !strings.Contains(r.Detail, "TestA") {
		t.Errorf("detail should name regressed test; got %q", r.Detail)
	}
}

func TestEvalNoRegression_NewlyFailingTestNotRegression(t *testing.T) {
	// TestC was already failing in baseline → not a regression when
	// it stays failing post-apply.
	env := Env{
		BaselineReport: &types.ChangeReport{
			TestResults: []types.TestResult{{AssertionID: "TestC", Passed: false}},
		},
		ChangeReport: &types.ChangeReport{
			TestResults: []types.TestResult{{AssertionID: "TestC", Passed: false}},
		},
	}
	r := evalNoRegression("", env)
	if !r.Satisfied {
		t.Errorf("pre-existing failure is not a regression; got %+v", r)
	}
}

func TestEvalNoRegression_MetricRegression(t *testing.T) {
	// Latency metric: baseline 100ms, current 150ms, threshold 20ms
	// → regression (50 > 20).
	env := Env{
		BaselineReport: &types.ChangeReport{},
		ChangeReport: &types.ChangeReport{
			MetricDeltas: map[string]types.MetricDelta{
				"latency_ms": {
					Baseline:  100.0,
					Current:   150.0,
					Unit:      "ms",
					Threshold: 20.0,
				},
			},
		},
	}
	r := evalNoRegression("", env)
	if r.Satisfied {
		t.Errorf("metric regression should not be satisfied; got %+v", r)
	}
	if !strings.Contains(r.Detail, "latency_ms") {
		t.Errorf("detail should name the regressed metric; got %q", r.Detail)
	}
}

func TestEvalNoRegression_MetricWithinThreshold(t *testing.T) {
	// Increase of 15 under threshold of 20 → not a regression.
	env := Env{
		BaselineReport: &types.ChangeReport{},
		ChangeReport: &types.ChangeReport{
			MetricDeltas: map[string]types.MetricDelta{
				"latency_ms": {Baseline: 100, Current: 115, Threshold: 20},
			},
		},
	}
	r := evalNoRegression("", env)
	if !r.Satisfied {
		t.Errorf("delta under threshold is not a regression; got %+v", r)
	}
}

func TestEvalNoRegression_ZeroThresholdSkipped(t *testing.T) {
	// MetricDelta.Threshold == 0 means "operator did not configure"
	// — those metrics are not checked for regression.
	env := Env{
		BaselineReport: &types.ChangeReport{},
		ChangeReport: &types.ChangeReport{
			MetricDeltas: map[string]types.MetricDelta{
				"unconfigured": {Baseline: 0, Current: 1_000_000, Threshold: 0},
			},
		},
	}
	r := evalNoRegression("", env)
	if !r.Satisfied {
		t.Errorf("zero-threshold metric must be skipped; got %+v", r)
	}
}
