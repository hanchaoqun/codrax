package orchestrator

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestPendingProofPlanRenamedEchoRetainsPublicProbeOnlyLane(t *testing.T) {
	mu := types.NewMutableState("verify applied target")
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: t.TempDir()}
	run := types.WriteWorkflowRun{
		RunID: "proof-identity", ActiveBatchID: "proof", Status: types.WriteWorkflowRunInProgress,
		Batches: []types.WriteWorkflowBatch{{
			ID: "proof", Goal: "verify existing target", Purpose: "verification_proof_followup",
			Status: types.WriteWorkflowBatchReadyToPlan, ExpectedPaths: []string{"pkg/target.py"},
			SuccessCriteria: []string{"symbol=Target verification_probe_required=true"}, DependsOn: []string{"applied"},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{BatchID: "applied", ReasonCode: "verification_proof_followup_requested"}},
	}
	before := run.Batches[0]
	next := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action: writeflow.ActionPlanBatch,
		Batch:  &writeflow.WriteBatchPlan{ID: "renamed", Goal: "rewrite target", Purpose: "ordinary patch", ExpectedPaths: []string{"other.js"}},
	}, &run)
	if next.Batch == nil || next.Batch.ID != before.ID || next.Batch.Purpose != before.Purpose ||
		next.Batch.Goal != before.Goal || next.Batch.ExecutionMode != before.ExecutionMode ||
		!reflect.DeepEqual(next.Batch.ExpectedPaths, before.ExpectedPaths) ||
		!reflect.DeepEqual(next.Batch.SuccessCriteria, before.SuccessCriteria) ||
		!reflect.DeepEqual(next.Batch.DependsOn, before.DependsOn) {
		t.Fatalf("pending plan lost controller identity before transition validation: %+v", next)
	}
	persisted, err := writeflow.ApplyWorkflowDecisionToRun(run, next)
	if err != nil {
		t.Fatal(err)
	}
	mu.SetWriteWorkflowRun(&persisted)
	o.seedControllerBatchPlanningHint(*next.Batch)
	if hint := mu.PlanningHint(); !strings.Contains(hint, "pkg/target.py") || strings.Contains(hint, "other.js") {
		t.Fatalf("planner saw a different scope from durable state: %s", hint)
	}
	if hint := mu.PlanningHint(); strings.Contains(hint, "verification_probe_required=true") || strings.Contains(hint, "Batch success criteria:") {
		t.Fatalf("controller injected a second copy of the current proof section: %s", hint)
	}
	blocked := writeflow.WriteWorkflowDecision{Action: writeflow.ActionBlock, ReasonCode: "explicit_block"}
	if got, ok := controllerPendingProofPlanDecision(blocked, &persisted); ok {
		t.Fatalf("explicit block was replaced with mandatory planning: %+v", got)
	}
	result, err := (&tool.EmitChangePlan{}).Execute(o.busCtx, json.RawMessage(`{
		"summary":"Verify existing target without further edits", "changes":[],
		"verification_probes":[{"id":"target-probe", "language":"python", "code":"import sys\nassert True\n", "changed_symbol_refs":["Target"]}]
	}`))
	if err != nil || !result.Success {
		t.Fatalf("public probe-only lane rejected: %v, %+v", err, result)
	}
	plan := mu.ChangePlan()
	if !types.IsPersistedProofProbeOnlyPlan(plan) || len(plan.Changes) != 0 ||
		!reflect.DeepEqual(plan.TargetPaths, before.ExpectedPaths) {
		t.Fatalf("source-free proof plan lost typed identity: %+v", plan)
	}
	// Planning/executability is not proof of a behavior contract or permission to edit source.
	if mu.ChangeReport() != nil {
		t.Fatal("planning fabricated a verification verdict")
	}
}

func TestControllerProofPlanningOwnsLongCriteriaBeforeExplorationClipping(t *testing.T) {
	// A real controller-produced row can exceed the handoff display bound.
	// Removing a duplicate after that bound cannot recover the exact row.
	criterion := "impact_obligation=verification kind=behavior_contract verification_probe_required=true evidence_ref=" + strings.Repeat("深", 160) + "/current.py:4-9"
	for _, tc := range []struct {
		name, purpose, active    string
		verifyFailure, duplicate bool
		wantInExploration        bool
	}{
		{"proof", "verification_proof_followup", "batch-1", false, false, false},
		{"impact-proof", "impact_and_verification_proof_followup", "batch-1", false, false, false},
		{"mixed-recovery", "impact_and_verification_proof_followup", "batch-1", true, false, true},
		{"ordinary", "repair", "batch-1", false, false, true},
		{"foreign", "verification_proof_followup", "other", false, false, true},
		{"ambiguous", "verification_proof_followup", "batch-1", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu := types.NewMutableState("verify existing work")
			batch := writeflow.WriteBatchPlan{ID: "batch-1", Goal: "keep exact source localization", Purpose: tc.purpose,
				ExpectedPaths: []string{"pkg/target.py"}, SuccessCriteria: []string{criterion}}
			batchRow := types.WriteWorkflowBatch{ID: batch.ID, Purpose: batch.Purpose, SuccessCriteria: batch.SuccessCriteria}
			run := &types.WriteWorkflowRun{RunID: "proof-long", ActiveBatchID: tc.active, Batches: []types.WriteWorkflowBatch{batchRow},
				ProgressLedger: []types.WriteWorkflowProgress{{BatchID: "applied", ReasonCode: "verification_proof_followup_requested"}},
				ContextPacks:   []types.WriteContextPack{testOwnerLocalizationContextPack("batch-1", "pkg/target.py", "Owner.handle")}}
			if tc.duplicate {
				run.Batches = append(run.Batches, batchRow)
			}
			mu.SetWriteWorkflowRun(run)
			if tc.verifyFailure {
				mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{})
			}
			o := New(types.PipelineSettings{}, nil, nil, nil)
			o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: t.TempDir()}
			o.seedControllerBatchPlanningHint(batch)
			o.seedControllerBatchExplorationContext(batch)
			req := mu.WriteExplorationRequest()
			if req == nil || len(req.CandidatePaths) == 0 || req.CandidatePaths[0] != "pkg/target.py" {
				t.Fatalf("lost scoped source candidates: %+v", req)
			}
			seen := strings.Join(req.EvidenceRequirements, "\n")
			if !strings.Contains(seen, "Owner.handle") {
				t.Fatalf("lost independent localization evidence: %s", seen)
			}
			if strings.Contains(seen, "impact_obligation=verification") != tc.wantInExploration ||
				strings.Contains(mu.PlanningHint(), "Batch success criteria:") != tc.wantInExploration {
				t.Fatalf("wrong current proof owner: hint=%s; exploration=%s", mu.PlanningHint(), seen)
			}
			if !reflect.DeepEqual(mu.WriteWorkflowRun().Batches[0].SuccessCriteria, []string{criterion}) {
				t.Fatal("presentation changed durable proof criteria")
			}
		})
	}
}
