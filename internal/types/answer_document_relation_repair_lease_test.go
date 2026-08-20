package types

import "testing"

func TestAnswerDiagramRelationRepairLease_LocalDeltaOnly(t *testing.T) {
	bad := DiagramEdgeAnchor{
		FromNode: "Analyze", ToNode: "Explore", FromIdentity: "runAnalyzePhase", ToIdentity: "runExplorePhase",
		RelationKind: DiagramRelCall, VisibleLabel: "old wording",
	}
	good := DiagramEdgeAnchor{
		FromNode: "Explore", ToNode: "Extract", FromIdentity: "runExplorePhase", ToIdentity: "runExtractPhase",
		RelationKind: DiagramRelCall, VisibleLabel: "old wording",
	}
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{bad, good}}}}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelCall,
		FromNode: "Analyze", ToNode: "Explore", FromIdentity: "runAnalyzePhase", ToIdentity: "runExplorePhase",
	}})
	if lease == nil {
		t.Fatal("expected a lease from a valid typed failure")
	}

	t.Run("remove only failed relation and relabel retained relation", func(t *testing.T) {
		relabelled := good
		relabelled.VisibleLabel = "business wording stays model-owned"
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{relabelled},
		}}})
		if len(got) != 0 {
			t.Fatalf("local removal plus display relabel must pass: %+v", got)
		}
	})

	t.Run("correct failed relation on same endpoint pair", func(t *testing.T) {
		corrected := bad
		corrected.FromNode, corrected.ToNode = corrected.ToNode, corrected.FromNode
		corrected.FromIdentity, corrected.ToIdentity = corrected.ToIdentity, corrected.FromIdentity
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{corrected, good},
		}}})
		if len(got) != 0 {
			t.Fatalf("same-pair direction correction must remain model-owned: %+v", got)
		}
	})

	t.Run("removing unlisted relation is rejected", func(t *testing.T) {
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "flow"}}})
		if len(got) != 1 || got[0].Issue != "unlisted_relation_removed" || got[0].FromNode != "Explore" {
			t.Fatalf("expected precise unlisted removal, got %+v", got)
		}
	})

	t.Run("adding unrelated relation is rejected", func(t *testing.T) {
		unrelated := DiagramEdgeAnchor{FromNode: "Extract", ToNode: "Finalize", RelationKind: DiagramRelCall}
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{good, unrelated},
		}}})
		if len(got) != 1 || got[0].Issue != "unlisted_relation_added" || got[0].FromNode != "Extract" {
			t.Fatalf("expected precise unlisted addition, got %+v", got)
		}
	})

	t.Run("retaining failure cannot expand same pair", func(t *testing.T) {
		duplicate := bad
		duplicate.RelationKind = DiagramRelAssignment
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{bad, duplicate, good},
		}}})
		if len(got) != 1 || got[0].Issue != "failed_relation_expanded" {
			t.Fatalf("expected no-expansion violation, got %+v", got)
		}
	})
}

func TestMutableState_AnswerDiagramRelationRepairLeaseLifecycle(t *testing.T) {
	m := NewMutableState("test")
	lease := &AnswerDiagramRelationRepairLease{Version: 1, Failures: []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
	}}}
	m.SetAnswerDiagramRelationRepairLease(lease)
	got := m.AnswerDiagramRelationRepairLease()
	if got == nil || len(got.Failures) != 1 {
		t.Fatalf("lease was not retained: %+v", got)
	}
	got.Failures[0].FromNode = "mutated"
	if fresh := m.AnswerDiagramRelationRepairLease(); fresh.Failures[0].FromNode != "A" {
		t.Fatal("lease getter must return a defensive copy")
	}
	m.SetAnswerDocumentV2WithMutation(MutationReplaceAll, &AnswerDocumentV2{DocumentModel: "v2"})
	if got := m.AnswerDiagramRelationRepairLease(); got != nil {
		t.Fatalf("accepted emit must clear retry-local lease: %+v", got)
	}
}
