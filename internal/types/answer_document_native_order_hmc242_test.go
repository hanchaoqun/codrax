package types

import "testing"

func TestHMC242NativeOrderCarrierIsExactAndPopulationPreserved(t *testing.T) {
	anchor := DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "runtime_instance_a", ToIdentity: "runtime_instance_b", RelationKind: DiagramRelWakeup}
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "d", Kind: BlockDiagram, Diagram: &AnswerDiagramBlock{Kind: DiagramSequence, Body: "sequenceDiagram\n A->>B: event"}, EdgeAnchors: []DiagramEdgeAnchor{anchor}}}}
	failure := AnswerDiagramRelationRepairFailure{BlockID: "d", Issue: DiagramRelationFailureNativeSequenceEventOrder, FromNode: "A", ToNode: "B", FromIdentity: anchor.FromIdentity, ToIdentity: anchor.ToIdentity, RelationKind: DiagramRelWakeup, BodyOccurrence: 1}
	candidate := AnswerDiagramRelationRepairCandidate{BlockID: "d", FromIdentity: anchor.FromIdentity, ToIdentity: anchor.ToIdentity, RelationKind: DiagramRelWakeup, FromNodeIDs: []string{"A"}, ToNodeIDs: []string{"B"}, Source: "current native source:1"}
	for _, name := range []string{"valid", "wrong_occurrence", "wrong_tuple", "other_window", "other_source", "same_pair_other_event", "notes", "duplicate_body", "duplicate_anchor"} {
		t.Run(name, func(t *testing.T) {
			b := *base
			b.Blocks = append([]AnswerBlock(nil), base.Blocks...)
			d := *base.Blocks[0].Diagram
			b.Blocks[0].Diagram = &d
			f := failure
			switch name {
			case "wrong_occurrence":
				f.BodyOccurrence = 2
			case "wrong_tuple", "other_window", "other_source", "same_pair_other_event":
				f.FromIdentity += "_" + name // Native instance identities bind these independent producer coordinates.
			case "notes":
				d.RuntimeNotesV1 = []RuntimeDiagramNoteSelection{{}}
			case "duplicate_body":
				d.Body += "\n A->>B: event"
			case "duplicate_anchor":
				b.Blocks[0].EdgeAnchors = append(append([]DiagramEdgeAnchor(nil), b.Blocks[0].EdgeAnchors...), anchor)
			}
			lease := NewAnswerDiagramRelationRepairLease(&b, []AnswerDiagramRelationRepairFailure{f}, []AnswerDiagramRelationRepairCandidate{candidate})
			if name != "valid" {
				if lease != nil {
					t.Fatalf("ambiguous/stale carrier acquired local movement: %+v", lease)
				}
				return
			}
			if lease == nil || len(lease.AllowedAdditions) != 1 || !lease.Failures[0].AllowsAction("remove") || lease.Failures[0].AllowsAction("attach") || !AnswerDiagramNativeOrderFailureMatchesBase(&b, lease.Failures[0]) {
				t.Fatalf("current exact movement unavailable: %+v", lease)
			}
			if NewAnswerDiagramRelationRepairLease(&b, []AnswerDiagramRelationRepairFailure{f}, nil) != nil {
				t.Fatal("removal without current reinsertion acquired a local lease")
			}
			optional := *lease
			optional.AllowTargetDiagramRemoval = true
			if got := ValidateAnswerDiagramRelationRepairLease(&optional, &AnswerDocumentV2{}); len(got) != 0 {
				t.Fatalf("explicit whole optional diagram removal regressed: %+v", got)
			}
			if got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{}); len(got) == 0 {
				t.Fatal("required diagram removal was accepted")
			}
			for _, anchors := range [][]DiagramEdgeAnchor{nil, {anchor, anchor}} {
				changed := b
				changed.Blocks = append([]AnswerBlock(nil), b.Blocks...)
				changed.Blocks[0].EdgeAnchors = anchors
				if got := ValidateAnswerDiagramRelationRepairLease(lease, &changed); len(got) == 0 {
					t.Fatal("reordering lost/duplicated the original event")
				}
			}
			d.Body += "\nNote over A: changed"
			if AnswerDiagramNativeOrderFailureMatchesBase(&b, lease.Failures[0]) {
				t.Fatal("old failure ref survived changed body snapshot")
			}
		})
	}
}
