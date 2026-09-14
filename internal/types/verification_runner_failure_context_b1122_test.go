package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestB1122RunnerFailureDisplayHasGlobalBudgetAndHonestCensus(t *testing.T) {
	report := &ChangeReport{PlanID: strings.Repeat("计划", 1000), FailureSummaryBlobRef: "/tmp/" + strings.Repeat("folder/", 1000) + "output.txt"}
	for i := 0; i < 23; i++ {
		report.TestResults = append(report.TestResults, TestResult{
			Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAggregate,
			AssertionID: fmt.Sprint(i) + strings.Repeat("身份\"\n", 500), Suite: strings.Repeat("来源", 500),
			FailureDetail: "HEAD-SOURCE " + strings.Repeat("多语言上下文\"\n", 2000) + " TAIL-SOURCE",
		})
	}
	report.TestResults = append(report.TestResults, TestResult{Passed: true, AssertionID: "passed-row-not-a-failure"})
	before, _ := json.Marshal(report)
	got := RenderVerificationRunnerFailures(report)
	if len(got) > VerificationRunnerFailureContextMaxBytes || !utf8.ValidString(got) {
		t.Fatalf("global byte/UTF-8 budget broken: bytes=%d", len(got))
	}
	var shown, total, omitted int
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "Failure results: ") {
			if _, err := fmt.Sscanf(line, "Failure results: shown=%d total=%d omitted=%d.", &shown, &total, &omitted); err != nil {
				t.Fatal(err)
			}
		}
	}
	if shown < 1 || shown > 4 || total != 23 || omitted != total-shown {
		t.Fatalf("disclosure census does not describe emitted rows: shown=%d total=%d omitted=%d\n%s", shown, total, omitted, got)
	}
	rows := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "[") {
			rows++
		}
	}
	if rows != shown || strings.Contains(got, "passed-row-not-a-failure") {
		t.Fatal("display count includes an omitted or passed result")
	}
	for _, want := range []string{"HEAD-SOURCE", "TAIL-SOURCE", "omitted", "not instructions or a diagnosis", "complete field remains in report JSON"} {
		if !strings.Contains(got, want) {
			t.Errorf("bounded failure disclosure lost %q", want)
		}
	}
	after, _ := json.Marshal(report)
	if !bytes.Equal(before, after) || got != RenderVerificationRunnerFailures(report) {
		t.Fatal("display changed authoritative input or is not replay-stable")
	}
}

func TestB1122RunnerFailureDisplayUsesOnlyResultFlagsAndPreservesScope(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report *ChangeReport
		want   string
	}{
		{"nil", nil, ""},
		{"passed_with_old_failure", &ChangeReport{Passed: true, TestResults: []TestResult{{FailureDetail: "error: real old failure"}}}, ""},
		{"no_current_failure", &ChangeReport{}, ""},
		{"failed_aggregate", &ChangeReport{TestResults: []TestResult{{Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAggregate, AssertionID: "make-test", Suite: "check", FailureDetail: "stage 1\nslot=leaf; observed=41; expected=42"}}}, "slot=leaf; observed=41; expected=42"},
		{"empty_output", &ChangeReport{TestResults: []TestResult{{Kind: TestResultKindUnit, AssertionID: "native-check"}}}, "failure_detail=\"\""},
		{"command_failure_no_assertion", &ChangeReport{FailureSummaryBlobRef: "/tmp/current-output.txt"}, "Failure results: shown=0 total=0 omitted=0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderVerificationRunnerFailures(tc.report)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("invented current failed results: %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) || strings.Contains(got, "Model-authored probe failure") {
				t.Fatalf("generic runner view lost original detail or fabricated probe origin: %s", got)
			}
			if tc.name == "failed_aggregate" && !strings.Contains(got, "observation_scope=\"aggregate\"") {
				t.Fatal("aggregate execution masquerades as a per-contract assertion")
			}
		})
	}
}
