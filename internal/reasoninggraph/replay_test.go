package reasoninggraph

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func replayTestEvents() []ReasoningEvent {
	return []ReasoningEvent{
		NewObservationEvent(ObservationInput{
			GraphID:    "graph-replay",
			NodeID:     "tool-read-file",
			NodeKind:   ReasoningNodeTool,
			Sequence:   1,
			Kind:       ReasoningEventToolCallObserved,
			ReasonCode: "tool_call_ok",
			Payload: ObservationPayload{
				InvocationID: "call-read-1",
				ToolName:     "read_file",
				Stage:        "explore",
				ParamsRef:    "blob://read-params",
				ResultRef:    "blob://read-result",
			},
		}),
		NewObservationEvent(ObservationInput{
			GraphID:    "graph-replay",
			NodeID:     "repair-plan",
			NodeKind:   ReasoningNodeRepair,
			Sequence:   2,
			Kind:       ReasoningEventRepairPackEmitted,
			ReasonCode: "schema_repaired",
			Payload: ObservationPayload{
				RepairCode: "schema_repaired",
			},
		}),
	}
}

func TestGraphReplayExecutorRecomputesViewAndAuditReadOnly(t *testing.T) {
	got, err := (GraphReplayExecutor{}).Replay(context.Background(), GraphReplayRequest{
		Events:     replayTestEvents(),
		Source:     "unit_test",
		AuditLimit: 4,
	})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if !got.ReadOnly || got.ReplayMode != "full" {
		t.Fatalf("replay result should be read-only full mode: %+v", got)
	}
	if got.View.EventCount != 2 || len(got.View.ToolEvents) != 1 || len(got.View.RepairEvents) != 1 {
		t.Fatalf("view not recomputed from events: %+v", got.View)
	}
	if got.View.ToolEvents[0].InvocationID != "call-read-1" ||
		got.View.ToolEvents[0].ParamsRef != "blob://read-params" ||
		got.View.ToolEvents[0].ResultRef != "blob://read-result" {
		t.Fatalf("tool invocation refs missing from replay view: %+v", got.View.ToolEvents[0])
	}
	if got.Audit == nil || got.Audit.Source != "unit_test" || got.Audit.EventCount != 2 {
		t.Fatalf("audit not recomputed from view: %+v", got.Audit)
	}
	if len(got.Audit.RecentEvents) == 0 ||
		got.Audit.RecentEvents[0].InvocationID != "call-read-1" ||
		got.Audit.RecentEvents[0].ParamsRef != "blob://read-params" ||
		got.Audit.RecentEvents[0].ResultRef != "blob://read-result" {
		t.Fatalf("tool invocation refs missing from replay audit: %+v", got.Audit.RecentEvents)
	}
	invocation, ok := FindToolInvocation(got.View, "call-read-1")
	if !ok || invocation.ToolName != "read_file" || invocation.ParamsRef != "blob://read-params" {
		t.Fatalf("FindToolInvocation = %+v, %v", invocation, ok)
	}
}

func TestGraphReplayExecutorNodeLocalRecompute(t *testing.T) {
	got, err := (GraphReplayExecutor{}).Replay(context.Background(), GraphReplayRequest{
		Events: replayTestEvents(),
		NodeID: "repair-plan",
	})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if got.ReplayMode != "node" || got.NodeID != "repair-plan" {
		t.Fatalf("node replay metadata mismatch: %+v", got)
	}
	if got.View.EventCount != 1 || len(got.View.RepairEvents) != 1 || len(got.View.ToolEvents) != 0 {
		t.Fatalf("node replay should only reduce selected node events: %+v", got.View)
	}
}

func TestGraphReplayExecutorLoadsEventsFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := WriteEventsToFile(replayTestEvents(), path); err != nil {
		t.Fatalf("WriteEventsToFile: %v", err)
	}
	got, err := (GraphReplayExecutor{}).Replay(context.Background(), GraphReplayRequest{EventFile: path})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if got.View.EventCount != 2 {
		t.Fatalf("file replay event count = %d", got.View.EventCount)
	}
}

func TestGraphReplayExecutorBudgetAndCancellation(t *testing.T) {
	_, err := (GraphReplayExecutor{}).Replay(context.Background(), GraphReplayRequest{
		Events:    replayTestEvents(),
		MaxEvents: 1,
	})
	if err == nil {
		t.Fatal("budget overflow should fail loud")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (GraphReplayExecutor{}).Replay(ctx, GraphReplayRequest{Events: replayTestEvents()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled replay err=%v", err)
	}
}

func TestGraphReplayExecutorIdempotent(t *testing.T) {
	exec := GraphReplayExecutor{}
	req := GraphReplayRequest{Events: replayTestEvents(), AuditLimit: 2}
	first, err := exec.Replay(context.Background(), req)
	if err != nil {
		t.Fatalf("first Replay: %v", err)
	}
	second, err := exec.Replay(context.Background(), req)
	if err != nil {
		t.Fatalf("second Replay: %v", err)
	}
	if !reflect.DeepEqual(first.View, second.View) || !reflect.DeepEqual(first.Audit, second.Audit) {
		t.Fatalf("replay must be idempotent:\nfirst=%+v\nsecond=%+v", first, second)
	}
}
