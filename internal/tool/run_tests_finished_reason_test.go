package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFinishedReportSummaryUsesEffectiveExecutionReason(t *testing.T) {
	plan := &types.ChangePlan{ID: "required-native-execution", WriteAnalysisIR: &types.WriteAnalysisIR{}}
	plan.WriteAnalysisIR.Request.Constraints = []types.WriteConstraint{{Kind: types.WriteConstraintRunExistingTest, Target: "tests/test_total.py"}}
	report := types.EffectiveExistingTestExecutionReport(plan, &types.ChangeReport{
		PlanID: plan.ID, Passed: true, Channel: types.ChangeReportChannelPostApplyVerify,
		VerificationStatus: types.VerificationStatusPassed,
	})
	if report.FailureReasonCode != "required_existing_test_not_executed" {
		t.Fatalf("fixture did not reach the production projection: %+v", report)
	}
	before, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	got := finishedReportSummary(report, "local test invocation passed")
	if !strings.Contains(got, "reason_code="+report.FailureReasonCode+"]") || strings.Contains(got, changedPathVerificationUncoveredReasonCode) {
		t.Fatalf("tool summary must disclose the effective report's reason: %s", got)
	}
	if !strings.Contains(got, report.FailureSummary) {
		t.Fatalf("effective report explanation was lost: %s", got)
	}
	after, err := json.Marshal(report)
	if err != nil || string(before) != string(after) {
		t.Fatal("summary rendering changed the report")
	}
}

func TestFinishedReportSummaryReasonIsNotInferredFromProse(t *testing.T) {
	for _, code := range []string{changedPathVerificationUncoveredReasonCode, "future_required_proof_missing", ""} {
		t.Run(code, func(t *testing.T) {
			report := &types.ChangeReport{
				FailureKind: types.FailureKindVerificationIncomplete, FailureReasonCode: code,
				FailureSummary: "free prose mentions runner_missing but grants no reason authority",
			}
			got := finishedReportSummary(report, "fallback")
			want := "[run_tests: verdict=UNAVAILABLE"
			if code != "" {
				want += " reason_code=" + code
			}
			want += "] " + report.FailureSummary
			if got != want {
				t.Fatalf("summary = %q, want %q", got, want)
			}
		})
	}
	for _, report := range []*types.ChangeReport{
		nil,
		{Passed: true},
		{FailureKind: types.FailureKindTestsFailed, FailureSummary: "native failure"},
		{FailureKind: types.FailureKindVerificationIncomplete},
	} {
		if got := finishedReportSummary(report, "fallback"); got != "fallback" {
			t.Fatalf("unrelated or unexplained verdict changed: %q", got)
		}
	}
}
