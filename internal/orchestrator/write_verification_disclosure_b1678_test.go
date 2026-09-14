package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestB1678MissingExecutionProofIsNotProofOfStaticOnly(t *testing.T) {
	for _, capability := range []types.VerificationCapability{types.VerificationCapabilityUnknown, types.VerificationCapabilitySourceStatic, types.VerificationCapabilitySyntaxOnly} {
		t.Run(string(capability), func(t *testing.T) {
			// A family without a direct inline runtime reaches the existing
			// unverified terminal lane, rather than first scheduling a probe.
			plan := &types.ChangePlan{ID: "plan-b1678-disclosure", Status: types.PlanStatusApplied, TargetPaths: []string{"src/widget.rs"}}
			report := &types.ChangeReport{
				PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true, VerificationStatus: types.VerificationStatusPassed,
				TestResults:         []types.TestResult{{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAggregate, AssertionID: "make-test", Suite: "check", Passed: true}},
				ExecutedCommands:    []types.ExecutedCommand{{Runner: "make", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 0, WorkingDir: ".", Suite: "check", CoveredPaths: []string{"src/widget.rs"}}},
				ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{Path: "src/widget.rs", Status: types.ChangedPathVerificationCovered, Caliber: types.ChangedPathVerificationProjectRunner, Capability: capability, Runner: "make"}},
			}
			before, _ := json.Marshal(report)
			mu := types.NewMutableState("missing proof must not become a negative execution observation")
			mu.SetChangePlan(plan)
			mu.SetChangeReport(report)
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
			run := &types.WriteWorkflowRun{
				RunID: "wf-b1678", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "batch-1",
				Batches: []types.WriteWorkflowBatch{{ID: "batch-1", PlanID: plan.ID, Status: types.WriteWorkflowBatchComplete,
					Completion: &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified, ReasonCode: "tests_passed"},
					Attempts:   []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: plan.ID}, {Kind: "verify", Status: "passed", PlanID: plan.ID}},
				}},
			}
			decision := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish, FinishDisposition: writeflow.FinishDispositionAllVerified}, run)
			if decision.Action != writeflow.ActionFinish || decision.FinishDisposition != writeflow.FinishDispositionAcceptUnverified || decision.ReasonCode != "production_verification_source_static_only" {
				t.Fatalf("existing completion/proof boundary changed: %+v", decision)
			}
			if !strings.Contains(decision.Reason, "target execution has not been established") || strings.Contains(decision.Reason, "was not executed") || strings.Contains(decision.Reason, "static checks passed") {
				t.Errorf("unknown proof was converted into a negative execution fact: %s", decision.Reason)
			}
			finished, err := writeflow.ApplyWorkflowDecisionToRun(*run, decision)
			if err != nil {
				t.Fatal(err)
			}
			for _, language := range []string{"zh", "en"} {
				text := renderWriteWorkflowTerminalStatus(finished, language)
				want := "尚无生产目标已执行的验证凭证"
				if language == "en" {
					want = "execution of the production target has not been established"
				}
				if !strings.Contains(text, want) || strings.Contains(text, "只有静态证据") || strings.Contains(text, "static evidence only") || strings.Contains(text, "production_verification_source_static_only") {
					t.Errorf("%s terminal scope disclosure is inaccurate: %s", language, text)
				}
			}
			after, _ := json.Marshal(report)
			if string(before) != string(after) {
				t.Error("disclosure normalization rewrote original report values")
			}
		})
	}
}
