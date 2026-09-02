package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestStandaloneRelationRepairKeepsReaderLabelsSeparateFromDiagramSyntax(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockOrderedList, types.BlockBulletList, types.BlockTable} {
		t.Run(string(kind), func(t *testing.T) {
			evidence := diagramEvidenceTestCall("Service.run", "Store.save")
			block := types.AnswerBlock{ID: "path", Kind: kind, SurfaceRole: types.SurfacePrincipal,
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, EvidenceID: evidence.ID}},
				Items:     []types.AnswerBlockItem{{ID: "step", Text: "保留模型的业务说明", EvidenceIDs: []string{evidence.ID}}}}
			if kind == types.BlockTable {
				block.Columns = []string{"说明"}
				block.Items[0].Cells = []string{"保留模型的业务说明"}
			}
			base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{block}}
			var delta types.AnswerDiagramRelationRepairDelta
			if err := json.Unmarshal([]byte(preEmitStandaloneRelationClaimRepairDeltaJSON(base, block, []types.EvidenceItem{evidence})), &delta); err != nil {
				t.Fatal(err)
			}
			if len(delta.AllowedAdditions) != 1 {
				t.Fatalf("exact selected relation lost: %+v", delta)
			}
			candidate := delta.AllowedAdditions[0]
			if len(candidate.FromNodeIDs) != 0 || len(candidate.ToNodeIDs) != 0 {
				t.Errorf("non-diagram reader labels must not be taught syntax aliases: %+v", candidate)
			}
			if candidate.FromIdentity != "Service.run" || candidate.ToIdentity != "Store.save" || candidate.EvidenceID != evidence.ID {
				t.Fatalf("display semantics changed evidence selection: %+v", candidate)
			}
			lease := types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions)
			if lease == nil || len(lease.AllowedAdditions) != 1 {
				t.Fatalf("selected list/table addition is no longer executable: %+v", lease)
			}
			mut := types.NewMutableState("reader-owned labels")
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
			mut.SetAnswerDiagramRelationRepairLease(lease)
			bus := &types.BusContext{Mutable: mut, EvidenceItems: []types.EvidenceItem{evidence}}
			ref := lease.AllowedAdditions[0].AdditionRef
			var schema map[string]any
			if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut, EvidenceItems: bus.EvidenceItems}), &schema); err != nil {
				t.Fatal(err)
			}
			// Inspect the actual dispatched schema, not only a helper's template.
			var branch map[string]any
			var visit func(any)
			visit = func(value any) {
				switch node := value.(type) {
				case map[string]any:
					props, _ := node["properties"].(map[string]any)
					selector, _ := props["addition_ref"].(map[string]any)
					values, _ := selector["enum"].([]any)
					if len(values) == 1 && values[0] == ref {
						branch = node
					}
					for _, child := range node {
						visit(child)
					}
				case []any:
					for _, child := range node {
						visit(child)
					}
				}
			}
			visit(schema)
			if branch == nil {
				t.Fatal("dispatch omitted selected addition")
			}
			props := branch["properties"].(map[string]any)
			edge := props["edge"].(map[string]any)
			edgeProps := edge["properties"].(map[string]any)
			for _, field := range []string{"from_node", "to_node", "visible_label"} {
				description, _ := edgeProps[field].(map[string]any)["description"].(string)
				if !strings.Contains(description, "reader") {
					t.Errorf("%s lacks visible reader semantics: %q", field, description)
				}
				if _, flat := props[field]; flat {
					t.Errorf("%s must live inside edge", field)
				}
			}
			if strings.Contains(fmt.Sprint(branch["description"]), "hidden typed anchor") {
				t.Error("schema wrongly promises invisible metadata")
			}
			bad := json.RawMessage(fmt.Sprintf(`{"diagram_edge_edits":[{"addition_ref":%q,"action":"add"}]}`, ref))
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, bad)
			if err != nil || result.Success || !strings.Contains(fmt.Sprint(result), `edge:{from_node,to_node,visible_label}`) {
				t.Errorf("first missing-edge error must teach nesting: err=%v result=%+v", err, result)
			}
			if !reflect.DeepEqual(mut.AnswerDocumentV2(), base) {
				t.Fatal("rejected patch changed prior answer")
			}
			params := json.RawMessage(fmt.Sprintf(`{"diagram_edge_edits":[{"addition_ref":%q,"action":"add","edge":{"from_node":"请求入口","to_node":"订单存储","visible_label":"保存订单"}}]}`, ref))
			result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, params)
			if err != nil || !result.Success {
				t.Fatalf("reader labels must execute without syntax-id authority: err=%v result=%+v", err, result)
			}
			got := mut.AnswerDocumentV2()
			if got == nil || len(got.Blocks) != 1 || len(got.Blocks[0].Items) != 1 ||
				got.Blocks[0].Items[0].ID != block.Items[0].ID || got.Blocks[0].Items[0].Text != block.Items[0].Text ||
				!reflect.DeepEqual(got.Blocks[0].Items[0].Cells, block.Items[0].Cells) ||
				!reflect.DeepEqual(got.Blocks[0].Items[0].EvidenceIDs, block.Items[0].EvidenceIDs) ||
				!reflect.DeepEqual(got.Blocks[0].Columns, block.Columns) {
				t.Fatalf("patch changed model's original content: %+v", got)
			}
			anchor := got.Blocks[0].EdgeAnchors[0]
			if anchor.FromNode != "请求入口" || anchor.ToNode != "订单存储" || anchor.FromIdentity != candidate.FromIdentity || anchor.ToIdentity != candidate.ToIdentity {
				t.Fatalf("reader labels and exact identities were conflated: %+v", anchor)
			}
			if output := render.RenderAnswerDocument(got, "zh"); !strings.Contains(output, "请求入口 → 订单存储") || !strings.Contains(output, "保存订单") {
				t.Fatalf("model-authored relationship did not reach reader: %s", output)
			}
		})
	}
}
