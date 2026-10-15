package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestHMC244WorkflowFinishRestoreDisplaysFactsNotModelAdvisory(t *testing.T) {
	attempt := types.WriteWorkflowAttempt{ID: "attempt-1", Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "p244", ReportID: "p244.report.json", ArtifactRef: "p244.report.json"}
	report := &types.ChangeReport{PlanID: "p244", Channel: types.ChangeReportChannelPostApplyVerify, Passed: true,
		TestResults: []types.TestResult{{Passed: true}, {Passed: true}, {Passed: true}, {Passed: true}}}
	attempt.VerificationResults = types.BindWriteWorkflowVerificationResults("p244", attempt, report)
	run := types.WriteWorkflowRun{RunID: "wf244", Goal: "fix", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "batch", Batches: []types.WriteWorkflowBatch{{ID: "batch", PlanID: "p244", VerifyRef: "p244.report.json", Status: types.WriteWorkflowBatchVerifying, Attempts: []types.WriteWorkflowAttempt{attempt}}}}
	const advisory = "test_widget.py全部4个测试通过"
	finished, err := writeflow.ApplyWorkflowDecisionToRun(run, writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish, Reason: advisory})
	if err != nil {
		t.Fatal(err)
	}
	storeRoot := t.TempDir()
	store := NewWriteWorkflowRunStore(storeRoot)
	if _, err = store.Save(&finished); err != nil {
		t.Fatal(err)
	}
	// Use a fresh reader instance, as the REPL restart path does.
	restored, err := NewWriteWorkflowRunStore(storeRoot).Load("wf244")
	if err != nil {
		t.Fatal(err)
	}
	last := restored.ProgressLedger[len(restored.ProgressLedger)-1]
	if last.AdvisoryMessage != advisory || strings.Contains(last.Message, advisory) {
		t.Fatalf("audit/default facts not separated: %+v", last)
	}
	for _, lang := range []string{"zh", "en"} {
		var out strings.Builder
		writeWorkflowProgressLines(&out, lang, *restored)
		if strings.Contains(out.String(), advisory) || !strings.Contains(out.String(), "4 verification result(s)") {
			t.Fatalf("restart replayed model claim: %s", out.String())
		}
	}
	if restored.Completion == nil || restored.Completion.Verdict != types.WriteWorkflowCompletionVerified {
		t.Fatal("display changed finish verdict")
	}
}
