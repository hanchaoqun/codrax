package types

import "testing"

func TestAnswerDiagramParticipantVisibilityRepairLease_PublishesOnlyExactNodeMissingRows(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		Diagram: &AnswerDiagramBlock{Kind: DiagramFlow, Language: "mermaid", Body: "flowchart TD\n A-->B"},
		ParticipantBoundaries: []DiagramParticipantBoundary{{
			Participant: "BusContext", Status: DiagramParticipantBoundaryUnproven,
		}},
	}}}
	lease := WithAnswerDiagramParticipantVisibilityRepairFailures(base, nil,
		[]AnswerDiagramParticipantVisibilityRepairFailure{
			{BlockID: "flow", Participant: "BusContext", Issue: "boundary_participant_not_visible"},
			{BlockID: "flow", Participant: "Mutable", Issue: "required_participant_identity_not_visible"},
		})
	if lease == nil || len(lease.ParticipantVisibilityFailures) != 1 {
		t.Fatalf("expected one exact visibility capability: %+v", lease)
	}
	failure := lease.ParticipantVisibilityFailures[0]
	if failure.ParticipantRef == "" || failure.BaseDiagramFingerprint == "" ||
		!failure.AllowsParticipantAction("ensure_visible") {
		t.Fatalf("visibility capability is not generation-bound and executable: %+v", failure)
	}
	repeat := WithAnswerDiagramParticipantVisibilityRepairFailures(base, nil,
		[]AnswerDiagramParticipantVisibilityRepairFailure{{
			BlockID: "flow", Participant: "BusContext", Issue: "boundary_participant_not_visible",
		}})
	if repeat == nil || repeat.ParticipantVisibilityFailures[0].ParticipantRef != failure.ParticipantRef {
		t.Fatalf("same immutable base must yield stable refs: first=%+v repeat=%+v", failure, repeat)
	}
	changed := cloneAnswerDocumentV2(base)
	changed.Blocks[0].Diagram.Body += "\n C"
	if got := WithAnswerDiagramParticipantVisibilityRepairFailures(changed, lease,
		[]AnswerDiagramParticipantVisibilityRepairFailure{{
			BlockID: "flow", Participant: "BusContext", Issue: "boundary_participant_not_visible",
		}}); got != nil {
		t.Fatalf("another diagram generation must reject the stale lease: %+v", got)
	}
}
