package operation

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReasoningEventsFromWorkflowInstanceProjectsTypedWorkflowState(t *testing.T) {
	workflow := NewWorkflowInstance("op-wf-1", "create deck", 4, 8)
	root, ok, reason := workflow.AddAction("", WorkflowEdgeNext, WorkflowAction{
		ID: "action-1",
		Plan: Plan{
			Kind:          "presentation_generation",
			TargetSurface: "slides",
			RiskLevel:     "low",
		},
		Status: WorkflowActionExecuted,
	})
	if !ok {
		t.Fatalf("add root: %s", reason)
	}
	_, ok, reason = workflow.AddAction(root.ID, WorkflowEdgeNext, WorkflowAction{
		ID: "action-2",
		NextAction: WorkflowNextAction{
			OperationKind: "document_generation",
			TargetSurface: "docx",
			RiskLevel:     "medium",
		},
		Status: WorkflowActionQueued,
	})
	if !ok {
		t.Fatalf("add child: %s", reason)
	}

	events := ReasoningEventsFromWorkflowInstance(workflow)
	view := reasoninggraph.ReduceEvents(events)
	if view.GraphID != "op-wf-1" || len(view.AuxiliaryEvents) != 3 {
		t.Fatalf("operation view = %+v", view)
	}
	header := view.AuxiliaryEvents[0]
	if header.Kind != reasoninggraph.ReasoningEventOperationWorkflowProjected ||
		header.WorkflowID != "op-wf-1" ||
		header.ActionCount != 2 ||
		header.EdgeCount != 1 ||
		header.QueueCount != 2 {
		t.Fatalf("workflow header = %+v", header)
	}
	action := view.AuxiliaryEvents[1]
	if action.Kind != reasoninggraph.ReasoningEventOperationActionProjected ||
		action.ActionID != "action-1" ||
		action.ActionStatus != string(WorkflowActionExecuted) ||
		action.OperationKind != "presentation_generation" ||
		action.TargetSurface != "slides" ||
		action.RiskLevel != "low" {
		t.Fatalf("operation action = %+v", action)
	}
	audit := reasoninggraph.AuditSummaryFromEvents(events, "operation_test", 5)
	if audit == nil || operationLaneCount(audit.Lanes, "auxiliary") != 3 || len(audit.AuxiliaryEvents) != 3 {
		t.Fatalf("audit = %+v", audit)
	}
}

func operationLaneCount(lanes []types.ReasoningGraphAuditLane, name string) int {
	for _, lane := range lanes {
		if lane.Name == name {
			return lane.Count
		}
	}
	return 0
}
