package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReasoningEventsFromWorkflowProjectionProjectsTypedDataState(t *testing.T) {
	current := dataquery.TaskPlan{
		Status: "continue_data",
		Actions: []dataquery.DataAction{{
			ID:             "assemble",
			Kind:           dataquery.DataActionAssembleAnswer,
			OutputArtifact: "answer",
		}},
	}
	rt := NewWorkflowRuntime(current)
	rt.AppendRecord(WorkflowRecord{
		Plan: dataquery.TaskPlan{
			Actions: []dataquery.DataAction{{
				ID:             "extract",
				Kind:           dataquery.DataActionExtractRecords,
				InputPaths:     []string{"input.csv"},
				OutputArtifact: "records",
			}},
		},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{{ID: "records", Kind: "table"}},
		},
		Violations: []dataquery.DataTaskViolation{{
			Code:          "missing_field",
			ActionID:      "extract",
			ActionKind:    string(dataquery.DataActionExtractRecords),
			InputAlias:    "records",
			Repairability: dataquery.RepairabilityNeedsRecompute,
		}},
		Evaluation: &dataquery.Evaluation{
			Status:      dataquery.EvalRepairNode,
			ActionID:    "extract",
			ActionKind:  string(dataquery.DataActionExtractRecords),
			RepairLocus: "records.status",
		},
	})
	state := WorkflowStateSnapshot{
		DecisionFallbackReasonCode: StageReconcileArtifacts,
		Decision: WorkflowDecision{
			Status:     "continue",
			ReasonCode: "missing_reconcile",
			NextActions: []string{
				string(dataquery.DataActionReconcile),
			},
		},
	}

	events := ReasoningEventsFromWorkflowProjection(ReasoningProjectionInput{
		GraphID: "data-run-1",
		Runtime: rt,
		State:   state,
	})
	view := reasoninggraph.ReduceEvents(events)
	if view.GraphID != "data-run-1" || len(view.AuxiliaryEvents) == 0 {
		t.Fatalf("view = %+v", view)
	}
	header := view.AuxiliaryEvents[0]
	if header.Kind != reasoninggraph.ReasoningEventDataWorkflowProjected ||
		header.WorkflowID != "data-run-1" ||
		header.DataStage != StageReconcileArtifacts ||
		header.RecordCount != 1 ||
		header.ResultCount != 1 ||
		header.ViolationCount != 1 {
		t.Fatalf("header = %+v", header)
	}
	if !dataReasoningViewHasKind(view, reasoninggraph.ReasoningEventDataActionProjected) ||
		!dataReasoningViewHasKind(view, reasoninggraph.ReasoningEventDataEvaluationProjected) ||
		!dataReasoningViewHasKind(view, reasoninggraph.ReasoningEventDataViolationProjected) {
		t.Fatalf("missing data projection events: %+v", view.AuxiliaryEvents)
	}
	audit := reasoninggraph.AuditSummaryFromEvents(events, "data_workflow_test", 8)
	if audit == nil || dataProjectionLaneCount(audit.Lanes, "auxiliary") == 0 || len(audit.AuxiliaryEvents) == 0 {
		t.Fatalf("audit = %+v", audit)
	}
	if audit.AuxiliaryEvents[0].DataStage != StageReconcileArtifacts ||
		audit.AuxiliaryEvents[0].RecordCount != 1 ||
		audit.AuxiliaryEvents[0].ViolationCount != 1 {
		t.Fatalf("audit header = %+v", audit.AuxiliaryEvents[0])
	}
}

func dataReasoningViewHasKind(view reasoninggraph.ReasoningGraphView, kind reasoninggraph.ReasoningEventKind) bool {
	for _, event := range view.AuxiliaryEvents {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func dataProjectionLaneCount(lanes []types.ReasoningGraphAuditLane, name string) int {
	for _, lane := range lanes {
		if lane.Name == name {
			return lane.Count
		}
	}
	return 0
}
