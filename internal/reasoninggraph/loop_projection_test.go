package reasoninggraph

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventsFromWriteWorkflowRunProjectsWorkflowTelemetry(t *testing.T) {
	now := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	run := types.WriteWorkflowRun{
		RunID:         "wf-graph",
		Status:        types.WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1",
		UpdatedAt:     now,
		Completion: &types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionVerified,
			ReasonCode: "tests_passed",
			At:         now,
		},
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchComplete,
			PlanID:        "plan-1",
			ApplyRef:      "refs/codrax/applied/plan-1",
			VerifyRef:     "plan-1.report.json",
			ActiveSliceID: "slice-1",
			Slices: []types.WriteWorkflowSlice{{
				ID:        "slice-1",
				Status:    types.ChangePlanSliceVerified,
				Paths:     []string{"src/app.py"},
				ApplyRef:  "refs/codrax/applied/plan-1",
				VerifyRef: "plan-1.report.json",
				Completion: &types.WriteWorkflowCompletion{
					Verdict:    types.WriteWorkflowCompletionVerified,
					ReasonCode: "tests_passed",
					At:         now,
				},
			}},
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionVerified,
				ReasonCode: "tests_passed",
				At:         now,
			},
		}},
	}

	events := EventsFromWriteWorkflowRun(run)
	if len(events) == 0 {
		t.Fatal("expected workflow reasoning events")
	}
	if got := events[0].Kind; got != ReasoningEventGraphSeeded {
		t.Fatalf("first event kind = %q, want graph_seeded", got)
	}
	view := ReduceEvents(events)
	if view.GraphID != "wf-graph" || view.EventCount != len(events) {
		t.Fatalf("view header = %+v, want graph wf-graph count %d", view, len(events))
	}
	if len(view.WorkflowEvents) == 0 {
		t.Fatalf("workflow events missing: %+v", view)
	}
	if !reasoningGraphViewHasKind(view, ReasoningEventAuthorityProjected) {
		t.Fatalf("authority event missing from counts: %+v", view.EventKindCounts)
	}
	if !reasoningGraphViewHasKind(view, ReasoningEventPatchEffectProjected) {
		t.Fatalf("patch effect event missing from counts: %+v", view.EventKindCounts)
	}
	var sawSliceNode bool
	for _, node := range view.Nodes {
		if node.NodeID == "workflow:slice-1" && node.Kind == ReasoningNodeVerify {
			sawSliceNode = true
			break
		}
	}
	if !sawSliceNode {
		t.Fatalf("expected verified slice node, nodes=%+v", view.Nodes)
	}
	for _, event := range events {
		if event.ID == "" || !strings.HasPrefix(event.ID, "wf-graph:") {
			t.Fatalf("event ID not preserved from loop event: %+v", event)
		}
	}
}

func reasoningGraphViewHasKind(view ReasoningGraphView, kind ReasoningEventKind) bool {
	for _, item := range view.EventKindCounts {
		if item.Kind == kind && item.Count > 0 {
			return true
		}
	}
	return false
}
