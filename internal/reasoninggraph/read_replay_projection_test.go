package reasoninggraph

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventsFromReadRunReplayAuditProjectsReadEvent(t *testing.T) {
	audit := types.BuildReadRunReplayAudit(&types.ReadRunSnapshot{
		SchemaVersion: types.ReadRunSnapshotSchemaVersion,
		RunID:         "run-before",
		RequestHash:   "req",
		TaskGraphHash: "graph",
		NodeStatuses:  map[string]types.NodeExecStatus{"explore": types.NodeExecRunning},
	}, &types.ReadRunSnapshot{
		SchemaVersion: types.ReadRunSnapshotSchemaVersion,
		RunID:         "run-after",
		RequestHash:   "req",
		TaskGraphHash: "graph",
		NodeStatuses:  map[string]types.NodeExecStatus{"explore": types.NodeExecRequeued},
	}, []types.ReadRunReplayTransition{{
		NodeID:     "explore",
		FromStatus: types.NodeExecRunning,
		ToStatus:   types.NodeExecRequeued,
		ReasonCode: types.ReadRunReplayReasonRunningInterrupted,
	}})

	events := EventsFromReadRunReplayAudit("read-replay-graph", audit)
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	view := ReduceEvents(events)
	if view.GraphID != "read-replay-graph" ||
		view.EventCount != 2 ||
		len(view.ReadEvents) != 1 ||
		!reasoningGraphViewHasKind(view, ReasoningEventReadReplayAuditProjected) {
		t.Fatalf("view did not classify replay audit as read event: %+v", view)
	}
	if view.ReadEvents[0].ReasonCode != "read_replay_transition_projected" {
		t.Fatalf("read replay reason = %+v", view.ReadEvents[0])
	}
	var projected types.ReadRunReplayAudit
	if err := json.Unmarshal(events[1].Payload, &projected); err != nil {
		t.Fatalf("payload is not replay audit JSON: %v payload=%s", err, events[1].Payload)
	}
	projected = types.NormalizeReadRunReplayAudit(projected)
	if projected.RequeuedFromRunning != 1 ||
		projected.TaskGraphHashMatch != types.ReadRunReplayAuditMatchEqual ||
		projected.RequestHashMatch != types.ReadRunReplayAuditMatchEqual {
		t.Fatalf("projected audit wrong: %+v", projected)
	}
}

func TestEventsFromReadRunReplayAuditIdentityMismatchReason(t *testing.T) {
	audit := types.BuildReadRunReplayAudit(&types.ReadRunSnapshot{
		RunID:         "before",
		TaskGraphHash: "old",
		RequestHash:   "same",
	}, &types.ReadRunSnapshot{
		RunID:         "after",
		TaskGraphHash: "new",
		RequestHash:   "same",
	}, nil)
	events := EventsFromReadRunReplayAudit("", audit)
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	if events[1].GraphID != "after" || events[1].ReasonCode != "read_replay_identity_mismatch" {
		t.Fatalf("identity mismatch projection wrong: %+v", events[1])
	}
	view := ReduceEvents(events)
	if len(view.ReadEvents) != 1 || view.ReadEvents[0].Kind != ReasoningEventReadReplayAuditProjected {
		t.Fatalf("read events = %+v", view.ReadEvents)
	}
}
