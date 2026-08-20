package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func atomicPatchTestDocument() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "keep"},
			{
				ID: "diag", Kind: types.BlockDiagram, Title: "model title",
				SurfaceRole: types.SurfacePrincipal,
				FacetIDs:    []string{"diagram_spine"},
				Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    A->>B: old label\n    B->>C: keep label\n"},
				EdgeAnchors: []types.DiagramEdgeAnchor{
					{FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence, VisibleLabel: "old label"},
					{FromNode: "B", ToNode: "C", FromIdentity: "Explorer", ToIdentity: "Extractor", RelationKind: types.DiagramRelPrecedence, VisibleLabel: "keep label"},
				},
			},
		},
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_PreservesUnmentionedGraphContent(t *testing.T) {
	prev := atomicPatchTestDocument()
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{
		{
			BlockID: "diag", Action: "relabel",
			Match:        &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence},
			VisibleLabel: "确定分析范围后收集证据",
		},
	}, []emitAnswerDiagramBoundaryReplacement{{
		BlockID: "diag",
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{
			Participant: "AnswerDocument", Status: types.DiagramParticipantBoundaryUnproven,
		}},
	}})
	if err != nil {
		t.Fatalf("atomic edit rejected: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("replace blocks=%d, want one compiled replacement", len(patch.ReplaceBlocks))
	}
	got := patch.ReplaceBlocks[0]
	for _, want := range []string{
		"participant A", "participant B", "participant C",
		"A->>B: 确定分析范围后收集证据", "B->>C: keep label",
	} {
		if !strings.Contains(got.Diagram.Body, want) {
			t.Fatalf("compiled body lost %q:\n%s", want, got.Diagram.Body)
		}
	}
	if got.Title != "model title" || got.SurfaceRole != types.SurfacePrincipal || len(got.FacetIDs) != 1 {
		t.Fatalf("unmentioned block fields changed: %+v", got)
	}
	if len(got.EdgeAnchors) != 2 || got.EdgeAnchors[0].VisibleLabel != "确定分析范围后收集证据" || got.EdgeAnchors[1].VisibleLabel != "keep label" {
		t.Fatalf("anchor delta was not local: %+v", got.EdgeAnchors)
	}
	if len(got.ParticipantBoundaries) != 1 || got.ParticipantBoundaries[0].Participant != "AnswerDocument" {
		t.Fatalf("boundary replacement missing: %+v", got.ParticipantBoundaries)
	}
}

func TestEmitAnswerDocumentPatch_AtomicRelationEditsHonorTypedLease(t *testing.T) {
	prev := atomicPatchTestDocument()
	mut := types.NewMutableState("atomic")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "semantic_relation_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, []types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: "Extractor", ToIdentity: "Finalizer", Source: "stageauthority",
		}}))
	bus := &types.BusContext{Mutable: mut}
	raw := json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[
			{"block_id":"diag","action":"remove","match":{"from_node":"A","to_node":"B","from_identity":"Analyzer","to_identity":"Explorer","relation_kind":"precedence"}},
			{"block_id":"diag","action":"add","edge":{"from_node":"C","to_node":"F","from_identity":"Extractor","to_identity":"Finalizer","relation_kind":"precedence","visible_label":"结构化事实就绪后组织答案"}}
		]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !res.Success {
		t.Fatalf("listed atomic transaction must pass: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || len(got.Blocks[1].EdgeAnchors) != 2 {
		t.Fatalf("unexpected persisted document: %+v", got)
	}
	if strings.Contains(got.Blocks[1].Diagram.Body, "A->>B: old label") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "B->>C: keep label") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "C->>F: 结构化事实就绪后组织答案") {
		t.Fatalf("atomic transaction changed the wrong graph surface:\n%s", got.Blocks[1].Diagram.Body)
	}
}

func TestEmitAnswerDocumentPatch_AtomicUnlistedAdditionStillRejectedByLease(t *testing.T) {
	prev := atomicPatchTestDocument()
	mut := types.NewMutableState("atomic-unlisted")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "semantic_relation_edge_unproven",
			FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelPrecedence,
		}}, nil))
	bus := &types.BusContext{Mutable: mut}
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"diagram_edge_edits":[{"block_id":"diag","action":"add","edge":{"from_node":"C","to_node":"F","from_identity":"Extractor","to_identity":"Finalizer","relation_kind":"precedence","visible_label":"组织答案"}}]
	}`))
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeAnswerDocRelationRepairScope || !strings.Contains(res.Summary, "unlisted_relation_added") {
		t.Fatalf("unlisted atomic relation must remain fail-closed: %+v", res)
	}
	if got := mut.AnswerDocumentV2(); got == nil || strings.Contains(got.Blocks[1].Diagram.Body, "C->>F") {
		t.Fatalf("rejected atomic transaction polluted the accepted base: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RejectsCompoundAndConflictingCarrier(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A->>B: first\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"diag"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "remove",
		Match: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall},
	}}, nil)
	if err == nil || !strings.Contains(err.Error(), "conflicts with unchanged_block_ids") {
		t.Fatalf("same carrier must not be both unchanged and atomically edited: %v", err)
	}
}

func TestRelabelAtomicMermaidEdgeLine_AllSupportedFamilies(t *testing.T) {
	for name, tc := range map[string][3]string{
		"sequence": {"sequenceDiagram\n  A->>B: old", "  A->>B: old", "A->>B: new"},
		"flow":     {"flowchart LR\n  A -->|old| B", "  A -->|old| B", "A -->|new| B"},
		"class":    {"classDiagram\n  A --> B : old", "  A --> B : old", "A --> B : new"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := relabelAtomicMermaidEdgeLine(tc[0], tc[1], "new")
			if err != nil || !strings.Contains(got, tc[2]) {
				t.Fatalf("got=%q err=%v want substring %q", got, err, tc[2])
			}
		})
	}
}
