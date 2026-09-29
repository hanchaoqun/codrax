package tool

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMemberSetMetadataChoicesRemainTypedAndAtomic(t *testing.T) {
	view := &types.AnswerSemanticView{Presentation: types.AnswerPresentationContract{RequestedDimensions: []types.RequestedAnswerDimension{
		{Index: 1, Role: types.RequestedAnswerDimensionMemberSet, Required: true},
		{Index: 2, Role: types.RequestedAnswerDimensionMemberSet, Required: true},
	}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "a", Kind: types.BlockTable, Columns: []string{"Time (s)"}, Items: []types.AnswerBlockItem{{Cells: []string{"1.3"}}}},
		{ID: "b", Kind: types.BlockBulletList, Items: []types.AnswerBlockItem{{SourceInventoryRowID: "source-row", Text: "Original item"}}},
		{ID: "c", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{Text: "Original order"}}},
		{ID: "edge", Kind: types.BlockTable, EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B"}}, Items: []types.AnswerBlockItem{{Text: "edge"}}},
		{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Language: "mermaid", Kind: "flow", Body: "flowchart LR\n A --> B"}},
	}}
	if got := answerDocumentMemberSetFacetAdditionCandidateBlockIDs(doc, view); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("wrong typed choices %v", got)
	}
	before, _ := json.Marshal(doc)
	edits := []types.AnswerBlockFieldEditV1{{BlockID: "a", Field: types.AnswerBlockFieldAddFacetID, Value: "member_set"}, {BlockID: "b", Field: types.AnswerBlockFieldAddFacetID, Value: "member_set"}}
	for _, bad := range []types.AnswerBlockFieldEditV1{{BlockID: "foreign", Field: types.AnswerBlockFieldAddFacetID, Value: "member_set"}, {BlockID: "c", Field: types.AnswerBlockFieldAddFacetID, Value: "invented"}} {
		ops := append(append([]types.AnswerBlockFieldEditV1{}, edits...), bad)
		if _, err := types.ApplyAnswerDocumentV2Patch(doc, &types.AnswerDocumentV2Patch{BlockFieldEditsV1: ops}); err == nil {
			t.Fatal("invalid target/value accepted")
		}
		after, _ := json.Marshal(doc)
		if string(before) != string(after) {
			t.Fatal("partial mutation")
		}
	}
	// Explicit whole-block replacement/deletion is still authoring, not a
	// metadata operation. Do not silently resurrect a deliberately removed field.
	got, err := types.ApplyAnswerDocumentV2Patch(doc, &types.AnswerDocumentV2Patch{ReplaceBlocks: []types.AnswerBlock{{ID: "a", Kind: types.BlockSection, Text: "Deliberate rewrite"}}, RemoveBlockIDs: []string{"diagram"}})
	if err != nil || len(got.Blocks) != 4 || len(got.Blocks[0].Columns) != 0 {
		t.Fatalf("explicit rewrite/deletion changed: %+v %v", got, err)
	}
	// Scope metadata on a diagram already uses the same lossless operation;
	// the new roster candidates must not grant relation ownership to it.
	got, err = types.ApplyAnswerDocumentV2Patch(doc, &types.AnswerDocumentV2Patch{BlockFieldEditsV1: []types.AnswerBlockFieldEditV1{{BlockID: "diagram", Field: types.AnswerBlockFieldSurfaceRole, Value: "principal"}}})
	if err != nil || !reflect.DeepEqual(doc.Blocks[4].Diagram, got.Blocks[4].Diagram) {
		t.Fatalf("diagram payload changed: %v", err)
	}
}
