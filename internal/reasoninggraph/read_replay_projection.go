package reasoninggraph

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func EventsFromReadRunReplayAudit(graphID string, audit types.ReadRunReplayAudit) []ReasoningEvent {
	audit = types.NormalizeReadRunReplayAudit(audit)
	graphID = strings.TrimSpace(graphID)
	if graphID == "" {
		graphID = firstNonEmpty(audit.CurrentRunID, audit.BaseRunID, "read-replay")
	}
	events := []ReasoningEvent{
		{
			GraphID:    graphID,
			NodeID:     "read:run",
			NodeKind:   ReasoningNodeAnalyze,
			Sequence:   1,
			Kind:       ReasoningEventGraphSeeded,
			ReasonCode: "read_replay_graph_seeded",
		},
		{
			GraphID:    graphID,
			NodeID:     "read:replay",
			NodeKind:   ReasoningNodeEvidence,
			Sequence:   2,
			Kind:       ReasoningEventReadReplayAuditProjected,
			ReasonCode: readReplayAuditReasonCode(audit),
			Payload:    eventPayload(audit),
		},
	}
	return NormalizeEvents(events)
}

func readReplayAuditReasonCode(audit types.ReadRunReplayAudit) string {
	audit = types.NormalizeReadRunReplayAudit(audit)
	if audit.TaskGraphHashMatch == types.ReadRunReplayAuditMatchMismatch ||
		audit.RequestHashMatch == types.ReadRunReplayAuditMatchMismatch ||
		audit.RepoFingerprintMatch == types.ReadRunReplayAuditMatchMismatch {
		return "read_replay_identity_mismatch"
	}
	if audit.RequeuedFromRunning > 0 ||
		audit.DoneRequeuedMissingArtifact > 0 ||
		audit.FailedRequeued > 0 ||
		audit.FailedExhausted > 0 {
		return "read_replay_transition_projected"
	}
	return "read_replay_audit_projected"
}
