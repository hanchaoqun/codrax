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
