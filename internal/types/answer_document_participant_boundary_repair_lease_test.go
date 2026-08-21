package types

import "testing"

func TestAnswerDiagramParticipantBoundaryRepairLease_PublishesOnlyExecutableLocalRows(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		Diagram: &AnswerDiagramBlock{Kind: DiagramFlow, Language: "mermaid", Body: "flowchart TD\n A-->B"},
		ParticipantBoundaries: []DiagramParticipantBoundary{
			{Participant: "Analyzer", Status: DiagramParticipantBoundaryUnproven},
			{Participant: "Keep", Status: DiagramParticipantBoundaryUnproven},
		},
	}}}
	lease := WithAnswerDiagramParticipantBoundaryRepairFailures(base, nil, []AnswerDiagramParticipantBoundaryRepairFailure{
		{BlockID: "flow", Participant: "Analyzer", Issue: "stale_boundary_for_connected_participant"},
		{BlockID: "flow", Participant: "Explorer", Issue: "missing_unproven_boundary"},
		{BlockID: "flow", Participant: "Mutable", Issue: "required_participant_identity_not_visible"},
	})
	if lease == nil || len(lease.ParticipantBoundaryFailures) != 2 {
		t.Fatalf("expected exactly the executable add/remove boundary rows: %+v", lease)
	}
	seen := make(map[string]AnswerDiagramParticipantBoundaryRepairFailure)
	for _, failure := range lease.ParticipantBoundaryFailures {
		if failure.BoundaryRef == "" || failure.BaseBoundaryFingerprint == "" || len(failure.AllowedBoundaryActions) != 1 {
			t.Fatalf("boundary capability must be generation-bound and single-action: %+v", failure)
		}
		seen[failure.Participant] = failure
	}
	if !seen["Analyzer"].AllowsBoundaryAction("remove_boundary") ||
		!seen["Explorer"].AllowsBoundaryAction("add_unproven") {
		t.Fatalf("unexpected action mapping: %+v", seen)
	}
	if _, exists := seen["Mutable"]; exists {
		t.Fatalf("visible identity repair must not receive a boundary-only capability: %+v", seen)
	}

	repeat := WithAnswerDiagramParticipantBoundaryRepairFailures(base, nil, []AnswerDiagramParticipantBoundaryRepairFailure{
		{BlockID: "flow", Participant: "Analyzer", Issue: "stale_boundary_for_connected_participant"},
	})
	if repeat == nil || repeat.ParticipantBoundaryFailures[0].BoundaryRef != seen["Analyzer"].BoundaryRef {
		t.Fatalf("same typed base must yield a stable opaque ref: first=%+v repeat=%+v", seen["Analyzer"], repeat)
	}
	changed := cloneAnswerDocumentV2(base)
	changed.Blocks[0].ParticipantBoundaries = append(changed.Blocks[0].ParticipantBoundaries,
		DiagramParticipantBoundary{Participant: "Other", Status: DiagramParticipantBoundaryUnproven})
	if got := WithAnswerDiagramParticipantBoundaryRepairFailures(changed, lease, []AnswerDiagramParticipantBoundaryRepairFailure{
		{BlockID: "flow", Participant: "Analyzer", Issue: "stale_boundary_for_connected_participant"},
	}); got != nil {
		t.Fatalf("a lease from another boundary generation must fail closed: %+v", got)
	}
}
