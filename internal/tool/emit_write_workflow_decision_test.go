package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestEmitWriteWorkflowDecisionStoresNormalizedTypedJSON(t *testing.T) {
	mut := types.NewMutableState("change request")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitWriteWorkflowDecision{}

	res, err := tool.Execute(bus, json.RawMessage(`{
		"action": "explore_code",
		"reason_code": " need_context ",
		"exploration_request": {
			"batch_id": " batch-1 ",
			"goal": " inspect planner ",
			"exploration_questions": "where is retry context consumed, where are approvals gated",
			"candidate_paths": "internal/agent/planner.go, internal/orchestrator/stage_hooks.go"
		}
	}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute rejected valid decision: %s", res.Summary)
	}
	raw := mut.WriteWorkflowDecisionJSON()
	if len(raw) == 0 {
		t.Fatal("decision JSON not stored")
	}
	var got writeflow.WriteWorkflowDecision
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("stored decision must unmarshal: %v", err)
	}
	if got.Action != writeflow.ActionExploreCode || got.ReasonCode != "need_context" {
		t.Fatalf("unexpected decision: %+v", got)
	}
	if got.ExplorationRequest == nil || got.ExplorationRequest.BatchID != "batch-1" {
		t.Fatalf("exploration request not normalized: %+v", got.ExplorationRequest)
	}
	if len(got.ExplorationRequest.CandidatePaths) != 2 {
		t.Fatalf("candidate paths not schema-repaired: %+v", got.ExplorationRequest.CandidatePaths)
	}
}

func TestEmitWriteWorkflowDecisionRejectsMissingTypedPayload(t *testing.T) {
	mut := types.NewMutableState("change request")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitWriteWorkflowDecision{}

	res, err := tool.Execute(bus, json.RawMessage(`{
		"action": "explore_code",
		"reason": "please explore the code"
	}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected missing exploration_request rejection")
	}
	if !strings.Contains(res.Summary, "exploration_request") {
		t.Fatalf("rejection should name missing typed payload, got %q", res.Summary)
	}
	if len(mut.WriteWorkflowDecisionJSON()) != 0 {
		t.Fatal("rejected decision must not be stored")
	}
}

func TestEmitWriteWorkflowDecision_ModePlanMasksApplyVerify(t *testing.T) {
	tl := &EmitWriteWorkflowDecision{}

	schema := string(tl.ParametersFor(&types.AgentContext{Mode: types.ModePlan}))
	if strings.Contains(schema, `"apply_plan"`) || strings.Contains(schema, `"verify_batch"`) {
		t.Fatalf("ModePlan dispatch schema must not offer apply/verify actions: %s", schema)
	}
	full := string(tl.ParametersFor(&types.AgentContext{Mode: types.ModeApply}))
	if !strings.Contains(full, `"apply_plan"`) {
		t.Fatalf("ModeApply dispatch schema must keep apply_plan: %s", full)
	}

	mu := types.NewMutableState("mask")
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModePlan}
	res, err := tl.Execute(ctx, json.RawMessage(`{"action":"apply_plan"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success {
		t.Fatalf("apply_plan in ModePlan must be rejected, got %+v", res)
	}
	if res.Repair == nil || res.Repair.Code != "workflow_action_not_in_mode" {
		t.Fatalf("rejection must carry the typed repair code, got %+v", res.Repair)
	}
	if len(mu.WriteWorkflowDecisionJSON()) != 0 {
		t.Fatal("masked action must not be stored as a decision")
	}

	res, err = tl.Execute(ctx, json.RawMessage(`{"action":"plan_batch","batch":{"id":"b1","goal":"bounded"}}`))
	if err != nil || !res.Success {
		t.Fatalf("plan_batch in ModePlan must pass, got res=%+v err=%v", res, err)
	}
}
