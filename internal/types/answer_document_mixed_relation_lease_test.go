package types

import "testing"

func TestMixedRelationLeaseAdmitsOnlyPreviouslySelectedSiblingAdditions(t *testing.T) {
	newBase := func() *AnswerDocumentV2 {
		return &AnswerDocumentV2{Blocks: []AnswerBlock{
			{ID: "diagram", Kind: BlockDiagram, Diagram: &AnswerDiagramBlock{Kind: DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A --> B\n"}},
			{ID: "list", Kind: BlockOrderedList,
				ClaimUses: []RenderedClaimUse{{ClaimForm: ClaimCallEdge, EvidenceID: "selected"}},
				Items:     []AnswerBlockItem{{Text: "arbitrary prose cannot grant a relationship", EvidenceIDs: []string{"selected"}}}},
		}}
	}
	failures := []AnswerDiagramRelationRepairFailure{{BlockID: "diagram", Issue: "missing_call_anchor", FromNode: "A", ToNode: "B", BodyOccurrence: 1}}
	selected := AnswerDiagramRelationRepairCandidate{BlockID: "list", RelationKind: DiagramRelCall,
		FromIdentity: "Caller.run", ToIdentity: "Store.save", EvidenceID: "selected", Source: "src/caller.go:7"}
	if got := NewAnswerDiagramRelationRepairLease(newBase(), failures, []AnswerDiagramRelationRepairCandidate{selected}); got == nil {
		t.Fatal("exact selected sibling capability must compose")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*AnswerDocumentV2, *AnswerDiagramRelationRepairCandidate)
	}{
		{"missing claim", func(b *AnswerDocumentV2, _ *AnswerDiagramRelationRepairCandidate) { b.Blocks[1].ClaimUses = nil }},
		{"missing item selection", func(b *AnswerDocumentV2, _ *AnswerDiagramRelationRepairCandidate) {
			b.Blocks[1].Items[0].EvidenceIDs = nil
		}},
		{"different relation", func(_ *AnswerDocumentV2, c *AnswerDiagramRelationRepairCandidate) {
			c.RelationKind = DiagramRelRegister
		}},
		{"different evidence", func(_ *AnswerDocumentV2, c *AnswerDiagramRelationRepairCandidate) { c.EvidenceID = "not-selected" }},
		{"missing source", func(_ *AnswerDocumentV2, c *AnswerDiagramRelationRepairCandidate) { c.Source = "" }},
		{"duplicate block", func(b *AnswerDocumentV2, _ *AnswerDiagramRelationRepairCandidate) {
			b.Blocks = append(b.Blocks, b.Blocks[1])
		}},
		{"unrelated diagram", func(b *AnswerDocumentV2, _ *AnswerDiagramRelationRepairCandidate) { b.Blocks[1].Kind = BlockDiagram }},
		{"already has relation", func(b *AnswerDocumentV2, _ *AnswerDiagramRelationRepairCandidate) {
			b.Blocks[1].EdgeAnchors = []DiagramEdgeAnchor{{FromNode: "C", ToNode: "D", RelationKind: DiagramRelCall}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, candidate := newBase(), selected
			tc.mutate(base, &candidate)
			if got := NewAnswerDiagramRelationRepairLease(base, failures, []AnswerDiagramRelationRepairCandidate{candidate}); got != nil {
				t.Fatalf("unselected sibling authority must remain rejected: %+v", got)
			}
		})
	}
	unselected := selected
	unselected.EvidenceID, unselected.ToIdentity = "unselected", "Other.save"
	for _, candidates := range [][]AnswerDiagramRelationRepairCandidate{{selected, unselected}, {unselected, selected}} {
		if got := NewAnswerDiagramRelationRepairLease(newBase(), failures, candidates); got != nil {
			t.Fatalf("a selected sibling row must not authorize another unselected row, regardless of order: %+v", got)
		}
	}
}
