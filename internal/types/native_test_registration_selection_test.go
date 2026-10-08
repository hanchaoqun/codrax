package types

import (
	"fmt"
	"strings"
	"testing"
)

func TestNativeRegistrationSelectionBoundedAndGenerationScoped(t *testing.T) {
	report := b1788IdentityReport()
	report.TestResults = nil
	for i := 0; i < 12; i++ {
		report.TestResults = append(report.TestResults, TestResult{Suite: "python/unittest@component::tests.Sample", AssertionID: fmt.Sprintf("python/unittest@component::test_%d", i), Passed: true, ObservationScope: TestObservationScopeAssertion})
	}
	text, choices := renderNativeTestIdentitySnapshotChoices("active", report, true, "grant-one")
	if len(choices) != 8 || len(text) > 8*1024 || !strings.Contains(text, "shown=8 total=12 omitted=4") {
		t.Fatalf("selection bypassed display budget: %d/%d %s", len(choices), len(text), text)
	}
	for _, choice := range choices {
		if !strings.Contains(text, choice.Ref) || !strings.Contains(text, choice.AssertionID) {
			t.Fatal("hidden row became selectable")
		}
	}
	_, replacement := renderNativeTestIdentitySnapshotChoices("active", report, true, "grant-two")
	for i, choice := range choices {
		if choice.Ref == replacement[i].Ref {
			t.Fatal("replaced grant revived a reference")
		}
	}
	_, ordinary := renderNativeTestIdentitySnapshotChoices("active", report, false, "")
	if len(ordinary) != 0 {
		t.Fatal("ordinary report created registration selectors")
	}
	for _, id := range []string{"bad\nid", " padded ", strings.Repeat("x", 513)} {
		report.TestResults = []TestResult{{Suite: "suite", AssertionID: id, Passed: true, ObservationScope: TestObservationScopeAssertion}}
		_, invalid := renderNativeTestIdentitySnapshotChoices("active", report, true, "grant")
		if len(invalid) != 0 {
			t.Fatalf("unusable pair became selector: %q", id)
		}
	}
}
