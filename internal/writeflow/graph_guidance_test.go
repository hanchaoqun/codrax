package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestValidateWorkflowTransitionWithGraphRepairStormRequiresExplore(t *testing.T) {
	view := WorkflowExecutionView{
		Mode:  types.ModeApply,
		State: WorkflowExecutionReadyToPlan,
	}
	audit := &types.ReasoningGraphAuditSummary{
		Status: types.ReasoningGraphAuditObserved,
		Lanes:  []types.ReasoningGraphAuditLane{{Name: "repair", Count: 3}},
	}

	got := ValidateWorkflowTransitionWithGraph(view, WriteWorkflowDecision{
		Action: ActionPlanBatch,
		Batch:  &WriteBatchPlan{ID: "batch-1", Goal: "repair failed surface"},
	}, audit)
	if got.Allowed || got.ReasonCode != "graph_repair_storm_requires_exploration" || got.RecommendedAction != ActionExploreCode {
		t.Fatalf("repair storm should require bounded exploration before planning: %+v", got)
	}
}

func TestValidateWorkflowTransitionWithGraphRepairStormDoesNotLoopAfterExplore(t *testing.T) {
	view := WorkflowExecutionView{
		Mode:            types.ModeApply,
		State:           WorkflowExecutionReadyToPlan,
		ExploreAttempts: 1,
	}
	audit := &types.ReasoningGraphAuditSummary{
		Status: types.ReasoningGraphAuditObserved,
		Lanes:  []types.ReasoningGraphAuditLane{{Name: "repair", Count: 5}},
	}

	got := ValidateWorkflowTransitionWithGraph(view, WriteWorkflowDecision{
		Action: ActionPlanBatch,
		Batch:  &WriteBatchPlan{ID: "batch-1", Goal: "repair failed surface"},
	}, audit)
	if !got.Allowed {
		t.Fatalf("graph guidance must not create an exploration loop after one bounded attempt: %+v", got)
	}
}

func TestValidateWorkflowTransitionWithGraphMissingEvidenceRequiresExplore(t *testing.T) {
	view := WorkflowExecutionView{
		Mode:  types.ModeApply,
		State: WorkflowExecutionNeedsReplan,
	}
	audit := &types.ReasoningGraphAuditSummary{
		Status: types.ReasoningGraphAuditPartial,
		Missing: []types.ReasoningGraphAuditGap{{
			Code:     "owner_evidence_missing",
			Severity: "warning",
			Detail:   "owner localization evidence is missing",
		}},
	}

	got := ValidateWorkflowTransitionWithGraph(view, WriteWorkflowDecision{
		Action: ActionReplanBatch,
		Batch:  &WriteBatchPlan{ID: "batch-1", Goal: "repair failed surface"},
	}, audit)
	if got.Allowed || got.ReasonCode != "graph_missing_evidence_requires_exploration" || got.RecommendedAction != ActionExploreCode {
		t.Fatalf("missing graph evidence should request existing explore action: %+v", got)
	}
}

func TestDeriveWorkflowGraphGuidanceLLMWaitIsAdvisory(t *testing.T) {
	view := WorkflowExecutionView{
		Mode:  types.ModeApply,
		State: WorkflowExecutionReadyToPlan,
	}
	audit := &types.ReasoningGraphAuditSummary{
		Status: types.ReasoningGraphAuditObserved,
		Lanes:  []types.ReasoningGraphAuditLane{{Name: "llm", Count: 2}},
	}

	guidance := DeriveWorkflowGraphGuidance(view, audit)
	if guidance.State != WorkflowGraphGuidanceLLMWait || guidance.RecommendedAction != "" {
		t.Fatalf("LLM wait telemetry should remain advisory, got %+v", guidance)
	}
}
