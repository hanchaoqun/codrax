package operation

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
)

func ReasoningEventsFromWorkflowInstance(workflow WorkflowInstance) []reasoninggraph.ReasoningEvent {
	graphID := firstNonEmpty(strings.TrimSpace(workflow.ID), "operation_workflow")
	reason := "operation_workflow_projected"
	if workflow.Cancelled {
		reason = "operation_workflow_cancelled"
	}
	events := []reasoninggraph.ReasoningEvent{reasoninggraph.NewObservationEvent(reasoninggraph.ObservationInput{
		GraphID:    graphID,
		NodeID:     "operation:" + graphID,
		NodeKind:   reasoninggraph.ReasoningNodeOperation,
		Sequence:   1,
		Kind:       reasoninggraph.ReasoningEventOperationWorkflowProjected,
		ReasonCode: reason,
		Payload: reasoninggraph.ObservationPayload{
			WorkflowID:  graphID,
			ActionCount: len(workflow.Actions),
			EdgeCount:   len(workflow.Edges),
			QueueCount:  len(workflow.Queue),
		},
	})}
	for i, action := range workflow.Actions {
		actionID := firstNonEmpty(strings.TrimSpace(action.ID), "action")
		status := string(action.Status)
		if status == "" {
			status = string(WorkflowActionQueued)
		}
		events = append(events, reasoninggraph.NewObservationEvent(reasoninggraph.ObservationInput{
			GraphID:    graphID,
			NodeID:     "operation:" + actionID,
			NodeKind:   reasoninggraph.ReasoningNodeOperation,
			Sequence:   int64(i + 2),
			Kind:       reasoninggraph.ReasoningEventOperationActionProjected,
			ReasonCode: status,
			Payload: reasoninggraph.ObservationPayload{
				WorkflowID:    graphID,
				ActionID:      actionID,
				ActionStatus:  status,
				OperationKind: operationActionKind(action),
				TargetSurface: operationActionSurface(action),
				RiskLevel:     operationActionRisk(action),
			},
		}))
	}
	return reasoninggraph.NormalizeEvents(events)
}

func operationActionKind(action WorkflowAction) string {
	return firstNonEmpty(action.Plan.Kind, action.Request.OperationKind, action.NextAction.OperationKind, action.NextAction.Operation)
}

func operationActionSurface(action WorkflowAction) string {
	return firstNonEmpty(action.Plan.TargetSurface, action.Request.TargetSurface, action.NextAction.TargetSurface)
}

func operationActionRisk(action WorkflowAction) string {
	return firstNonEmpty(action.Plan.RiskLevel, action.Request.RiskLevel, action.NextAction.RiskLevel)
}
