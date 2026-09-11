package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func b1657BoundMetadata(t *testing.T) (*AnswerDocumentV2, *AnswerDiagramRelationRepairLease, AnswerDiagramOrphanCleanupCandidate) {
	t.Helper()
	base := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{ID: "d", Kind: BlockDiagram,
		Diagram: &AnswerDiagramBlock{Kind: DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n participant X\n participant Y\n participant Z\n"},
		EdgeAnchors: []DiagramEdgeAnchor{
			{FromNode: "X", ToNode: "Y", FromIdentity: "A", ToIdentity: "B", RelationKind: DiagramRelCall, VisibleLabel: "first"},
			{FromNode: "X", ToNode: "Y", FromIdentity: "A", ToIdentity: "B", RelationKind: DiagramRelCall, VisibleLabel: "second"},
		},
	}}}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{
		{BlockID: "d", Issue: "typed_anchor_without_visible_edge", FromNode: "X", ToNode: "Y", RelationKind: DiagramRelCall, AnchorOccurrence: 1},
		{BlockID: "d", Issue: "typed_anchor_without_visible_edge", FromNode: "X", ToNode: "Y", RelationKind: DiagramRelCall, AnchorOccurrence: 2},
	}, nil)
	candidate, ok := BindAnswerDiagramOrphanMetadataDependency(base, AnswerDiagramOrphanCleanupCandidate{
		BlockID: "d", ParticipantID: "X", AllowedActions: []AnswerDiagramOrphanDispositionAction{AnswerDiagramOrphanDispositionRemove, AnswerDiagramOrphanDispositionRetain},
	}, lease)
	if !ok || !AnswerDiagramOrphanMetadataDependencyMatchesBase(base, candidate, lease) {
		t.Fatal("exact metadata binding failed")
	}
	lease.OptionalOrphanCleanups = []AnswerDiagramOrphanCleanupCandidate{candidate}
	return base, lease, candidate
}

func TestB1657MetadataReceiptCannotBeForgedThroughJSON(t *testing.T) {
	base, lease, candidate := b1657BoundMetadata(t)
	wire, err := json.Marshal(candidate)
	if err != nil || !strings.Contains(string(wire), `"decision_optional":true`) || strings.Contains(string(wire), candidate.MetadataDependency.baseFingerprint) || strings.Contains(string(wire), "rf1-") {
		t.Fatalf("read-only hint/private source serialization: %v %s", err, wire)
	}
	for _, raw := range []string{string(wire), `{"block_id":"d","participant_id":"X","allowed_actions":["remove_if_isolated"],"decision_optional":true,"MetadataDependency":{"baseFingerprint":"fake","failureRefs":["fake"]}}`} {
		var decoded AnswerDiagramOrphanCleanupCandidate
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.MetadataDependency != nil || AnswerDiagramOrphanMetadataDependencyMatchesBase(base, decoded, lease) {
			t.Fatal("model JSON created private cleanup permission")
		}
		again, _ := json.Marshal(decoded)
		if strings.Contains(string(again), "decision_optional") {
			t.Fatalf("decoded hint became a durable optional authority: %s", again)
		}
	}
}

func TestB1657MetadataReceiptRequiresTheCompleteExactCurrentBase(t *testing.T) {
	for _, change := range []string{"body", "anchor_order", "anchor_label", "anchor_identity", "missing_ref", "wrong_ref", "missing_anchor", "duplicate_block", "participant", "sibling_participant", "no_receipt"} {
		t.Run(change, func(t *testing.T) {
			base, lease, candidate := b1657BoundMetadata(t)
			switch change {
			case "body":
				base.Blocks[0].Diagram.Body += " X->>Y: newly connected\n"
			case "anchor_order":
				base.Blocks[0].EdgeAnchors[0], base.Blocks[0].EdgeAnchors[1] = base.Blocks[0].EdgeAnchors[1], base.Blocks[0].EdgeAnchors[0]
			case "anchor_label":
				base.Blocks[0].EdgeAnchors[0].VisibleLabel = "different opaque bytes"
			case "anchor_identity":
				base.Blocks[0].EdgeAnchors[0].FromIdentity = "DifferentMethod"
			case "missing_ref":
				lease.Failures = lease.Failures[:1]
			case "wrong_ref":
				lease.Failures[1].FailureRef = "old-ref"
			case "missing_anchor":
				base.Blocks[0].EdgeAnchors = base.Blocks[0].EdgeAnchors[:1]
			case "duplicate_block":
				base.Blocks = append(base.Blocks, base.Blocks[0])
			case "participant":
				candidate.ParticipantID = "Z"
			case "sibling_participant":
				candidate.ParticipantID = "Y"
			case "no_receipt":
				candidate.MetadataDependency = nil
			}
			if AnswerDiagramOrphanMetadataDependencyMatchesBase(base, candidate, lease) {
				t.Fatal("mismatched source granted cleanup permission")
			}
		})
	}
}

func TestB1657MetadataReceiptMutableCopiesAndDispatchCarryAreIndependent(t *testing.T) {
	base, lease, candidate := b1657BoundMetadata(t)
	mu := NewMutableState("private metadata receipt")
	mu.SetAnswerDiagramRelationRepairLease(lease)
	candidate.MetadataDependency.failureRefs[0] = "mutated-input"
	got := mu.AnswerDiagramRelationRepairLease()
	if !AnswerDiagramOrphanMetadataDependencyMatchesBase(base, got.OptionalOrphanCleanups[0], got) {
		t.Fatal("setter aliased caller's receipt")
	}
	got.OptionalOrphanCleanups[0].MetadataDependency.failureRefs[0] = "mutated-output"
	fresh := mu.AnswerDiagramRelationRepairLease()
	if !AnswerDiagramOrphanMetadataDependencyMatchesBase(base, fresh.OptionalOrphanCleanups[0], fresh) {
		t.Fatal("getter aliased Mutable's receipt")
	}
	target := NewAnswerDiagramRelationRepairLease(base, fresh.Failures, nil)
	carried := CarryAnswerDiagramOrphanMetadataDependencies(base, fresh, target)
	if len(carried) != 1 || !AnswerDiagramOrphanMetadataDependencyMatchesBase(base, carried[0], target) {
		t.Fatal("exact current redispatch lost private receipt")
	}
	carried[0].MetadataDependency.failureRefs[0] = "mutated-carried"
	if !AnswerDiagramOrphanMetadataDependencyMatchesBase(base, fresh.OptionalOrphanCleanups[0], fresh) {
		t.Fatal("carry aliased source")
	}
	if len(CarryAnswerDiagramOrphanMetadataDependencies(base, nil, target)) != 0 {
		t.Fatal("a new delta invented source without a current installed receipt")
	}
}
