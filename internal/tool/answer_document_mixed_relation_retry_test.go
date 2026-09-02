package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMixedRelationRetryKeepsSelectedListAdditionExecutableWithDiagramFailure(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockOrderedList, types.BlockBulletList, types.BlockTable} {
		t.Run(string(kind), func(t *testing.T) {
			evidence := diagramEvidenceTestCall("Service.run", "Store.save")
			base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, Text: "keep the model summary"},
				{ID: "path", Kind: kind, SurfaceRole: types.SurfacePrincipal,
					ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, EvidenceID: evidence.ID}},
					Items:     []types.AnswerBlockItem{{ID: "step", Text: "keep the model explanation", EvidenceIDs: []string{evidence.ID}}}},
				{ID: "picture", Kind: types.BlockDiagram,
					Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A[Service.run] --> B[Store.save]\n X --> Y\n"},
					EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", FromIdentity: "Service.run", ToIdentity: "Store.save", RelationKind: types.DiagramRelCall}}},
			}}
			mut := types.NewMutableState("mixed independent repair capabilities")
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
			bus := &types.BusContext{Mutable: mut, EvidenceItems: []types.EvidenceItem{evidence},
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
					AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}}}}
			hints := preCheckDiagramCallEdgeEvidenceAlignment(base, &types.AnswerSemanticView{Family: types.QFCallChain}, newPreEmitCheckContext(bus))
			repair := emitFixHintsRepair(hints)
			if repair == nil {
				t.Fatal("mixed draft must produce a typed repair")
			}
			var delta types.AnswerDiagramRelationRepairDelta
			if err := json.Unmarshal([]byte(repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
				t.Fatal(err)
			}
			lease := types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions)
			if lease == nil {
				t.Fatalf("individually executable list addition must coexist with sibling diagram failure: %+v", delta)
			}
			var additionRef, failureRef string
			for _, row := range lease.AllowedAdditions {
				if row.BlockID == "path" && row.EvidenceID == evidence.ID {
					additionRef = row.AdditionRef
				}
			}
			for _, row := range lease.Failures {
				if row.BlockID == "picture" && row.FromNode == "X" && row.ToNode == "Y" {
					failureRef = row.FailureRef
				}
			}
			if additionRef == "" || failureRef == "" {
				t.Fatalf("same-generation refs lost: %+v", lease)
			}
			mut.SetAnswerDiagramRelationRepairLease(lease)
			schema := string((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut, EvidenceItems: bus.EvidenceItems}))
			if !strings.Contains(schema, additionRef) || !strings.Contains(schema, failureRef) {
				t.Fatalf("live schema must publish both executable refs: %s", schema)
			}
			params := json.RawMessage(fmt.Sprintf(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[
				{"failure_ref":%q,"action":"remove"},
				{"addition_ref":%q,"action":"add","edge":{"from_node":"service","to_node":"store","visible_label":"保存数据"}}]}`, failureRef, additionRef))
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
			if err != nil || !result.Success {
				t.Fatalf("model-selected sibling repairs must commit in one patch: err=%v result=%+v", err, result)
			}
			got := mut.AnswerDocumentV2()
			if got == nil || len(got.Blocks) != 3 || got.Blocks[0].Text != base.Blocks[0].Text ||
				len(got.Blocks[1].Items) != 1 || got.Blocks[1].Items[0].Text != base.Blocks[1].Items[0].Text ||
				got.Blocks[1].Items[0].Label != base.Blocks[1].Items[0].Label ||
				!reflect.DeepEqual(got.Blocks[1].Items[0].EvidenceIDs, base.Blocks[1].Items[0].EvidenceIDs) || len(got.Blocks[1].EdgeAnchors) != 1 ||
				got.Blocks[2].Diagram.Body != "flowchart LR\n A[Service.run] --> B[Store.save]\n" {
				t.Fatalf("unselected model content changed: %+v", got)
			}
		})
	}
}

func TestRelationRetryGuidanceDoesNotOverrideLiveToolSchema(t *testing.T) {
	got := formatEmitFixHints([]emitFixHint{{Field: "blocks[id=path].edge_anchors", ExpectedShape: "repair the selected relation"}})
	for _, forbidden := range []string{"re-emit emit_answer_document", "SAME tool turn", "does NOT consume"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("shared retry envelope promises unavailable dispatch behavior: %s", got)
		}
	}
	if !strings.Contains(got, "current tool schema") {
		t.Fatalf("retry envelope must defer to the active dispatch: %s", got)
	}
}
