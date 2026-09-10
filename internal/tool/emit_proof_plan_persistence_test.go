package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProofPlanEmittersPreserveDurableIdentityWithoutModelField(t *testing.T) {
	for _, skeleton := range []bool{false, true} {
		ctx := newTestBusCtx()
		ctx.Mode, ctx.PipelineStage = types.ModeApply, types.StagePlan
		ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{
			RunID: "wf-proof-only", ActiveBatchID: "proof-batch",
			Batches:        []types.WriteWorkflowBatch{{ID: "proof-batch", Purpose: "verification_proof_followup", ExpectedPaths: []string{"pkg/widget.py"}}},
			ProgressLedger: []types.WriteWorkflowProgress{{ReasonCode: "verification_proof_followup_requested"}},
		})
		params := json.RawMessage(`{"request":"close proof coverage","summary":"Run a bounded proof on the existing widget without modifying source files.","changes":[],"verification_probes":[{"id":"widget-proof","language":"python","code":"assert True\n"}]}`)
		var result types.ToolResult
		var err error
		if skeleton {
			result, err = (&EmitPlanSkeleton{}).Execute(ctx, params)
		} else {
			result, err = (&EmitChangePlan{}).Execute(ctx, params)
		}
		if err != nil || !result.Success {
			t.Fatalf("skeleton=%t: result=%+v error=%v", skeleton, result, err)
		}
		plan := ctx.Mutable.ChangePlan()
		if !types.IsPersistedProofProbeOnlyPlan(plan) {
			t.Fatalf("skeleton=%t: producer identity absent: %+v", skeleton, plan)
		}
		fingerprint := types.PlanFingerprint(plan)
		plan.Status = types.PlanStatusApplied
		path := filepath.Join(t.TempDir(), "plan.json")
		if err := types.WritePlanToFile(plan, path); err != nil {
			t.Fatal(err)
		}
		loaded, err := types.LoadChangePlanFromFile(path)
		if err != nil || types.PlanFingerprint(loaded) != fingerprint {
			t.Fatalf("skeleton=%t transitioned reload: %+v %v", skeleton, loaded, err)
		}
		// This is producer metadata, not a new planner field or escape hatch.
		var model map[string]any
		if err := json.Unmarshal(params, &model); err != nil {
			t.Fatal(err)
		}
		model["persistence_kind"] = types.PlanPersistenceProofProbeOnly
		forged, _ := json.Marshal(model)
		if skeleton {
			result, err = (&EmitPlanSkeleton{}).Execute(ctx, forged)
		} else {
			result, err = (&EmitChangePlan{}).Execute(ctx, forged)
		}
		if result.Success || !strings.Contains(result.Summary, `unknown field "persistence_kind"`) {
			t.Fatalf("skeleton=%t: model must not mint persistence kind: %+v %v", skeleton, result, err)
		}
	}
}
