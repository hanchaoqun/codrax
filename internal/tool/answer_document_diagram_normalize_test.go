package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeDiagramDefinitionLabelsByEvidence_RewritesCallSitePath(t *testing.T) {
	mut := types.NewMutableState("diagram ownership")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "source/runtime/result_writer.py",
			LineStart:       8,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "write_results",
			Subject:         "write_results",
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceRelationship,
			Source:          "source/runtime/batch_runner.py",
			LineStart:       34,
			AnchorKind:      types.AnchorCall,
			Subject:         "BatchRunner.run_file",
			Predicate:       "calls",
			Object:          "write_results",
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
		},
	})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "d1",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramArchitecture,
			Language: "mermaid",
			Body: strings.Join([]string{
				"flowchart TD",
				"    n2[\"run_file @ source/runtime/batch_runner.py:22\"]",
				"    n9[\"write_results @ source/runtime/batch_runner.py\"]",
				"    n2 --> n9",
			}, "\n"),
		},
	}}}

	fixed := normalizeDiagramDefinitionLabelsByEvidence(doc, newPreEmitCheckContext(&types.BusContext{Mutable: mut}))
	if fixed != 1 {
		t.Fatalf("fixed=%d, want 1\n%s", fixed, doc.Blocks[0].Diagram.Body)
	}
	body := doc.Blocks[0].Diagram.Body
	if !strings.Contains(body, `n9["write_results @ source/runtime/result_writer.py:8"]`) {
		t.Fatalf("definition label not rewritten:\n%s", body)
	}
	if strings.Contains(body, `write_results @ source/runtime/batch_runner.py"]`) {
		t.Fatalf("call-site path survived in definition-looking label:\n%s", body)
	}
}

func TestNormalizeDiagramDefinitionLabelsByEvidence_CrossLanguage(t *testing.T) {
	mut := types.NewMutableState("diagram ownership")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "src/ui/render.ets",
			LineStart:       42,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RenderPanel",
			Subject:         "RenderPanel",
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceRelationship,
			Source:          "src/pages/Index.ets",
			LineStart:       17,
			AnchorKind:      types.AnchorCall,
			Subject:         "Index",
			Predicate:       "calls",
			Object:          "RenderPanel",
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
		},
	})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "d1",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramArchitecture,
			Language: "mermaid",
			Body:     "flowchart TD\n    page[\"RenderPanel @ src/pages/Index.ets\"]\n",
		},
	}}}

	fixed := normalizeDiagramDefinitionLabelsByEvidence(doc, newPreEmitCheckContext(&types.BusContext{Mutable: mut}))
	if fixed != 1 {
		t.Fatalf("fixed=%d, want 1\n%s", fixed, doc.Blocks[0].Diagram.Body)
	}
	if !strings.Contains(doc.Blocks[0].Diagram.Body, `RenderPanel @ src/ui/render.ets:42`) {
		t.Fatalf("cross-language label not rewritten:\n%s", doc.Blocks[0].Diagram.Body)
	}
}

func TestNormalizeDiagramDefinitionLabelsByEvidence_AddsMissingDefinitionLine(t *testing.T) {
	mut := types.NewMutableState("diagram ownership")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/tool/handler.go",
		LineStart:       77,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Handle",
		Subject:         "Handle",
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "d1",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: `flowchart TD
    h["Handle @ internal/tool/handler.go"]
`},
	}}}

	fixed := normalizeDiagramDefinitionLabelsByEvidence(doc, newPreEmitCheckContext(&types.BusContext{Mutable: mut}))
	if fixed != 1 {
		t.Fatalf("fixed=%d, want 1\n%s", fixed, doc.Blocks[0].Diagram.Body)
	}
	if !strings.Contains(doc.Blocks[0].Diagram.Body, `Handle @ internal/tool/handler.go:77`) {
		t.Fatalf("missing definition line not materialized:\n%s", doc.Blocks[0].Diagram.Body)
	}
}

func TestNormalizeDiagramEdgeAnchorMetadata_NormalizesAndAddsRelations(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "d1",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramArchitecture,
			Language: "mermaid",
			Body: strings.Join([]string{
				"flowchart TD",
				"    A[\"Caller\"] -->|calls| B[\"Callee\"]",
				"    B -->|imports| C[\"Module\"]",
			}, "\n"),
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode:     "Caller",
			ToNode:       "Callee",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimImportEdge,
		}},
	}}}

	fixed := normalizeDiagramEdgeAnchorMetadata(doc)
	if fixed < 4 {
		t.Fatalf("fixed=%d, want at least 4; anchors=%+v", fixed, doc.Blocks[0].EdgeAnchors)
	}
	anchors := doc.Blocks[0].EdgeAnchors
	if len(anchors) != 2 {
		t.Fatalf("len(edge_anchors)=%d, want 2: %+v", len(anchors), anchors)
	}
	if anchors[0].FromNode != "A" || anchors[0].ToNode != "B" ||
		anchors[0].RelationKind != types.DiagramRelCall ||
		anchors[0].ClaimForm != types.ClaimCallEdge {
		t.Fatalf("existing anchor not normalized: %+v", anchors[0])
	}
	if anchors[1].FromNode != "B" || anchors[1].ToNode != "C" ||
		anchors[1].RelationKind != types.DiagramRelImport ||
		anchors[1].ClaimForm != types.ClaimImportEdge {
		t.Fatalf("label-only relation anchor not added: %+v", anchors[1])
	}
}
