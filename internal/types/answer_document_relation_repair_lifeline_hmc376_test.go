package types

import "testing"

// One recorded arrow on another thread's lifeline keeps its proved tuple and
// only needs its visible endpoint moved. The lease therefore publishes an
// exact replace beside remove on the unique prior anchor, and nothing more.
func TestHMC376RuntimeLifelineIssueLeasePublishesExactReplace(t *testing.T) {
	first := DiagramEdgeAnchor{FromNode: "Helpers", ToNode: "Main", FromIdentity: "runtime_instance_a_from", ToIdentity: "runtime_instance_a_to", RelationKind: DiagramRelWakeup, VisibleLabel: "wake 1"}
	odd := DiagramEdgeAnchor{FromNode: "Helpers", ToNode: "Main", FromIdentity: "runtime_instance_b_from", ToIdentity: "runtime_instance_b_to", RelationKind: DiagramRelWakeup, VisibleLabel: "wake 2"}
	base := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{ID: "diagram", Kind: BlockDiagram, EdgeAnchors: []DiagramEdgeAnchor{first, odd},
		Diagram: &AnswerDiagramBlock{Kind: DiagramSequence, Body: "sequenceDiagram\n    Helpers->>Main: wake 1\n    Helpers->>Main: wake 2\n"}}}}
	failure := func(issue string) AnswerDiagramRelationRepairFailure {
		return AnswerDiagramRelationRepairFailure{BlockID: "diagram", Issue: issue, FromNode: odd.FromNode, ToNode: odd.ToNode,
			FromIdentity: odd.FromIdentity, ToIdentity: odd.ToIdentity, RelationKind: odd.RelationKind}
	}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{failure(DiagramRelationFailureRuntimeLifelineTwoThreads)}, nil)
	if lease == nil || len(lease.Failures) != 1 {
		t.Fatalf("no lease for the lifeline issue: %+v", lease)
	}
	got := lease.Failures[0]
	if got.Issue != "runtime_lifeline_hosts_two_threads" || got.TargetCarrier != AnswerDiagramRelationRepairCarrierPriorAnchor || got.FailureRef == "" ||
		len(got.AllowedActions) != 2 || !got.AllowsAction(string(AnswerDiagramRelationRepairActionRemove)) || !got.AllowsAction(string(AnswerDiagramRelationRepairActionReplace)) {
		t.Fatalf("lifeline issue must publish remove and replace on the exact prior anchor: %+v", got)
	}
	// The failure selects the odd arrow only, never the arrow that shares its
	// visible endpoints.
	if matches := AnswerDiagramRelationRepairFailureAnchorCandidates(got, base.Blocks[0].EdgeAnchors); len(matches) != 1 || matches[0] != odd {
		t.Fatalf("failure does not select exactly the odd arrow: %+v", matches)
	}
	// Moving that arrow to another lifeline stays inside the lease. The
	// unlisted arrow is not a donor: dropping it to keep both forms of the
	// listed arrow does not.
	moved := odd
	moved.FromNode = "Helper2"
	merged := func(anchors ...DiagramEdgeAnchor) *AnswerDocumentV2 {
		return &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{ID: "diagram", Kind: BlockDiagram, EdgeAnchors: anchors, Diagram: base.Blocks[0].Diagram}}}
	}
	if violations := ValidateAnswerDiagramRelationRepairLease(lease, merged(first, moved)); len(violations) != 0 {
		t.Fatalf("moving the listed arrow escaped the lease: %+v", violations)
	}
	if violations := ValidateAnswerDiagramRelationRepairLease(lease, merged(odd, moved)); len(violations) == 0 {
		t.Fatal("an unlisted recorded arrow was traded for a second copy of the listed arrow")
	}
	// A newly minted issue name without a decision stays remove-only.
	unknown := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{failure("runtime_lifeline_unlisted_probe")}, nil)
	if unknown == nil || len(unknown.Failures) != 1 || unknown.Failures[0].AllowsAction(string(AnswerDiagramRelationRepairActionReplace)) {
		t.Fatalf("an undecided issue acquired replace: %+v", unknown)
	}
}
