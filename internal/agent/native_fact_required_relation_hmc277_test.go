package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC277NativeFactDiagramCannotSatisfyRequiredRuntimeRelation(t *testing.T) {
	nativeCtx, _ := hmc277MeasurementContext(t, "hmosperf_io_inflight", "window_stats", 1, 1.05)
	results := append([]types.ToolResult(nil), nativeCtx.Mutable.TurnAArtifacts().ToolResults...)
	results = append(results, queryTransactionDiagramSupport(t, 1, 1.05))
	// Keep the query-owning MutableState: moving ToolResults alone to a new
	// context must not recreate the private prepared-source display receipt.
	ctx := nativeCtx
	ctx.AnalysisIR = transactionRequiredDiagramContext(t, types.DiagramFlow, 1, 1.05, nil).AnalysisIR
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if view.DiagramPlan == nil || !view.DiagramPlan.Required || !view.DiagramPlan.RequireStructuralEdge {
		t.Fatal("fixture must have an actual required runtime relationship plan")
	}
	var receipt *types.AnswerRuntimeMeasurementReceipt
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.HasNativeFixedFacts() && strings.Contains(table.ObservationID, "#io_inflight:") {
			receipt = &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
			break
		}
	}
	if receipt == nil || !types.BindRuntimeMeasurementReceipt(receipt, view.RuntimeMeasurementContract) {
		t.Fatal("missing current native fact selector")
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Measured occupancy is separate from the requested transaction correspondence."},
		{ID: "facts", Kind: types.BlockDiagram, RuntimeMeasurement: receipt},
	}}
	raw, _ := json.Marshal(doc)
	if err := toolparam.Validate(raw, tool.BuildAnswerDocumentParametersFor(view)); err != nil {
		t.Fatalf("the pure fact shape must remain legal before relation coverage: %v", err)
	}
	// This shared predicate also feeds orchestrator's final contract check.
	// That checker has an additional generic fact/relationship separation guard;
	// pre-emit must not accept what the later guard will correctly reject.
	if !tool.RequiredRuntimeDiagramRelationMissing(bus, doc, view) {
		t.Error("shared runtime relationship check missed the edgeless-only fact diagram")
	}
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success {
		t.Errorf("public emit accepted no requested runtime relationship: err=%v success=%t summary=%s", err, result.Success, result.Summary)
	}
	if installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &result, false) {
		t.Error("an edgeless fact selector received an inapplicable add-edge repair lease")
	}
	for _, mode := range []string{"optional", "node_only"} {
		control := *view
		plan := *view.DiagramPlan
		control.DiagramPlan = &plan
		if mode == "optional" {
			plan.Required = false
		} else {
			plan.RequireStructuralEdge = false
		}
		if tool.RequiredRuntimeDiagramRelationMissing(bus, doc, &control) {
			t.Errorf("%s plan acquired a new relationship obligation", mode)
		}
	}
	rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if len(rows) == 0 {
		t.Fatal("fixture lost its actual scoped relationship choices")
	}
	corrected := transactionSupportedDiagram(rows[:1], types.DiagramFlow)
	facts := doc.Blocks[1]
	facts.Kind = types.BlockTable
	corrected.Blocks = append(corrected.Blocks, facts)
	raw, _ = json.Marshal(corrected)
	if err := toolparam.Validate(raw, tool.BuildAnswerDocumentParametersFor(view)); err != nil {
		t.Fatalf("model-separated fact/relationship correction was not executable: %v", err)
	}
	result, err = (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("model-selected real relation plus preserved native table rejected: %v %s", err, result.Summary)
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	if tool.RequiredRuntimeDiagramRelationMissing(bus, accepted, view) {
		t.Fatal("one actual model-selected relation did not settle the minimum relationship obligation")
	}
	for _, block := range accepted.Blocks {
		if block.ID == "facts" && block.RuntimeMeasurement != nil && block.RuntimeMeasurement.IsBound() && block.Diagram == nil {
			return
		}
	}
	t.Fatal("relationship correction lost the independent native fact table")
}
