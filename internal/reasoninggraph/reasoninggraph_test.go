package reasoninggraph

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
)

func TestNormalizeEventsSortsTrimsAndBuildsStableIDs(t *testing.T) {
	events := NormalizeEvents([]ReasoningEvent{
		{
			GraphID:    " graph-1 ",
			NodeID:     " tool-1 ",
			Sequence:   2,
			Kind:       ReasoningEventToolCallRejected,
			ReasonCode: " schema_rejected ",
			ArtifactRefs: []loopkernel.ArtifactRef{
				{Kind: " plan ", ID: " p1 ", Path: " .codrax/plans/p1.json "},
				{Kind: " plan ", ID: " p1 ", Path: " .codrax/plans/p1.json "},
			},
			EvidenceRefs: []ReasoningEvidenceRef{
				{ID: " ev-1 ", Kind: " tool ", NodeID: " tool-1 ", Priority: " P0 ", Confidence: " HIGH "},
				{ID: " ev-1 ", Kind: " tool ", NodeID: " tool-1 ", Priority: " P0 ", Confidence: " HIGH "},
			},
		},
		{
			GraphID:  "graph-1",
			NodeID:   "tool-1",
			Sequence: 1,
			Kind:     ReasoningEventToolCallObserved,
		},
	})
	if len(events) != 2 {
		t.Fatalf("len(events)=%d", len(events))
	}
	if events[0].Kind != ReasoningEventToolCallObserved {
		t.Fatalf("events not sorted by sequence: %+v", events)
	}
	if got, want := events[1].ID, "graph-1:000002:tool_call_rejected"; got != want {
		t.Fatalf("stable ID mismatch: got %q want %q", got, want)
	}
	if got := events[1].ArtifactRefs; len(got) != 1 || got[0].Kind != "plan" || got[0].ID != "p1" {
		t.Fatalf("artifact refs not normalized/deduped: %+v", got)
	}
	if got := events[1].EvidenceRefs; len(got) != 1 || got[0].Priority != "p0" || got[0].Confidence != "high" {
		t.Fatalf("evidence refs not normalized/deduped: %+v", got)
	}
}

func TestReduceEventsSummarizesToolRepairAndLLMObservationLanes(t *testing.T) {
	now := time.Date(2026, 6, 20, 3, 0, 0, 0, time.UTC)
	events := []ReasoningEvent{
		NewObservationEvent(ObservationInput{
			GraphID:    "graph-1",
			NodeID:     "tool-emit-change-plan",
			NodeKind:   ReasoningNodeTool,
			Sequence:   1,
			Kind:       ReasoningEventToolCallObserved,
			ReasonCode: "tool_call_ok",
			Payload: ObservationPayload{
				InvocationID: "call-plan-1",
				ToolName:     "emit_change_plan",
				Agent:        "planner",
				Stage:        "plan",
				ParamsRef:    "blob://params",
				ResultRef:    "blob://result",
			},
			At: now,
		}),
		NewObservationEvent(ObservationInput{
			GraphID:    "graph-1",
			NodeID:     "repair-plan",
			NodeKind:   ReasoningNodeRepair,
			Sequence:   2,
			Kind:       ReasoningEventRepairPackEmitted,
			ReasonCode: "write_plan_repair_pack",
			Payload: ObservationPayload{
				ToolName:         "emit_change_plan",
				RepairCode:       "write_plan_repair_pack",
				ViolationKind:    "owner_evidence_missing",
				RepairLocus:      "planner",
				RepairReasonCode: "owner_anchor_required",
			},
			At: now.Add(time.Second),
		}),
		NewObservationEvent(ObservationInput{
			GraphID:    "graph-1",
			NodeID:     "llm-planner",
			NodeKind:   ReasoningNodeLLM,
			Sequence:   3,
			Kind:       ReasoningEventLLMRequestWaiting,
			ReasonCode: "llm_still_running",
			Payload: ObservationPayload{
				Agent:         "planner",
				Stage:         "plan",
				Model:         "test-model",
				Attempt:       1,
				MaxAttempts:   6,
				ElapsedMillis: 30000,
			},
			At: now.Add(2 * time.Second),
		}),
	}
	view := ReduceEvents(events)
	if view.GraphID != "graph-1" || view.EventCount != 3 || view.LastEventKind != ReasoningEventLLMRequestWaiting {
		t.Fatalf("unexpected view header: %+v", view)
	}
	if len(view.ToolEvents) != 1 || view.ToolEvents[0].ToolName != "emit_change_plan" {
		t.Fatalf("tool events not summarized: %+v", view.ToolEvents)
	}
	if view.ToolEvents[0].InvocationID != "call-plan-1" ||
		view.ToolEvents[0].ParamsRef != "blob://params" ||
		view.ToolEvents[0].ResultRef != "blob://result" {
		t.Fatalf("tool invocation refs not summarized: %+v", view.ToolEvents[0])
	}
	if len(view.RepairEvents) != 1 ||
		view.RepairEvents[0].RepairCode != "write_plan_repair_pack" ||
		view.RepairEvents[0].ViolationKind != "owner_evidence_missing" {
		t.Fatalf("repair events not summarized: %+v", view.RepairEvents)
	}
	if len(view.LLMEvents) != 1 ||
		view.LLMEvents[0].Agent != "planner" ||
		view.LLMEvents[0].ElapsedMillis != 30000 {
		t.Fatalf("LLM events not summarized: %+v", view.LLMEvents)
	}
	if len(view.Nodes) != 3 {
		t.Fatalf("nodes=%+v", view.Nodes)
	}
	if len(view.EventKindCounts) != 3 {
		t.Fatalf("event kind counts=%+v", view.EventKindCounts)
	}
}

func TestEventStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events", "graph-1.json")
	events := []ReasoningEvent{
		NewObservationEvent(ObservationInput{
			GraphID:    "graph-1",
			NodeID:     "tool-read-file",
			NodeKind:   ReasoningNodeTool,
			Sequence:   2,
			Kind:       ReasoningEventToolParamNormalized,
			ReasonCode: "schema_normalized",
			Payload: ObservationPayload{
				ToolName:          "read_file",
				OriginalByteLen:   41,
				NormalizedByteLen: 39,
			},
		}),
		{GraphID: "graph-1", Sequence: 1, Kind: ReasoningEventGraphSeeded},
	}
	if err := WriteEventsToFile(events, path); err != nil {
		t.Fatalf("WriteEventsToFile: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file leaked: %v", err)
	}
	got, err := LoadEventsFromFile(path)
	if err != nil {
		t.Fatalf("LoadEventsFromFile: %v", err)
	}
	if len(got) != 2 || got[0].Kind != ReasoningEventGraphSeeded || got[1].Kind != ReasoningEventToolParamNormalized {
		t.Fatalf("roundtrip events=%+v", got)
	}
	view := ReduceEvents(got)
	if len(view.ToolEvents) != 1 || view.ToolEvents[0].ToolName != "read_file" {
		t.Fatalf("roundtrip view=%+v", view)
	}
}
