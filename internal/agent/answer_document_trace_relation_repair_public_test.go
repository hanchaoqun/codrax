package agent

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceRelationDiagramPublicRepairKeepsNativeAuthority(t *testing.T) {
	ctx := traceRelationDiagramContext(t, queryTransactionDiagramSupport(t, 1, 1.05))
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	_ = (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "保留原说明，关联不等于根因。"},
		{ID: "graph", Kind: types.BlockDiagram, FacetIDs: []string{string(types.FacetDiagramSpine), string(types.FacetObservedArtifactFact)},
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n participant App as 应用\n participant Service as 渲染服务"}},
	}}
	raw, _ := json.Marshal(doc)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success || !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &result, false) {
		t.Fatalf("zero-edge failure must publish a local executable repair: %v %+v", err, result)
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil || len(lease.AllowedAdditions) != 3 {
		t.Fatalf("repair must offer existing native relations, not manufacture a choice: %+v", lease)
	}
	choice := lease.AllowedAdditions[0]
	var schema any
	if err := json.Unmarshal((&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx), &schema); err != nil {
		t.Fatal(err)
	}
	placement := traceRelationRepairPlacement(schema)
	if placement == "" {
		t.Fatal("actual patch schema must publish the current sequence insertion position")
	}
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": []any{map[string]any{
		"addition_ref": choice.AdditionRef, "action": "add", "placement_ref": placement,
		"edge":                    map[string]string{"from_node": choice.FromNodeIDs[0], "to_node": choice.ToNodeIDs[0], "visible_label": "记录对应"},
		"from_node_visible_label": "提交记录", "to_node_visible_label": "消费记录",
	}}})
	if err := toolparam.Validate(raw, (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)); err != nil {
		t.Fatal(err)
	}
	result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("model-selected exact relation patch rejected: %v %+v", err, result)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if got == nil || got.Blocks[0].Text != doc.Blocks[0].Text || tool.RequiredRuntimeDiagramRelationMissing(bus, got, view) || len(tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, got, view, nil)) != 0 {
		t.Fatalf("repaired graph lost native authority or sibling prose: %+v", got)
	}
}

func traceRelationRepairPlacement(value any) string {
	switch value := value.(type) {
	case map[string]any:
		if field, ok := value["placement_ref"].(map[string]any); ok {
			if choices, ok := field["enum"].([]any); ok && len(choices) > 0 {
				ref, _ := choices[0].(string)
				return ref
			}
		}
		for _, child := range value {
			if ref := traceRelationRepairPlacement(child); ref != "" {
				return ref
			}
		}
	case []any:
		for _, child := range value {
			if ref := traceRelationRepairPlacement(child); ref != "" {
				return ref
			}
		}
	}
	return ""
}

func TestTraceRelationDiagramPublicOptionalArrowsStillNeedAuthority(t *testing.T) {
	result := queryTransactionDiagramSupport(t, 1, 1.05)
	for _, axis := range []types.PredicateAxis{types.AxisFlow, types.AxisDefine, ""} {
		t.Run(string(axis), func(t *testing.T) {
			ctx := traceRelationDiagramContext(t, result)
			ctx.AnalysisIR.RequestModel.PredicateAxis = axis
			ctx.AnalysisIR.RequestModel.DiagramHint.Required = false
			ctx.AnalysisIR.AnswerContract.Diagram.Required = false
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			_ = (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			view := types.BuildAnswerSemanticViewForBusContext(bus)
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, Text: "仅报告观察到的关系。"},
				{ID: "graph", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n A->>B: 关系"}},
			}}
			if tool.RequiredRuntimeDiagramRelationMissing(bus, doc, view) {
				t.Fatal("optional diagram must not acquire minimum coverage")
			}
			if len(tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, doc, view, nil)) == 0 {
				t.Fatal("optional relation arrow cannot escape its native evidence owner through legacy axis labels")
			}
			raw, _ := json.Marshal(doc)
			r, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || r.Success {
				t.Fatalf("unanchored optional relation accepted: %v %+v", err, r)
			}
		})
	}
}
