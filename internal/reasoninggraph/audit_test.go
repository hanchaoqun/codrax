package reasoninggraph

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAuditSummaryFromEventsProjectsTypedLanes(t *testing.T) {
	base := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	events := []ReasoningEvent{
		NewObservationEvent(ObservationInput{
			GraphID:    "graph-audit",
			NodeID:     "planner",
			NodeKind:   ReasoningNodePlan,
			Sequence:   1,
			Kind:       ReasoningEventSchemaRejected,
			ReasonCode: "emit_change_plan_schema_rejected",
			Payload: ObservationPayload{
				ToolName:      "emit_change_plan",
				RepairCode:    "missing_required_field",
				ViolationKind: "schema",
			},
			At: base,
		}),
		NewObservationEvent(ObservationInput{
			GraphID:    "graph-audit",
			NodeID:     "llm",
			NodeKind:   ReasoningNodeLLM,
			Sequence:   2,
			Kind:       ReasoningEventLLMRequestWaiting,
			ReasonCode: "llm_slow_response",
			Payload: ObservationPayload{
				Agent:         "planner",
				Model:         "test-model",
				ElapsedMillis: 1500,
			},
			At: base.Add(time.Second),
		}),
		NewObservationEvent(ObservationInput{
			GraphID:    "graph-audit",
			NodeID:     "verify",
			NodeKind:   ReasoningNodeVerify,
			Sequence:   3,
			Kind:       ReasoningEventAuthorityProjected,
			ReasonCode: "tests_passed",
			At:         base.Add(2 * time.Second),
		}),
	}

	got := AuditSummaryFromEvents(events, "unit_test", 2)
	if got == nil {
		t.Fatal("audit summary nil")
	}
	if got.Source != "unit_test" || got.Status != types.ReasoningGraphAuditObserved {
		t.Fatalf("audit identity = %+v", got)
	}
	if got.EventCount != 3 || got.NodeCount != 3 || got.LastEventKind != string(ReasoningEventAuthorityProjected) || got.LastReasonCode != "tests_passed" {
		t.Fatalf("audit counters = %+v", got)
	}
	if laneCount(got.Lanes, "repair") != 1 || laneCount(got.Lanes, "llm") != 1 || laneCount(got.Lanes, "workflow") != 1 {
		t.Fatalf("audit lanes = %+v", got.Lanes)
	}
	if len(got.RepairEvents) != 1 || got.RepairEvents[0].ToolName != "emit_change_plan" || got.RepairEvents[0].RepairCode != "missing_required_field" {
		t.Fatalf("repair events = %+v", got.RepairEvents)
	}
	if len(got.LLMEvents) != 1 || got.LLMEvents[0].ReasonCode != "llm_slow_response" || got.LLMEvents[0].ElapsedMillis != 1500 {
		t.Fatalf("llm events = %+v", got.LLMEvents)
	}
	if len(got.RecentEvents) != 2 || got.RecentEvents[1].ReasonCode != "tests_passed" {
		t.Fatalf("recent events = %+v", got.RecentEvents)
	}
}

func laneCount(lanes []types.ReasoningGraphAuditLane, name string) int {
	for _, lane := range lanes {
		if lane.Name == name {
			return lane.Count
		}
	}
	return 0
}
