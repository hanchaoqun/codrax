package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
)

type ReasoningProjectionInput struct {
	GraphID  string
	Runtime  *WorkflowRuntime
	Snapshot WorkflowRuntimeSnapshot
	State    WorkflowStateSnapshot
	Journal  WorkflowJournal
}

func ReasoningEventsFromWorkflowRuntime(runtime *WorkflowRuntime, state WorkflowStateSnapshot) []reasoninggraph.ReasoningEvent {
	return ReasoningEventsFromWorkflowProjection(ReasoningProjectionInput{
		Runtime: runtime,
		State:   state,
	})
}

func ReasoningEventsFromWorkflowProjection(input ReasoningProjectionInput) []reasoninggraph.ReasoningEvent {
	snapshot := input.Snapshot
	if input.Runtime != nil {
		snapshot = input.Runtime.Snapshot()
	}
	graphID := firstNonEmpty(strings.TrimSpace(input.GraphID), "data_workflow")
	state := input.State
	actionGraph := reasoningProjectionActionGraph(snapshot, state)
	violations := reasoningProjectionViolations(snapshot, state, input.Journal)
	dataStage := reasoningProjectionStage(snapshot, state, input.Journal)
	events := make([]reasoninggraph.ReasoningEvent, 0, 1+
		reasoningActionGraphNodeCount(actionGraph)+
		reasoningEvaluationCount(snapshot, state)+
		len(violations)+
		len(snapshot.ProcessEvents)+
		len(input.Journal.ProcessEvents))
	sequence := int64(1)
	events = append(events, reasoninggraph.NewObservationEvent(reasoninggraph.ObservationInput{
		GraphID:    graphID,
		NodeID:     "data:" + graphID,
		NodeKind:   reasoninggraph.ReasoningNodeData,
		Sequence:   sequence,
		Kind:       reasoninggraph.ReasoningEventDataWorkflowProjected,
		ReasonCode: firstNonEmpty(strings.TrimSpace(state.Decision.ReasonCode), "data_workflow_projected"),
		Payload: reasoninggraph.ObservationPayload{
			WorkflowID:     graphID,
			DataStage:      dataStage,
			ActionCount:    reasoningActionGraphNodeCount(actionGraph),
			QueueCount:     reasoningQueueCount(snapshot, actionGraph),
			RecordCount:    len(snapshot.Records),
			ResultCount:    reasoningResultCount(snapshot.Records),
			ViolationCount: len(violations),
		},
	}))
	add := func(kind reasoninggraph.ReasoningEventKind, nodeID, reasonCode string, payload reasoninggraph.ObservationPayload) {
		sequence++
		payload.WorkflowID = graphID
		if payload.DataStage == "" {
			payload.DataStage = dataStage
		}
		events = append(events, reasoninggraph.NewObservationEvent(reasoninggraph.ObservationInput{
			GraphID:    graphID,
			NodeID:     nodeID,
			NodeKind:   reasoninggraph.ReasoningNodeData,
			Sequence:   sequence,
			Kind:       kind,
			ReasonCode: firstNonEmpty(strings.TrimSpace(reasonCode), string(kind)),
			Payload:    payload,
		}))
	}
	for _, node := range reasoningProjectionActionNodes(actionGraph) {
		actionID := firstNonEmpty(strings.TrimSpace(node.ID), fmt.Sprintf("action_%d", sequence+1))
		add(reasoninggraph.ReasoningEventDataActionProjected, "data:action:"+actionID, firstNonEmpty(node.Status, "data_action_projected"), reasoninggraph.ObservationPayload{
			ActionID:       actionID,
			ActionStatus:   node.Status,
			DataActionKind: node.Kind,
			InputRefCount:  len(node.InputAliases),
			OutputRefCount: boolCount(strings.TrimSpace(node.OutputAlias) != ""),
		})
	}
	for round, record := range snapshot.Records {
		if record.Evaluation == nil {
			continue
		}
		eval := *record.Evaluation
		add(reasoninggraph.ReasoningEventDataEvaluationProjected, fmt.Sprintf("data:evaluation:%d", round+1), string(eval.Status), reasoninggraph.ObservationPayload{
			DataEvalStatus: string(eval.Status),
			ActionID:       strings.TrimSpace(eval.ActionID),
			DataActionKind: strings.TrimSpace(eval.ActionKind),
			RepairLocus:    strings.TrimSpace(eval.RepairLocus),
			InputRefCount:  len(eval.MissingInputs),
		})
	}
	if !workflowDecisionEmpty(state.Decision) {
		add(reasoninggraph.ReasoningEventDataEvaluationProjected, "data:decision", firstNonEmpty(state.Decision.ReasonCode, state.Decision.Status), reasoninggraph.ObservationPayload{
			ActionStatus:   strings.TrimSpace(state.Decision.Status),
			DataEvalStatus: strings.TrimSpace(state.Decision.Status),
			ActionCount:    len(state.Decision.NextActions),
		})
	}
	for i, violation := range violations {
		repairLocus := firstNonEmpty(
			strings.TrimSpace(violation.InputAlias),
			strings.TrimSpace(violation.Param),
			strings.TrimSpace(violation.Field),
			strings.TrimSpace(violation.OutputAlias),
		)
		add(reasoninggraph.ReasoningEventDataViolationProjected, fmt.Sprintf("data:violation:%d", i+1), violation.Code, reasoninggraph.ObservationPayload{
			ActionID:          strings.TrimSpace(violation.ActionID),
			DataActionKind:    strings.TrimSpace(violation.ActionKind),
			DataViolationCode: strings.TrimSpace(violation.Code),
			ViolationKind:     strings.TrimSpace(violation.Code),
			RepairCode:        string(violation.Repairability),
			RepairLocus:       repairLocus,
		})
	}
	processEvents := appendWorkflowJournalEvents(snapshot.ProcessEvents, input.Journal.ProcessEvents...)
	for i, event := range processEvents {
		nodeID := fmt.Sprintf("data:process:%d", i+1)
		add(reasoninggraph.ReasoningEventDataWorkflowProjected, nodeID, firstNonEmpty(event.Kind, event.Status), reasoninggraph.ObservationPayload{
			ActionStatus:   strings.TrimSpace(event.Status),
			DataStage:      strings.TrimSpace(event.Kind),
			ActionCount:    boolCount(strings.TrimSpace(event.ActionSummary) != ""),
			ViolationCount: event.ViolationSummary.Total,
		})
	}
	return reasoninggraph.NormalizeEvents(events)
}

func reasoningProjectionStage(snapshot WorkflowRuntimeSnapshot, state WorkflowStateSnapshot, journal WorkflowJournal) string {
	computedStage := ""
	if !stageFactsIsZero(state.StageFacts) {
		computedStage = strings.TrimSpace(state.NextStage())
	}
	return firstNonEmpty(
		computedStage,
		strings.TrimSpace(state.LedgerGraph.NextStage),
		strings.TrimSpace(state.DecisionFallbackReasonCode),
		strings.TrimSpace(journal.Decision.ReasonCode),
		strings.TrimSpace(journal.Status),
		strings.TrimSpace(snapshot.CurrentPlan.Status),
	)
}

func reasoningProjectionActionGraph(snapshot WorkflowRuntimeSnapshot, state WorkflowStateSnapshot) ActionGraph {
	if !reasoningActionGraphIsZero(state.ActionGraph) {
		return state.ActionGraph
	}
	violations := appendWorkflowViolationsUnique(
		WorkflowViolationsFromRecordExecution(snapshot.Records),
		ActiveGuardViolationsFromRecords(snapshot.Records)...,
	)
	return ReduceActionGraphState(ActionGraphInput{
		Events:        BuildWorkflowActionEvents(snapshot.Records),
		Ready:         append([]dataquery.DataAction(nil), snapshot.CurrentPlan.Actions...),
		DeferredPlan:  snapshot.DeferredPlan,
		DeferredQueue: snapshot.DeferredQueue,
		Blocked:       violations,
		EventLimit:    20,
	})
}

func reasoningProjectionViolations(snapshot WorkflowRuntimeSnapshot, state WorkflowStateSnapshot, journal WorkflowJournal) []WorkflowViolation {
	out := appendWorkflowViolationsUnique(nil, state.WorkflowViolations...)
	out = appendWorkflowViolationsUnique(out, WorkflowViolationsFromRecordExecution(snapshot.Records)...)
	out = appendWorkflowViolationsUnique(out, ActiveGuardViolationsFromRecords(snapshot.Records)...)
	out = appendWorkflowViolationsUnique(out, journal.WorkflowViolations...)
	return out
}

func reasoningProjectionActionNodes(graph ActionGraph) []ActionNode {
	out := make([]ActionNode, 0, reasoningActionGraphNodeCount(graph))
	out = append(out, graph.Executed...)
	out = append(out, graph.Ready...)
	out = append(out, graph.Deferred...)
	out = append(out, graph.Blocked...)
	return out
}

func reasoningActionGraphNodeCount(graph ActionGraph) int {
	return len(graph.Executed) + len(graph.Ready) + len(graph.Deferred) + len(graph.Blocked)
}

func reasoningActionGraphIsZero(graph ActionGraph) bool {
	return reasoningActionGraphNodeCount(graph) == 0 &&
		graph.DeferredQueue.Actions == 0 &&
		graph.DeferredPlan == nil &&
		len(graph.DeferredEvents) == 0
}

func reasoningQueueCount(snapshot WorkflowRuntimeSnapshot, graph ActionGraph) int {
	if graph.DeferredQueue.Actions > 0 {
		return graph.DeferredQueue.Actions
	}
	if len(snapshot.DeferredQueue.Plan.Actions) > 0 {
		return len(snapshot.DeferredQueue.Plan.Actions)
	}
	if len(snapshot.DeferredPlan.Actions) > 0 {
		return len(snapshot.DeferredPlan.Actions)
	}
	return len(graph.Deferred)
}

func reasoningResultCount(records []WorkflowRecord) int {
	count := 0
	for _, record := range records {
		if record.Result != nil {
			count++
		}
	}
	return count
}

func reasoningEvaluationCount(snapshot WorkflowRuntimeSnapshot, state WorkflowStateSnapshot) int {
	count := 0
	for _, record := range snapshot.Records {
		if record.Evaluation != nil {
			count++
		}
	}
	if !workflowDecisionEmpty(state.Decision) {
		count++
	}
	return count
}
