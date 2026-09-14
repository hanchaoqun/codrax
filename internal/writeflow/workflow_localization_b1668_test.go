package writeflow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1668LocalizationInput() (types.WriteWorkflowRun, *types.ChangePlan) {
	plan := &types.ChangePlan{ID: "active-plan", TargetPaths: []string{"src/owner.c"}}
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchComplete, plan.ID)
	run.ContextPacks = []types.WriteContextPack{{BatchID: "batch-1", Items: []types.WriteContextItem{
		{Kind: "localization_anchor", LocalizationAnchor: &types.SourceLocalizationAnchor{
			Path: "src/owner.c", Role: types.SourcePathRoleProduction,
			Kind: types.SourceLocalizationAnchorGroundedEvidence, Strength: types.SourceLocalizationAnchorOwner,
		}},
		{Kind: "localization_anchor", LocalizationAnchor: &types.SourceLocalizationAnchor{
			Path: "neighbor.c", Role: types.SourcePathRoleProduction,
			Kind: types.SourceLocalizationAnchorReadFile, Strength: types.SourceLocalizationAnchorObserved,
		}},
	}}}
	return run, plan
}

func TestB1668WorkflowLocalizationUsesBoundDeliveryDomain(t *testing.T) {
	for _, observed := range []string{"test_repository.c", "nearby_module.rs", "support/adapter.py"} {
		t.Run(observed, func(t *testing.T) {
			run, plan := b1668LocalizationInput()
			run.ContextPacks[0].Items[1].LocalizationAnchor.Path = observed
			before, _ := json.Marshal(struct {
				Run  types.WriteWorkflowRun
				Plan *types.ChangePlan
			}{run, plan})
			view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
			if view.Localization.State != loopkernel.LocalizationAuthorityOwnerSupported ||
				view.Localization.RequiresMoreContext || !reflect.DeepEqual(view.Localization.SourcePaths, []string{"src/owner.c"}) {
				t.Fatalf("observed context became delivery obligation: %+v", view.Localization)
			}
			after, _ := json.Marshal(struct {
				Run  types.WriteWorkflowRun
				Plan *types.ChangePlan
			}{run, plan})
			if string(before) != string(after) {
				t.Fatal("read-only view changed plan/workflow")
			}
		})
	}
}

func TestB1668WorkflowLocalizationKeepsRequiredMissingPaths(t *testing.T) {
	for _, source := range []string{"expected", "scope_anchor", "change", "rename", "applied", "cumulative"} {
		t.Run(source, func(t *testing.T) {
			run, plan := b1668LocalizationInput()
			missing := "required.c"
			switch source {
			case "expected":
				run.Batches[0].ExpectedPaths = []string{missing}
			case "scope_anchor":
				run.ContextPacks[0].Items = append(run.ContextPacks[0].Items, types.WriteContextItem{
					Kind: "localization_anchor", LocalizationAnchor: &types.SourceLocalizationAnchor{
						Path: missing, Role: types.SourcePathRoleProduction,
						Kind: types.SourceLocalizationAnchorScope, Strength: types.SourceLocalizationAnchorSupporting,
					},
				})
			case "change":
				plan.Changes = []types.FileChange{{Path: missing, Kind: "patch"}}
			case "rename":
				plan.Changes = []types.FileChange{{Path: "src/owner.c", NewPath: missing, Kind: "rename"}}
			case "applied":
				plan.AppliedPaths = []string{missing}
			case "cumulative":
				run.Batches[0].Attempts = []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "old-plan"}}
				plan.CumulativeVerificationScope = &types.CumulativeVerificationScope{
					SourcePlanIDs: []string{"old-plan"}, TargetPaths: []string{missing},
				}
			}
			view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
			if !view.Localization.RequiresMoreContext ||
				!reflect.DeepEqual(view.Localization.OwnerMissingPaths, []string{missing}) {
				t.Fatalf("required source lost or observed neighbor promoted: %+v", view.Localization)
			}
		})
	}
}

func TestB1668WorkflowLocalizationUnknownHistoryCannotUseSupportedPlanFallback(t *testing.T) {
	for _, keepCurrentOwner := range []bool{false, true} {
		t.Run(fmt.Sprint(keepCurrentOwner), func(t *testing.T) {
			run, plan := b1668LocalizationInput()
			run.ContextPacks[0].Items = run.ContextPacks[0].Items[:1]
			if !keepCurrentOwner {
				run.ContextPacks = nil
			}
			run.Batches[0].Attempts = []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "unknown-old-plan"}}
			plan.LocalizationReview = &types.SourceLocalizationReview{PlanID: plan.ID, Status: types.SourceLocalizationSupported,
				SourcePaths: []string{"src/owner.c"}, OwnerSupportedPaths: []string{"src/owner.c"}}
			view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
			if !view.Localization.RequiresMoreContext || !view.LocalizationGateEligible || view.Localization.State == loopkernel.LocalizationAuthorityOwnerSupported {
				t.Fatalf("missing historical domain laundered by current plan or current owner: %+v", view.Localization)
			}
		})
	}
}

func TestB1668WorkflowLocalizationUnknownHistoryDoesNotShrink(t *testing.T) {
	for _, source := range []string{"old_apply", "other_batch_apply", "unknown_apply_id", "partial_apply", "foreign_plan", "missing_batch_plan", "missing_plan_id", "empty_cumulative_paths"} {
		t.Run(source, func(t *testing.T) {
			run, plan := b1668LocalizationInput()
			switch source {
			case "old_apply":
				run.Batches[0].Attempts = []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "old-plan"}}
			case "other_batch_apply":
				run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: "older-batch", Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "old-plan"}}})
			case "unknown_apply_id":
				run.Batches[0].Attempts = []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied"}}
			case "partial_apply":
				run.Batches[0].Attempts = []types.WriteWorkflowAttempt{{Kind: "apply", Status: "partially_applied", PlanID: "old-plan"}}
			case "foreign_plan":
				plan.ID = "foreign-plan"
			case "missing_batch_plan":
				run.Batches[0].PlanID = ""
			case "missing_plan_id":
				plan.ID = ""
			case "empty_cumulative_paths":
				run.Batches[0].Attempts = []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "old-plan"}}
				plan.CumulativeVerificationScope = &types.CumulativeVerificationScope{SourcePlanIDs: []string{"old-plan"}}
			}
			view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
			if !view.Localization.RequiresMoreContext || !reflect.DeepEqual(view.Localization.OwnerMissingPaths, []string{"neighbor.c"}) {
				t.Fatalf("unknown source authority must retain conservative legacy projection: %+v", view.Localization)
			}
		})
	}
}

func TestB1668WorkflowLocalizationDoesNotBorrowForeignOwnerOrRewriteProof(t *testing.T) {
	run, plan := b1668LocalizationInput()
	plan.CumulativeVerificationScope = &types.CumulativeVerificationScope{SourcePlanIDs: []string{"old-plan"}, TargetPaths: []string{"old.c"}}
	run.Batches[0].Attempts = []types.WriteWorkflowAttempt{
		{Kind: "apply", Status: "applied", PlanID: "old-plan"},
		{Kind: "verify", Status: "unverified", ReasonCode: "runner_missing", PlanID: plan.ID},
	}
	run.ContextPacks = append(run.ContextPacks, types.WriteContextPack{BatchID: "other-batch", Items: []types.WriteContextItem{{
		Kind: "localization_anchor", LocalizationAnchor: &types.SourceLocalizationAnchor{
			Path: "old.c", Role: types.SourcePathRoleProduction,
			Kind: types.SourceLocalizationAnchorGroundedEvidence, Strength: types.SourceLocalizationAnchorOwner,
		},
	}}})
	plan.LocalizationReview = &types.SourceLocalizationReview{PlanID: plan.ID, Status: types.SourceLocalizationSupported,
		SourcePaths: []string{"src/owner.c"}, OwnerSupportedPaths: []string{"src/owner.c"}}
	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if !reflect.DeepEqual(view.Localization.OwnerMissingPaths, []string{"old.c"}) {
		t.Fatalf("current supported review or foreign batch laundered old missing source: %+v", view.Localization)
	}
	if view.Proof.State != loopkernel.ProofCoverageUnavailable {
		t.Fatalf("localization projection changed proof authority: %+v", view.Proof)
	}
	if !reflect.DeepEqual(plan.TargetPaths, []string{"src/owner.c"}) || len(plan.Changes) != 0 {
		t.Fatal("verification/localization scope must not widen apply")
	}
}

func TestB1668WorkflowLocalizationKeepsScopeBeyondAnchorPreview(t *testing.T) {
	run, plan := b1668LocalizationInput()
	for i := 0; i < 70; i++ {
		run.ContextPacks[0].Items = append(run.ContextPacks[0].Items, types.WriteContextItem{
			Kind: "localization_anchor", LocalizationAnchor: &types.SourceLocalizationAnchor{
				Path: fmt.Sprintf("a%02d.c", i), Role: types.SourcePathRoleProduction,
				Kind: types.SourceLocalizationAnchorReadFile, Strength: types.SourceLocalizationAnchorObserved,
			},
		})
	}
	run.ContextPacks[0].Items = append(run.ContextPacks[0].Items, types.WriteContextItem{
		Kind: "localization_anchor", LocalizationAnchor: &types.SourceLocalizationAnchor{
			Path: "zzz_required.c", Role: types.SourcePathRoleProduction,
			Kind: types.SourceLocalizationAnchorScope, Strength: types.SourceLocalizationAnchorSupporting,
		},
	})
	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if !reflect.DeepEqual(view.Localization.OwnerMissingPaths, []string{"zzz_required.c"}) {
		t.Fatalf("bounded anchor preview must not erase typed scope: %+v", view.Localization)
	}
}

func TestB1668WorkflowLocalizationRetainsObservedPriorContext(t *testing.T) {
	run, plan := b1668LocalizationInput()
	review, bound := loopkernel.LocalizationReviewFromWriteWorkflowPlan(run, "batch-1", plan)
	if !bound || !reflect.DeepEqual(review.PriorContextPaths, []string{"neighbor.c", "src/owner.c"}) {
		t.Fatalf("observed paths must remain available as prior context: %+v, bound=%t", review, bound)
	}
}

func TestB1668WorkflowLocalizationPendingOtherBatchDoesNotBecomeCurrentDelivery(t *testing.T) {
	run, plan := b1668LocalizationInput()
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: "next-batch",
		Status: types.WriteWorkflowBatchNeedsExploration, ExpectedPaths: []string{"future.c"}})
	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if !reflect.DeepEqual(view.Localization.SourcePaths, []string{"src/owner.c"}) || view.Localization.RequiresMoreContext {
		t.Fatalf("unapplied future batch was folded into current delivery localization: %+v", view.Localization)
	}
	if run.Batches[1].Status != types.WriteWorkflowBatchNeedsExploration {
		t.Fatal("current projection must not settle a pending batch")
	}
}

func TestB1668WorkflowLocalizationDoesNotBorrowForeignPlanOwnerAnchors(t *testing.T) {
	run, plan := b1668LocalizationInput()
	run.ContextPacks[0].Items = run.ContextPacks[0].Items[1:]
	plan.LocalizationReview = &types.SourceLocalizationReview{PlanID: "foreign-plan", Status: types.SourceLocalizationSupported,
		OwnerSupportedPaths: []string{"src/owner.c"}, Anchors: []types.SourceLocalizationAnchor{{
			Path: "src/owner.c", Kind: types.SourceLocalizationAnchorGroundedEvidence,
			Strength: types.SourceLocalizationAnchorOwner, Role: types.SourcePathRoleProduction,
		}}}
	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if !reflect.DeepEqual(view.Localization.OwnerMissingPaths, []string{"src/owner.c"}) || !view.Localization.RequiresMoreContext {
		t.Fatalf("foreign plan owner became current authority: %+v", view.Localization)
	}
}
