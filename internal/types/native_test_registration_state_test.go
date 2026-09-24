package types

import "testing"

// This is a state-boundary fixture, not a producer/execution claim. Real
// controller/read/model/native execution is covered in the public suites.
func TestNativeRegistrationExecutionGrantDoesNotSurviveReset(t *testing.T) {
	for _, reset := range []string{"run", "nil_run", "nil_analysis", "different_plan", "blocked_run"} {
		t.Run(reset, func(t *testing.T) {
			p, _ := nativeRegistrationProofFixture(t)
			m := NewMutableState("grant lifecycle")
			r := &WriteWorkflowRun{RunID: "run", ActiveBatchID: "batch", Status: WriteWorkflowRunInProgress,
				NativeTestRegistrations: []NativeTestRegistrationRecord{{PlanID: p.ID, Digest: NativeTestRegistrationDigest(p)}},
				Batches:                 []WriteWorkflowBatch{{ID: "batch", PlanID: p.ID, Purpose: "verification_proof_followup", ExecutionMode: WriteWorkflowBatchExecutionVerifyOnly, Status: WriteWorkflowBatchVerifying}}}
			m.SetChangePlan(p)
			m.SetWriteWorkflowRun(r)
			if err := m.AuthorizeNativeTestRegistrationExecution(p, "/repo", p.NativeTestRegistration.Delivery, p.BehaviorContracts); err != nil {
				t.Fatal(err)
			}
			if !m.NativeTestRegistrationExecutionAuthorized(p, "/repo") {
				t.Fatal("fresh current grant missing")
			}
			switch reset {
			case "run":
				m.ResetWriteWorkflowRun()
				m.SetWriteWorkflowRun(r)
			case "nil_run":
				m.SetWriteWorkflowRun(nil)
				m.SetWriteWorkflowRun(r)
			case "nil_analysis":
				m.SetWriteAnalysisIR(nil)
			case "different_plan":
				m.SetChangePlan(&ChangePlan{ID: "other"})
			case "blocked_run":
				r.Status = WriteWorkflowRunBlocked
				m.SetWriteWorkflowRun(r)
			}
			if m.NativeTestRegistrationExecutionAuthorized(p, "/repo") {
				t.Fatal("stale execution grant survived context reset/change")
			}
		})
	}
}

func TestNativeRegistrationRequiredExecutionRejectsLegacyInvocation(t *testing.T) {
	p, r := nativeRegistrationProofFixture(t)
	p.WriteAnalysisIR = &WriteAnalysisIR{Request: WriteRequestModel{Constraints: []WriteConstraint{{Kind: WriteConstraintRunExistingTest, Target: p.NativeTestRegistration.Tests[0].Path}}}}
	sealNativeTestRegistration(p)
	r.ExistingTestExecutions[0].NativeTestRegistrationDigest = NativeTestRegistrationDigest(p)
	if records := ExistingTestExecutionConfidence(p, r); len(records) != 1 || records[0].Status != "satisfied" {
		t.Fatalf("fresh required execution missing: %+v", records)
	}
	r.ExecutedCommands[0].InvocationID = ""
	r.TestResults[0].InvocationID = ""
	if records := ExistingTestExecutionConfidence(p, r); len(records) != 1 || records[0].Status == "satisfied" {
		t.Fatalf("legacy invocation borrowed new required execution: %+v", records)
	}
}
