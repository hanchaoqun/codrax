package writeflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const graphGuidanceRepairStormThreshold = 3

type WorkflowGraphGuidanceState string

const (
	WorkflowGraphGuidanceNone         WorkflowGraphGuidanceState = "none"
	WorkflowGraphGuidanceMissingInput WorkflowGraphGuidanceState = "missing_input"
	WorkflowGraphGuidanceRepairStorm  WorkflowGraphGuidanceState = "repair_storm"
	WorkflowGraphGuidanceLLMWait      WorkflowGraphGuidanceState = "llm_wait_observed"
)

// WorkflowGraphGuidanceView is the controller-facing interpretation of the
// typed reasoning graph audit. It is deliberately small: graph signals can only
// recommend existing workflow actions, and the caller still validates the
// resulting action against the ordinary state machine.
type WorkflowGraphGuidanceView struct {
	State             WorkflowGraphGuidanceState `json:"state,omitempty"`
	ReasonCode        string                     `json:"reason_code,omitempty"`
	Reason            string                     `json:"reason,omitempty"`
	RecommendedAction WorkflowAction             `json:"recommended_action,omitempty"`
	RepairEventCount  int                        `json:"repair_event_count,omitempty"`
	LLMEventCount     int                        `json:"llm_event_count,omitempty"`
	MissingCount      int                        `json:"missing_count,omitempty"`
}

func DeriveWorkflowGraphGuidance(view WorkflowExecutionView, audit *types.ReasoningGraphAuditSummary) WorkflowGraphGuidanceView {
	audit = types.NormalizeReasoningGraphAuditSummaryPtr(audit)
	if audit == nil {
		return WorkflowGraphGuidanceView{State: WorkflowGraphGuidanceNone}
	}
	guidance := WorkflowGraphGuidanceView{
		State:            WorkflowGraphGuidanceNone,
		RepairEventCount: reasoningGraphAuditLaneCount(audit, "repair"),
		LLMEventCount:    reasoningGraphAuditLaneCount(audit, "llm"),
		MissingCount:     len(audit.Missing),
	}
	if graphGuidanceCanRequestExploration(view) && guidance.MissingCount > 0 {
		guidance.State = WorkflowGraphGuidanceMissingInput
		guidance.ReasonCode = "graph_missing_evidence_requires_exploration"
		guidance.Reason = fmt.Sprintf("typed reasoning graph reports %d missing evidence gap(s)", guidance.MissingCount)
		guidance.RecommendedAction = ActionExploreCode
		return guidance
	}
	if graphGuidanceCanRequestExploration(view) && guidance.RepairEventCount >= graphGuidanceRepairStormThreshold {
		guidance.State = WorkflowGraphGuidanceRepairStorm
		guidance.ReasonCode = "graph_repair_storm_requires_exploration"
		guidance.Reason = fmt.Sprintf("typed reasoning graph reports %d repair event(s); run one bounded exploration before more planning", guidance.RepairEventCount)
		guidance.RecommendedAction = ActionExploreCode
		return guidance
	}
	if guidance.LLMEventCount > 0 {
		guidance.State = WorkflowGraphGuidanceLLMWait
		guidance.ReasonCode = "graph_llm_wait_observed"
		guidance.Reason = fmt.Sprintf("typed reasoning graph reports %d LLM wait/retry event(s)", guidance.LLMEventCount)
	}
	return guidance
}

func ValidateWorkflowTransitionWithGraph(view WorkflowExecutionView, decision WriteWorkflowDecision, audit *types.ReasoningGraphAuditSummary) WorkflowTransitionValidation {
	base := ValidateWorkflowTransition(view, decision)
	if !base.Allowed {
		return base
	}
	guidance := DeriveWorkflowGraphGuidance(view, audit)
	if guidance.RecommendedAction == "" || guidance.RecommendedAction == decision.Action {
		return base
	}
	switch guidance.RecommendedAction {
	case ActionExploreCode:
		switch decision.Action {
		case ActionPlanBatch, ActionAppendBatch, ActionSplitBatch, ActionReplanBatch, ActionFinish:
			return workflowTransitionDenied(guidance.ReasonCode, guidance.Reason, ActionExploreCode)
		}
	}
	return base
}

func graphGuidanceCanRequestExploration(view WorkflowExecutionView) bool {
	if view.ExploreAttempts > 0 {
		return false
	}
	switch view.State {
	case WorkflowExecutionReadyToPlan, WorkflowExecutionNeedsReplan:
		return true
	default:
		return false
	}
}

func reasoningGraphAuditLaneCount(audit *types.ReasoningGraphAuditSummary, name string) int {
	if audit == nil {
		return 0
	}
	name = strings.TrimSpace(name)
	for _, lane := range audit.Lanes {
		if strings.TrimSpace(lane.Name) == name {
			return lane.Count
		}
	}
	switch name {
	case "repair":
		return len(audit.RepairEvents)
	case "llm":
		return len(audit.LLMEvents)
	default:
		return 0
	}
}
