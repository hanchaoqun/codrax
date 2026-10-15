package writeflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func verificationBatchHMC244(id string) types.WriteWorkflowBatch {
	a := types.WriteWorkflowAttempt{ID: "attempt-1", Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: id, ReportID: id + ".report.json", ArtifactRef: id + ".report.json"}
	a.VerificationResults = types.BindWriteWorkflowVerificationResults(id, a, &types.ChangeReport{PlanID: id, Channel: types.ChangeReportChannelPostApplyVerify, TestResults: []types.TestResult{{Passed: true}, {Passed: true}}})
	return types.WriteWorkflowBatch{ID: id, PlanID: id, VerifyRef: a.ReportID, Status: types.WriteWorkflowBatchComplete, Attempts: []types.WriteWorkflowAttempt{a}}
}

func TestHMC244FinishFactsDoNotMergeBatchesOrChangeGate(t *testing.T) {
	run := types.WriteWorkflowRun{RunID: "run", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "b", Batches: []types.WriteWorkflowBatch{verificationBatchHMC244("a"), verificationBatchHMC244("b")}}
	decision := WriteWorkflowDecision{Action: ActionFinish, Reason: "all 40 independent tests passed"}
	got, err := ApplyWorkflowDecisionToRun(run, decision)
	if err != nil {
		t.Fatal(err)
	}
	last := got.ProgressLedger[len(got.ProgressLedger)-1]
	if strings.Count(last.Message, "2 verification result(s)") != 2 || strings.Contains(last.Message, "4 verification result(s)") || strings.Contains(last.Message, "40") {
		t.Fatalf("overlapping batches improperly summed: %q", last.Message)
	}
	if last.AdvisoryMessage != decision.Reason {
		t.Fatal("model audit explanation lost")
	}
	// A snapshot has no authority to repair failed verification or permit finish.
	run.Batches[1].Status = types.WriteWorkflowBatchReadyToPlan
	run.Batches[1].Attempts[0].Status = "failed"
	run.Batches[1].Attempts[0].ReasonCode = "tests_failed"
	if _, err := ApplyWorkflowDecisionToRun(run, decision); err == nil {
		t.Fatal("display snapshot bypassed finish gate")
	}
	decision.FinishDisposition = FinishDispositionAcceptUnverified
	if _, err := ApplyWorkflowDecisionToRun(run, decision); err == nil {
		t.Fatal("display snapshot bypassed typed failure gate")
	}
}

func TestHMC244FinishLegacyAndStaleCountsStayUnknown(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*types.WriteWorkflowBatch)
	}{
		{"legacy", func(b *types.WriteWorkflowBatch) { b.Attempts[0].VerificationResults = nil }},
		{"plan", func(b *types.WriteWorkflowBatch) { b.PlanID = "different" }},
		{"new_attempt", func(b *types.WriteWorkflowBatch) {
			b.Attempts = append(b.Attempts, types.WriteWorkflowAttempt{Kind: "verify", Status: "passed", PlanID: b.PlanID})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := verificationBatchHMC244("a")
			tc.change(&b)
			run := types.WriteWorkflowRun{RunID: "run", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "a", Batches: []types.WriteWorkflowBatch{b}}
			got, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{Action: ActionFinish, Reason: "all 90 tests passed"})
			if err != nil {
				t.Fatal(err)
			}
			message := got.ProgressLedger[len(got.ProgressLedger)-1].Message
			if !strings.Contains(message, "count unavailable") || strings.Contains(message, "0 verification") || strings.Contains(message, "90") {
				t.Fatalf("unknown count invented: %q", message)
			}
			wire, _ := json.Marshal(got)
			var restored types.WriteWorkflowRun
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			if types.NormalizeWriteWorkflowRun(restored).ProgressLedger[0].Message != message {
				t.Fatal("normalization changed durable facts")
			}
		})
	}
}
