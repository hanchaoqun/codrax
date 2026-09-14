package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Exercise the same normalize -> transition-recovery boundary as dispatch,
// rather than only checking the shape of the first controller override.
func TestB1668CompletedLocalizationRecoveryIsLegalAndHonest(t *testing.T) {
	for _, action := range []writeflow.WorkflowAction{writeflow.ActionFinish, writeflow.ActionExploreCode, writeflow.ActionVerifyBatch} {
		t.Run(string(action), func(t *testing.T) {
			mu := types.NewMutableState("bounded localization recovery")
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
			run := b1668CompletedRun()
			before := writeflow.DeriveWorkflowExecutionView(types.ModeApply, *run, nil)
			if !before.Localization.RequiresMoreContext || !before.LocalizationGateEligible {
				t.Fatalf("fixture must contain a real typed localization gap: %+v", before)
			}
			decision := writeflow.WriteWorkflowDecision{Action: action}
			if action == writeflow.ActionFinish {
				decision.FinishDisposition = writeflow.FinishDispositionAllVerified
			}
			if action == writeflow.ActionExploreCode {
				decision.ExplorationRequest = &types.WriteExplorationRequest{BatchID: "batch-1", Goal: "locate owner"}
			}
			got := o.enforceControllerWorkflowTransition(o.normalizeControllerTypedStateDecision(decision, run), run)
			view := writeflow.DeriveWorkflowExecutionView(types.ModeApply, *run, nil)
			if check := writeflow.ValidateWorkflowTransition(view, got); !check.Allowed {
				t.Fatalf("recovery emitted an impossible action: %+v; check=%+v", got, check)
			}
			if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
				t.Fatalf("completed same-batch localization must remain honestly unverified: %+v", got)
			}
			if run.Batches[0].Completion == nil || run.Batches[0].Completion.Verdict != types.WriteWorkflowCompletionUnverified {
				t.Fatalf("unresolved localization was not retained durably: %+v", run.Batches[0].Completion)
			}
			again := o.enforceControllerWorkflowTransition(o.normalizeControllerTypedStateDecision(got, run), run)
			if again.Action != got.Action || again.FinishDisposition != got.FinishDisposition {
				t.Fatalf("same-generation recovery is not stable: %+v -> %+v", got, again)
			}
		})
	}
}

func b1668CompletedRun() *types.WriteWorkflowRun {
	return &types.WriteWorkflowRun{
		RunID: "wf-b1668", Goal: "locate owner", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{ID: "batch-1", Goal: "locate owner", Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified},
			Attempts:   []types.WriteWorkflowAttempt{{Kind: "verify", Status: "passed", ReasonCode: "tests_passed"}},
		}},
		ContextPacks: []types.WriteContextPack{{BatchID: "batch-1", Items: []types.WriteContextItem{{
			Kind: "localization_anchor", Priority: types.WriteContextP1,
			LocalizationAnchor: &types.SourceLocalizationAnchor{Path: "src/owner.c", Role: types.SourcePathRoleProduction,
				Kind: types.SourceLocalizationAnchorReadFile, Strength: types.SourceLocalizationAnchorObserved},
		}}}},
	}
}

func TestB1668TruthOverridesRespectStateKernel(t *testing.T) {
	for _, state := range []writeflow.WorkflowExecutionState{writeflow.WorkflowExecutionReadyToPlan, writeflow.WorkflowExecutionNeedsReplan,
		writeflow.WorkflowExecutionObserveRequired, writeflow.WorkflowExecutionComplete} {
		for _, ledger := range []types.TruthLedger{
			{State: types.TruthLedgerWeak, RecommendedAction: types.TruthActionLocalize, ReasonCode: "localization_owner_partial"},
			{State: types.TruthLedgerWeak, RecommendedAction: types.TruthActionRepair, ReasonCode: "repair_needed"},
			{State: types.TruthLedgerFailed, RecommendedAction: types.TruthActionRepair, ReasonCode: "tests_failed"},
		} {
			t.Run(string(state)+"/"+string(ledger.State)+"/"+string(ledger.RecommendedAction), func(t *testing.T) {
				view := writeflow.WorkflowExecutionView{Mode: types.ModeApply, State: state, BatchID: "batch-1",
					RunStatus: types.WriteWorkflowRunInProgress, LocalizationGateEligible: true,
					Localization: loopkernel.LocalizationAuthorityView{RequiresMoreContext: true, SourcePaths: []string{"src/owner.c"}},
				}
				got, ok := controllerTruthLedgerDecisionFromView(writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish}, view, ledger, b1668CompletedRun())
				if ok {
					if check := writeflow.ValidateWorkflowTransition(view, got); !check.Allowed {
						t.Fatalf("typed truth override disagrees with state kernel: %+v / %+v", got, check)
					}
					if ledger.State == types.TruthLedgerFailed && got.Action == writeflow.ActionFinish {
						t.Fatalf("failure must not become verified finish: %+v", got)
					}
				}
			})
		}
	}
}

func TestB1668CompletedExplicitNextBatchStillAllowed(t *testing.T) {
	run := b1668CompletedRun()
	mu := types.NewMutableState("next batch")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	decision := writeflow.WriteWorkflowDecision{Action: writeflow.ActionExploreCode,
		Batch:              &writeflow.WriteBatchPlan{ID: "batch-2", Goal: "next distinct work"},
		ExplorationRequest: &types.WriteExplorationRequest{BatchID: "batch-2", Goal: "next distinct work"},
	}
	got := o.enforceControllerWorkflowTransition(o.normalizeControllerTypedStateDecision(decision, run), run)
	if got.Action != writeflow.ActionExploreCode || got.Batch == nil || got.Batch.ID != "batch-2" {
		t.Fatalf("legitimate next batch was lost: %+v", got)
	}
}

func TestB1668KernelPlanRecommendationIsRecoverable(t *testing.T) {
	for _, action := range []writeflow.WorkflowAction{writeflow.ActionApplyPlan, writeflow.ActionVerifyBatch} {
		t.Run(string(action), func(t *testing.T) {
			run := b1668CompletedRun()
			run.Batches[0].Status = types.WriteWorkflowBatchReadyToPlan
			run.Batches[0].Attempts = nil
			run.ContextPacks = nil
			mu := types.NewMutableState("recover plan prerequisite")
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
			got := o.enforceControllerWorkflowTransition(writeflow.WriteWorkflowDecision{Action: action}, run)
			if got.Action != writeflow.ActionPlanBatch || got.Batch == nil || got.Batch.ID != "batch-1" {
				t.Fatalf("kernel plan prerequisite was lost: %+v", got)
			}
			view := writeflow.DeriveWorkflowExecutionView(types.ModeApply, *run, nil)
			if check := writeflow.ValidateWorkflowTransition(view, got); !check.Allowed {
				t.Fatalf("recovered plan is illegal: %+v", check)
			}
		})
	}
}

func TestB1668RecoveryRevalidatesAfterOtherAuthorityNormalizer(t *testing.T) {
	// A blocked verify-only batch is a defensive persisted-state shape. The
	// verify-only normalizer can reinterpret the kernel's AskUser recovery as
	// VerifyBatch; the final transition check must still prevent execution.
	run := b1668CompletedRun()
	run.Batches[0].Status = types.WriteWorkflowBatchBlocked
	run.Batches[0].ExecutionMode = types.WriteWorkflowBatchExecutionVerifyOnly
	mu := types.NewMutableState("blocked verify-only recovery")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	got := o.enforceControllerWorkflowTransition(writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish}, run)
	if got.Action != writeflow.ActionBlock || got.ReasonCode != "workflow_transition_recovery_conflict" {
		t.Fatalf("post-normalization transition must fail closed: %+v", got)
	}
	view := writeflow.DeriveWorkflowExecutionView(types.ModeApply, *run, nil)
	if check := writeflow.ValidateWorkflowTransition(view, got); !check.Allowed {
		t.Fatalf("last-line recovery itself is illegal: %+v", check)
	}
}

func TestB1668QualifiedDeliveryFinishesWithoutObservedNeighborRetry(t *testing.T) {
	run := b1668CompletedRun()
	run.Batches[0].PlanID = "plan-current"
	run.Batches[0].Attempts = []types.WriteWorkflowAttempt{
		{Kind: "apply", Status: "applied", PlanID: "plan-previous"},
		{Kind: "apply", Status: "applied", PlanID: "plan-current"},
		{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "plan-current"},
	}
	run.ContextPacks[0].Items[0].LocalizationAnchor.Strength = types.SourceLocalizationAnchorOwner
	run.ContextPacks[0].Items[0].LocalizationAnchor.Kind = types.SourceLocalizationAnchorGroundedEvidence
	run.ContextPacks[0].Items = append(run.ContextPacks[0].Items, types.WriteContextItem{
		Kind: "localization_anchor", Priority: types.WriteContextP1,
		LocalizationAnchor: &types.SourceLocalizationAnchor{Path: "neighbor.c", Role: types.SourcePathRoleProduction,
			Kind: types.SourceLocalizationAnchorReadFile, Strength: types.SourceLocalizationAnchorObserved},
	})
	plan := &types.ChangePlan{ID: "plan-current", Status: types.PlanStatusApplied, TargetPaths: []string{"src/owner.c"},
		CumulativeVerificationScope: &types.CumulativeVerificationScope{SourcePlanIDs: []string{"plan-previous"}, TargetPaths: []string{"src/owner.c"}},
	}
	report := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify,
		Passed: true, VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, Suite: "native", AssertionID: "TestOwner", Passed: true}},
		ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{Path: "src/owner.c", Status: types.ChangedPathVerificationCovered,
			Caliber: types.ChangedPathVerificationProjectRunner, Capability: types.VerificationCapabilityTargetBehavior}},
	}
	mu := types.NewMutableState("qualified delivery, unrelated observation")
	mu.SetChangePlan(plan)
	mu.SetChangeReport(report)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	decision := writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish, FinishDisposition: writeflow.FinishDispositionAllVerified}
	got := o.enforceControllerWorkflowTransition(o.normalizeControllerTypedStateDecision(decision, run), run)
	if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAllVerified {
		t.Fatalf("unrelated observation changed qualified completion: %+v", got)
	}
	if workflowProgressHasReason(run.ProgressLedger, "workflow_transition_rejected") {
		t.Fatalf("system unnecessarily rejected its own completion: %+v", run.ProgressLedger)
	}
}

func TestB1668UnknownAppliedHistoryCannotFinishVerifiedWithoutContext(t *testing.T) {
	run := b1668CompletedRun()
	run.ContextPacks = nil
	run.Batches[0].PlanID = "current"
	run.Batches[0].Attempts = []types.WriteWorkflowAttempt{
		{Kind: "apply", Status: "applied", PlanID: "old-unavailable"},
		{Kind: "verify", Status: "passed", PlanID: "current"},
	}
	plan := &types.ChangePlan{ID: "current", Status: types.PlanStatusApplied, TargetPaths: []string{"src/owner.c"},
		LocalizationReview: &types.SourceLocalizationReview{PlanID: "current", Status: types.SourceLocalizationSupported},
	}
	mu := types.NewMutableState("unknown retained delivery")
	mu.SetChangePlan(plan)
	mu.SetChangeReport(&types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify,
		Passed: true, VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, Suite: "native", AssertionID: "TestOwner", Passed: true}},
		ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{Path: "src/owner.c", Status: types.ChangedPathVerificationCovered,
			Caliber: types.ChangedPathVerificationProjectRunner, Capability: types.VerificationCapabilityTargetBehavior}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	got := o.enforceControllerWorkflowTransition(o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action: writeflow.ActionFinish, FinishDisposition: writeflow.FinishDispositionAllVerified,
	}, run), run)
	if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
		t.Fatalf("unavailable historical delivery scope was signed verified: %+v", got)
	}
}
