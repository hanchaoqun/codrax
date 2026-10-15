package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func verificationReportHMC244() *types.ChangeReport {
	return &types.ChangeReport{PlanID: "p244", Channel: types.ChangeReportChannelPostApplyVerify, Passed: true,
		TestResults: []types.TestResult{
			{InvocationID: "unittest", AssertionID: "test_a", ObservationScope: types.TestObservationScopeAssertion, Passed: true},
			{InvocationID: "unittest", AssertionID: "test_b", ObservationScope: types.TestObservationScopeAssertion, Passed: true},
			{InvocationID: "unittest", AssertionID: "test_c", ObservationScope: types.TestObservationScopeAssertion, Passed: true},
			{InvocationID: "probe", AssertionID: "probe", ObservationScope: types.TestObservationScopeAssertion, Passed: true},
		}}
}

func TestHMC244FinishUsesRecordedVerificationFacts(t *testing.T) {
	run := types.WriteWorkflowRun{RunID: "wf244", Goal: "fix widget", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "b244",
		Batches: []types.WriteWorkflowBatch{{ID: "b244", PlanID: "p244", Status: types.WriteWorkflowBatchVerifying}}}
	report := verificationReportHMC244()
	updateWorkflowRunBatchVerify(&run, "b244", report, "passed", "tests_passed")
	// Exercise the durable boundary before finish, not just an in-memory card.
	wire, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	var restored types.WriteWorkflowRun
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	got, err := writeflow.ApplyWorkflowDecisionToRun(restored, writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish,
		ReasonCode: "done", Reason: "test_widget.py全部4个测试通过"})
	if err != nil {
		t.Fatal(err)
	}
	message := got.ProgressLedger[len(got.ProgressLedger)-1].Message
	if !strings.Contains(message, "4 verification result(s)") || strings.Contains(message, "test_widget.py") {
		t.Fatalf("finish promoted model prose instead of report facts: %q", message)
	}
	if got.Status != types.WriteWorkflowRunComplete || got.Completion == nil || got.Completion.Verdict != types.WriteWorkflowCompletionVerified {
		t.Fatalf("display changed completion authority: %+v", got.Completion)
	}
	if got.ProgressLedger[len(got.ProgressLedger)-1].AdvisoryMessage != "test_widget.py全部4个测试通过" {
		t.Fatal("original model explanation was not retained as advisory audit")
	}
	if !strings.Contains(renderVerifySuccess(report, "en"), "4 verification result(s)") {
		t.Fatal("card and durable finish use different count units")
	}
}

func TestHMC244RecordVerifyDoesNotBorrowOtherPlanOrReport(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*types.ChangeReport)
	}{
		{"other_plan", func(r *types.ChangeReport) { r.PlanID = "other" }},
		{"legacy_channel", func(r *types.ChangeReport) { r.Channel = "" }},
		{"reference_channel", func(r *types.ChangeReport) { r.Channel = types.ChangeReportChannel("reference") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := types.WriteWorkflowRun{RunID: "run", ActiveBatchID: "batch", Batches: []types.WriteWorkflowBatch{{ID: "batch", PlanID: "p244"}}}
			report := verificationReportHMC244()
			tc.change(report)
			updateWorkflowRunBatchVerify(&run, "batch", report, "passed", "tests_passed")
			if run.Batches[0].Attempts[0].VerificationResults != nil {
				t.Fatal("unbound report minted display counts")
			}
			if run.Batches[0].Attempts[0].Status != "passed" {
				t.Fatal("display changed existing attempt status")
			}
		})
	}
}

func TestHMC244VerifySuccessDoesNotCallEveryRecordedRowPassed(t *testing.T) {
	report := verificationReportHMC244()
	report.TestResults[2].Passed = false // e.g. a retained reference-run failure
	report.TestResults[3].ObservationScope = types.TestObservationScopeNonAsserting
	for _, lang := range []string{"en", "zh"} {
		got := renderVerifySuccess(report, lang)
		if strings.Contains(got, "4 verification result(s) recorded as passed") || strings.Contains(got, "4 条验证结果已记录为通过") {
			t.Fatalf("mixed rows were all called passed: %q", got)
		}
		if !strings.Contains(got, "4") || !strings.Contains(got, "2") || !strings.Contains(got, "1") {
			t.Fatalf("mixed recorded counts omitted: %q", got)
		}
	}
}
