package tool

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1584LiveRelationAddTeachingMatchesNestedSchema(t *testing.T) {
	for _, relation := range []struct {
		kind   types.DiagramRelationKind
		claim  types.ClaimForm
		anchor types.AnchorKind
	}{
		{types.DiagramRelCall, types.ClaimCallEdge, types.AnchorCall},
		{types.DiagramRelCallback, types.ClaimCallbackHandoff, types.AnchorCallback},
	} {
		for _, blockKind := range []types.AnswerBlockKind{types.BlockOrderedList, types.BlockTable} {
			t.Run(string(relation.kind)+"/"+string(blockKind), func(t *testing.T) {
				evidence := types.EvidenceItem{ID: "ev-selected", Kind: types.EvidenceRelationship,
					Subject: "Service.run", Object: "Worker.handle", Predicate: "calls", Source: "src/service.py",
					LineStart: 10, LineEnd: 10, Scope: types.ScopeLine, AnchorKind: relation.anchor, GroundingStatus: types.GroundingGrounded}
				block := types.AnswerBlock{ID: "path", Kind: blockKind, SurfaceRole: types.SurfacePrincipal,
					ClaimUses: []types.RenderedClaimUse{{ClaimForm: relation.claim, EvidenceID: evidence.ID}},
					Items:     []types.AnswerBlockItem{{ID: "step", Text: "模型业务说明", EvidenceIDs: []string{evidence.ID}}}}
				if blockKind == types.BlockTable {
					block.Columns, block.Items[0].Cells = []string{"说明"}, []string{"模型业务说明"}
				}
				base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{block}}
				before, _ := json.Marshal(base)
				lease := types.NewAnswerDiagramRelationRepairLease(base, nil, []types.AnswerDiagramRelationRepairCandidate{{
					BlockID: block.ID, RelationKind: relation.kind, FromIdentity: evidence.Subject, ToIdentity: evidence.Object,
					EvidenceID: evidence.ID, Source: "src/service.py:10",
				}})
				if lease == nil || len(lease.AllowedAdditions) != 1 {
					t.Fatalf("missing live addition: %+v", lease)
				}
				mut := types.NewMutableState("B1584 nested addition teaching")
				mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
				mut.SetAnswerDiagramRelationRepairLease(lease)
				ctx := &types.AgentContext{Mutable: mut}
				var schema map[string]any
				if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(ctx), &schema); err != nil {
					t.Fatal(err)
				}
				props := schema["properties"].(map[string]any)
				branches := props["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
				if len(branches) != 1 {
					t.Fatalf("expected exact addition-only branch: %+v", branches)
				}
				operation := branches[0].(map[string]any)["properties"].(map[string]any)
				edge := operation["edge"].(map[string]any)["properties"].(map[string]any)
				for _, field := range []string{"from_node", "to_node", "visible_label"} {
					if edge[field] == nil || operation[field] != nil {
						t.Fatalf("field %s must remain nested in the actual schema", field)
					}
				}
				for _, hidden := range []string{"from_identity", "to_identity", "relation_kind"} {
					if edge[hidden] != nil {
						t.Fatalf("ref branch must not add a model obligation for %s", hidden)
					}
				}
				hints := preCheckStandaloneCallChainRelationAnchorPresence(base, &types.AnswerSemanticView{Family: types.QFCallChain},
					newPreEmitCheckContext(&types.BusContext{Mutable: mut, EvidenceItems: []types.EvidenceItem{evidence}}))
				if len(hints) != 1 {
					t.Fatalf("missing real precheck repair hint: %+v", hints)
				}
				for name, teaching := range map[string]string{
					"live description":       (&EmitAnswerDocumentPatch{}).DescriptionFor(ctx),
					"shared patch teaching":  types.AnswerDocumentPatchOperationTeaching,
					"missing anchors action": hints[0].ExpectedShape,
				} {
					for _, want := range []string{"diagram_edge_edits[].edge.{from_node,to_node,visible_label}", "ref-selected", "replace_blocks", "endpoint identities"} {
						if !strings.Contains(teaching, want) {
							t.Errorf("%s omits exact nested/ownership teaching %q: %s", name, want, teaching)
						}
					}
				}
				after, _ := json.Marshal(base)
				stored, _ := json.Marshal(mut.AnswerDocumentV2())
				if !bytes.Equal(before, after) || !bytes.Equal(before, stored) {
					t.Fatal("teaching/schema projection changed model-authored content")
				}
			})
		}
	}
}

func TestB1584LiveMetadataAttachDoesNotRequestAnEdgeReplay(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "path", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "caller", ToNode: "callee", RelationKind: types.DiagramRelCall, VisibleLabel: "模型文字"}}}}}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "path", Issue: diagramStandaloneRelationIdentityMissing, FromNode: "caller", ToNode: "callee",
		RelationKind: types.DiagramRelCall, TargetCarrier: types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata,
	}}, []types.AnswerDiagramRelationRepairCandidate{{BlockID: "path", RelationKind: types.DiagramRelCall,
		FromIdentity: "Service.run", ToIdentity: "Worker.handle", Source: "src/service.py:10"}})
	mut := types.NewMutableState("B1584 attach teaching")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	ctx := &types.AgentContext{Mutable: mut}
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(ctx), &schema); err != nil {
		t.Fatal(err)
	}
	branches := schema["properties"].(map[string]any)["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	found := false
	for _, raw := range branches {
		properties := raw.(map[string]any)["properties"].(map[string]any)
		if properties["action"].(map[string]any)["enum"].([]any)[0] == "attach" {
			found = true
			if properties["edge"] != nil {
				t.Fatal("metadata attach must keep its no-replay schema")
			}
		}
	}
	if !found || !strings.Contains((&EmitAnswerDocumentPatch{}).DescriptionFor(ctx), "attach uses its published schema branch") {
		t.Fatal("attach teaching must not borrow addition's edge requirement")
	}
}

func TestB1584WholeBlockRepairTeachingDoesNotInheritAtomicFieldOmissions(t *testing.T) {
	for _, delegated := range []bool{false, true} {
		t.Run(map[bool]string{false: "broad_compatibility", true: "delegated_replacement"}[delegated], func(t *testing.T) {
			base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, Text: "仅解释现有行为，不选择关系元数据"},
				{ID: "path", Kind: types.BlockOrderedList, EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "caller", ToNode: "callee", RelationKind: types.DiagramRelCall, VisibleLabel: "模型文字",
				}}},
			}}
			failure := types.AnswerDiagramRelationRepairFailure{BlockID: "path", Issue: "typed_anchor_without_visible_edge",
				FromNode: "caller", ToNode: "callee", RelationKind: types.DiagramRelCall}
			if delegated {
				failure.Issue = diagramStandaloneRelationIdentityMissing
				failure.TargetCarrier = types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata
			}
			lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{failure}, nil)
			if lease == nil || (delegated && !types.BindAnswerDiagramRelationRepairOrdinaryValidationBlocks(lease, base, []string{"path"})) {
				t.Fatal("failed to construct real compatibility/delegated repair surface")
			}
			mut := types.NewMutableState("B1584 whole-block scope")
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
			mut.SetAnswerDiagramRelationRepairLease(lease)
			ctx := &types.AgentContext{Mutable: mut}
			var schema map[string]any
			if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(ctx), &schema); err != nil {
				t.Fatal(err)
			}
			properties := schema["properties"].(map[string]any)
			replacement := properties["replace_blocks"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
			anchors := replacement["edge_anchors"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
			for _, field := range []string{"from_identity", "to_identity", "relation_kind"} {
				if anchors[field] == nil {
					t.Errorf("whole-block replacement unexpectedly lost %s", field)
				}
			}
			description := (&EmitAnswerDocumentPatch{}).DescriptionFor(ctx)
			for _, want := range []string{"chosen relation metadata", "endpoint identities its block schema requires", "attach uses its published schema branch"} {
				if !strings.Contains(description, want) {
					t.Errorf("whole-block teaching broadened atomic obligations or omissions: missing %q in %s", want, description)
				}
			}
			if delegated && !strings.Contains(description, "For ref-selected atomic branches") {
				t.Errorf("unavailable-fields sentence must be atomic-ref scoped: %s", description)
			}
			if strings.Contains(description, "authority: omitted legacy coordinates, hidden endpoint identities, and relation kinds are unavailable") {
				t.Errorf("whole-block schema permits identities that broad prose incorrectly forbids: %s", description)
			}
			if len(mut.AnswerDocumentV2().Blocks[0].EdgeAnchors) != 0 {
				t.Fatal("descriptive summary must not gain a relationship obligation")
			}
		})
	}
}
