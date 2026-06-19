package reasoninggraph

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

func EventsFromWriteWorkflowRun(run types.WriteWorkflowRun) []ReasoningEvent {
	return EventsFromLoopEvents(loopkernel.EventsFromWriteWorkflowRun(run))
}

func EventsFromLoopEvents(events []loopkernel.LoopEvent) []ReasoningEvent {
	loopEvents := loopkernel.NormalizeLoopEvents(events)
	out := make([]ReasoningEvent, 0, len(loopEvents))
	for _, event := range loopEvents {
		out = append(out, NormalizeEvent(ReasoningEvent{
			ID:           strings.TrimSpace(event.ID),
			GraphID:      strings.TrimSpace(event.RunID),
			NodeID:       loopReasoningNodeID(event),
			NodeKind:     loopReasoningNodeKind(event.Kind),
			Sequence:     event.Sequence,
			Kind:         loopReasoningEventKind(event.Kind),
			ReasonCode:   strings.TrimSpace(event.ReasonCode),
			ArtifactRefs: append([]loopkernel.ArtifactRef(nil), event.ArtifactRefs...),
			Payload:      cloneRawMessage(event.Payload),
			At:           event.At,
		}))
	}
	return NormalizeEvents(out)
}

func loopReasoningNodeID(event loopkernel.LoopEvent) string {
	unitID := strings.TrimSpace(event.UnitID)
	if unitID == "" {
		return "workflow:run"
	}
	return "workflow:" + unitID
}

func loopReasoningEventKind(kind loopkernel.LoopEventKind) ReasoningEventKind {
	switch kind {
	case loopkernel.LoopEventRunSeeded:
		return ReasoningEventGraphSeeded
	case loopkernel.LoopEventLocalizationProjected,
		loopkernel.LoopEventProofProjected,
		loopkernel.LoopEventPermissionDecided,
		loopkernel.LoopEventTruthProjected:
		return ReasoningEventAuthorityProjected
	case loopkernel.LoopEventEffectDescribed,
		loopkernel.LoopEventPatchEffectRecorded:
		return ReasoningEventPatchEffectProjected
	case loopkernel.LoopEventRepairRequested:
		return ReasoningEventRepairRequested
	default:
		return ReasoningEventWorkflowEventProjected
	}
}

func loopReasoningNodeKind(kind loopkernel.LoopEventKind) ReasoningNodeKind {
	switch kind {
	case loopkernel.LoopEventContextRequested,
		loopkernel.LoopEventContextObserved,
		loopkernel.LoopEventLocalizationProjected:
		return ReasoningNodeExplore
	case loopkernel.LoopEventPlanRequested,
		loopkernel.LoopEventPlanEmitted:
		return ReasoningNodePlan
	case loopkernel.LoopEventEffectDescribed,
		loopkernel.LoopEventCheckpointCreated,
		loopkernel.LoopEventUnitApplyStarted,
		loopkernel.LoopEventUnitApplyCompleted,
		loopkernel.LoopEventPatchEffectRecorded:
		return ReasoningNodeApply
	case loopkernel.LoopEventPermissionDecided,
		loopkernel.LoopEventApprovalRequested,
		loopkernel.LoopEventApprovalResolved:
		return ReasoningNodeOperation
	case loopkernel.LoopEventObserveStarted,
		loopkernel.LoopEventObserveCompleted,
		loopkernel.LoopEventProofProjected,
		loopkernel.LoopEventTruthProjected,
		loopkernel.LoopEventUnitCompleted,
		loopkernel.LoopEventUnitBlocked:
		return ReasoningNodeVerify
	case loopkernel.LoopEventRepairRequested:
		return ReasoningNodeRepair
	default:
		return ReasoningNodeEvidence
	}
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	return json.RawMessage(append([]byte(nil), in...))
}
