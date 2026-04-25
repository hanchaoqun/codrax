package types

import (
	"strings"
	"testing"
)

func TestCollectBaselineFailures_NilBaseline(t *testing.T) {
	if got := CollectBaselineFailures(nil); got != nil {
		t.Errorf("nil → nil; got %v", got)
	}
}

func TestCollectBaselineFailures_EmptyResults(t *testing.T) {
	got := CollectBaselineFailures(&ChangeReport{})
	if got != nil {
		t.Errorf("empty results → nil; got %v", got)
	}
}

func TestCollectBaselineFailures_PreservesOrder(t *testing.T) {
	in := &ChangeReport{
		TestResults: []TestResult{
			{Kind: TestResultKindUnit, AssertionID: "A", Passed: true},
			{Kind: TestResultKindUnit, AssertionID: "B", Passed: false},
			{Kind: TestResultKindUnit, AssertionID: "C", Passed: false},
			{Kind: TestResultKindUnit, AssertionID: "D", Passed: true},
		},
	}
	got := CollectBaselineFailures(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 failures; got %d", len(got))
	}
	if got[0].AssertionID != "B" || got[1].AssertionID != "C" {
		t.Errorf("order not preserved; got %+v", got)
	}
}

// Build-error rows (Kind=BuildError) are NOT baseline test failures —
// they belong to a different failure class. Skip them so the coder /
// verifier prompts don't show "fix the compile error before tests
// could run" as a pre-existing test failure.
func TestCollectBaselineFailures_SkipsBuildErrors(t *testing.T) {
	in := &ChangeReport{
		TestResults: []TestResult{
			{Kind: TestResultKindUnit, AssertionID: "TestUnit", Passed: false},
			{Kind: TestResultKindBuildError, AssertionID: "/x.java:1", Passed: false},
		},
	}
	got := CollectBaselineFailures(in)
	if len(got) != 1 {
		t.Fatalf("build-error row should be excluded; got %d entries", len(got))
	}
	if got[0].Kind != TestResultKindUnit {
		t.Errorf("only unit-test failures should remain; got %v", got[0].Kind)
	}
}

func TestRenderBaselineFailureList_BasicShape(t *testing.T) {
	got := RenderBaselineFailureList([]TestResult{
		{AssertionID: "TestFoo", Suite: "pkg/a"},
		{AssertionID: "TestBar"},
	}, 15)
	if !strings.Contains(got, "- TestFoo (pkg/a)") {
		t.Errorf("suite-bearing entry should render with parens; got %q", got)
	}
	if !strings.Contains(got, "- TestBar\n") {
		t.Errorf("suite-less entry should render bare; got %q", got)
	}
}

func TestRenderBaselineFailureList_EmptyReturnsEmpty(t *testing.T) {
	if got := RenderBaselineFailureList(nil, 15); got != "" {
		t.Errorf("nil → empty string; got %q", got)
	}
}

func TestRenderBaselineFailureList_CapsAndOverflows(t *testing.T) {
	var failures []TestResult
	for i := 0; i < 25; i++ {
		failures = append(failures, TestResult{AssertionID: "TestX"})
	}
	got := RenderBaselineFailureList(failures, 15)
	if !strings.Contains(got, "+10 more") {
		t.Errorf("should mark +10 more after cap=15; got %q", got)
	}
}

func TestRenderBaselineFailureList_CapBelowOneClampedToOne(t *testing.T) {
	failures := []TestResult{{AssertionID: "A"}, {AssertionID: "B"}}
	got := RenderBaselineFailureList(failures, 0)
	// At least one entry should render (cap clamped to 1) plus the overflow.
	if !strings.Contains(got, "- A") {
		t.Errorf("first entry should render at clamped cap; got %q", got)
	}
	if !strings.Contains(got, "+1 more") {
		t.Errorf("overflow marker missing; got %q", got)
	}
}
