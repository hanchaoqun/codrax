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

func TestEmitWriteWorkflowDecision_RepairSchemaIsModeProjected(t *testing.T) {
	tl := &EmitWriteWorkflowDecision{}

	projected := string(writeWorkflowDecisionSchemaForMode(types.ModeVerify))
	if got := string(tl.ParametersFor(&types.AgentContext{Mode: types.ModeVerify})); got != projected {
		t.Fatalf("runtime repair schema must match dispatch schema for ModeVerify\nrepair=%s\ndispatch=%s", projected, got)
	}

	var decoded struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(projected), &decoded); err != nil {
		t.Fatalf("projected schema must decode: %v", err)
	}
	actionSchema, ok := decoded.Properties["action"]
	if !ok {
		t.Fatal("projected schema must include action property")
	}
	allowed := map[string]bool{}
	for _, action := range actionSchema.Enum {
		allowed[action] = true
	}
	for _, action := range []writeflow.WorkflowAction{
		writeflow.ActionVerifyBatch,
		writeflow.ActionAskUser,
		writeflow.ActionFinish,
		writeflow.ActionBlock,
	} {
		if !allowed[string(action)] {
			t.Fatalf("ModeVerify schema omitted allowed action %q; enum=%v", action, actionSchema.Enum)
		}
	}
	for _, masked := range []writeflow.WorkflowAction{
		writeflow.ActionExploreCode,
		writeflow.ActionPlanBatch,
		writeflow.ActionApplyPlan,
		writeflow.ActionAppendBatch,
		writeflow.ActionSplitBatch,
		writeflow.ActionReplanBatch,
	} {
		if allowed[string(masked)] || strings.Contains(projected, `"`+string(masked)+`"`) {
			t.Fatalf("ModeVerify repair schema must not expose masked action %q; schema=%s", masked, projected)
		}
	}

	mu := types.NewMutableState("mode verify")
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeVerify}
	res, err := tl.Execute(ctx, json.RawMessage(`{"action":"plan_batch","batch":{"id":"b1","goal":"bounded"}}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success {
		t.Fatalf("plan_batch in ModeVerify must be rejected, got %+v", res)
	}
	if res.Repair == nil || res.Repair.Code != "workflow_action_not_in_mode" {
		t.Fatalf("rejection must carry typed mode repair, got %+v", res.Repair)
	}
	if strings.Contains(res.Repair.Hint, string(writeflow.ActionPlanBatch)) ||
		strings.Contains(res.Repair.Hint, string(writeflow.ActionApplyPlan)) ||
		strings.Contains(res.Repair.Hint, string(writeflow.ActionExploreCode)) {
		t.Fatalf("mode repair hint must only name ModeVerify actions, got %q", res.Repair.Hint)
	}
	if len(mu.WriteWorkflowDecisionJSON()) != 0 {
		t.Fatal("masked action must not be stored as a decision")
	}
}
