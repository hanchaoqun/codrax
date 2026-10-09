package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Start with an actual native execution. The routing hint may offer planning,
// but must neither turn that old execution into proof nor mint a read grant.
func TestNativeRegistrationProofPlanningCapability(t *testing.T) {
	for _, condition := range []string{"available", "wrong_report", "wrong_channel", "duplicate_invocation", "unsupported_runner", "failed_report", "changed_contract", "multiple_sources", "changed_delivery", "already_attempted"} {
		t.Run(condition, func(t *testing.T) {
			f := newControllerRegistrationFixture(t)
			o, mu := f.o, f.o.busCtx.Mutable
			run := mu.WriteWorkflowRun()
			proof := &run.Batches[1]
			proof.Status = types.WriteWorkflowBatchComplete
			proof.ExecutionMode = types.WriteWorkflowBatchExecutionVerifyOnly
			proof.SuccessCriteria = []string{"kind=behavior_contract contract_ref=increment-result verification_probe_required=true"}
			proof.Attempts = []types.WriteWorkflowAttempt{{Kind: "verify", Status: "unverified", ReasonCode: "verification_proof_incomplete", PlanID: f.source.ID}}
			report := mu.ChangeReport()
			switch condition {
			case "wrong_report":
				report.PlanID = "another-source"
			case "wrong_channel":
				report.Channel = types.ChangeReportChannelPlannerProbe
			case "duplicate_invocation":
				report.ExecutedCommands = append(report.ExecutedCommands, report.ExecutedCommands...)
			case "unsupported_runner":
				for i := range report.ExecutedCommands {
					report.ExecutedCommands[i].Framework = "other"
				}
			case "failed_report":
				report.Passed, report.VerificationStatus = false, types.VerificationStatusFailed
			case "changed_contract":
				ir := mu.WriteAnalysisIR()
				ir.Request.BehaviorContracts[0].Expected = "increment(4) == 99"
				mu.SetWriteAnalysisIR(ir)
			case "multiple_sources":
				run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: "another-source", PlanID: "another-source", Status: types.WriteWorkflowBatchComplete, Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "another-source"}}})
			case "changed_delivery":
				plan := mu.ChangePlan()
				plan.AppliedCommitSHA = "1111111111111111111111111111111111111111"
				mu.SetChangePlan(plan)
			case "already_attempted":
				run.ProgressLedger = append(run.ProgressLedger, types.WriteWorkflowProgress{ReasonCode: "verification_proof_probe_plan_requested"})
			}
			mu.SetChangeReport(report)
			mu.SetWriteWorkflowRun(run)
			plan := mu.ChangePlan()
			before, _ := json.Marshal([]any{plan, report, types.BuildVerificationProofLedger(plan, report, nil)})
			// A bad report may independently leave changed-target execution
			// debt. Preserve that existing generic route, without mistaking it
			// for native registration capability.
			_, genericRoute := verificationProofProbePlanningFollowupDecisionWithRuntimeAvailability(run, plan, report, func(string) bool { return true })
			identityAvailable := types.NativeTestRegistrationSourceIdentitiesAvailable(f.source.ID, report)
			if (condition == "wrong_report" || condition == "wrong_channel" || condition == "duplicate_invocation" || condition == "unsupported_runner") && identityAvailable {
				t.Fatal("invalid source native identity offered registration")
			}
			next := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish, FinishDisposition: writeflow.FinishDispositionAllVerified}, run)
			if got := next.Action == writeflow.ActionExploreCode; got != (condition == "available" || genericRoute) {
				t.Fatalf("registration routing=%t: %+v", got, next)
			}
			after, _ := json.Marshal([]any{plan, report, types.BuildVerificationProofLedger(plan, report, nil)})
			if string(before) != string(after) || mu.NativeTestRegistrationAuthorization() != nil || mu.NativeTestRegistrationExecutionAuthorized(plan, o.busCtx.RepoRoot) || len(plan.ProjectTestObservations) != 0 {
				t.Fatal("planning capability changed proof or granted registration authority")
			}
		})
	}
}
