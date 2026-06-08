package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestGateEvaluationWithWorkflowViolationsBlocksOptimisticCompletion(t *testing.T) {
	eval, gated := GateEvaluationWithWorkflowViolations(dataquery.Evaluation{
		Status:     dataquery.EvalComplete,
		Confidence: "high",
		Reason:     "model says complete",
	}, []WorkflowViolation{{
		Code:          "field_contract_violation",
		Severity:      "error",
		Repairability: RepairNeedsTypedAction,
		ActionID:      "filter_bad",
		ActionKind:    string(dataquery.DataActionFilterRecords),
		InputAlias:    "records.json",
		Field:         "status",
		Reason:        "status field is absent",
	}})
	if !gated {
		t.Fatal("gated=false, want typed hard blocker to gate evaluator status")
	}
	if eval.Status != dataquery.EvalRepairNode ||
		eval.Confidence != "low" ||
		eval.ActionID != "filter_bad" ||
		eval.ActionKind != string(dataquery.DataActionFilterRecords) ||
		eval.RepairLocus != "records.json" ||
		!strings.Contains(eval.Reason, "original_status=complete") ||
		!strings.Contains(eval.Reason, "typed_violation=field_contract_violation") {
		t.Fatalf("eval=%+v, want repair from typed workflow blocker", eval)
	}
}

func TestGateEvaluationWithWorkflowViolationsFillsUnderspecifiedRepairNode(t *testing.T) {
	eval, gated := GateEvaluationWithWorkflowViolations(dataquery.Evaluation{
		Status:     dataquery.EvalRepairNode,
		Confidence: "medium",
		Reason:     "repair this node",
	}, []WorkflowViolation{{
		Code:          "action_param_violation",
		Severity:      "error",
		Repairability: RepairNeedsTypedAction,
		ActionID:      "derive_bad",
		ActionKind:    string(dataquery.DataActionDeriveFields),
		Param:         "field_specs",
		Reason:        "field_specs is malformed",
	}})
	if !gated {
		t.Fatal("gated=false, want repair node to be enriched from reducer blocker")
	}
	if eval.Status != dataquery.EvalRepairNode ||
		eval.Confidence != "medium" ||
		eval.ActionID != "derive_bad" ||
		eval.ActionKind != string(dataquery.DataActionDeriveFields) ||
		eval.RepairLocus != "field_specs" ||
		!strings.Contains(eval.Reason, "typed_violation=action_param_violation") {
		t.Fatalf("eval=%+v, want repair locus filled from typed blocker", eval)
	}
}

func TestGateEvaluationWithWorkflowViolationsKeepsWarningSoft(t *testing.T) {
	eval, gated := GateEvaluationWithWorkflowViolations(dataquery.Evaluation{
		Status:     dataquery.EvalComplete,
		Confidence: "high",
		Reason:     "complete",
	}, []WorkflowViolation{{
		Code:          "zero_match_filter",
		Severity:      "warning",
		Repairability: RepairNeedsTypedAction,
		InputAlias:    "records.json",
	}})
	if gated {
		t.Fatalf("gated=true eval=%+v, warning-only diagnostics should remain soft", eval)
	}
	if eval.Status != dataquery.EvalComplete || eval.Confidence != "high" {
		t.Fatalf("eval=%+v, want original evaluation", eval)
	}
}

func TestGateEvaluationWithWorkflowViolationsUsesClarificationRepairability(t *testing.T) {
	eval, gated := GateEvaluationWithWorkflowViolations(dataquery.Evaluation{
		Status: dataquery.EvalContinueData,
		Reason: "continue",
	}, []WorkflowViolation{{
		Code:          "ambiguous_contract",
		Severity:      "error",
		Repairability: RepairNeedsClarification,
		Field:         "target",
	}})
	if !gated {
		t.Fatal("gated=false, want clarification blocker to gate evaluator status")
	}
	if eval.Status != dataquery.EvalNeedsClarification || eval.RepairLocus != "target" {
		t.Fatalf("eval=%+v, want needs_clarification from typed repairability", eval)
	}
}

func TestConservativeEvaluationFromWorkflowStateUsesTypedViolation(t *testing.T) {
	eval := ConservativeEvaluationFromWorkflowState(ConservativeEvaluationInput{
		Records: []WorkflowRecord{{
			Plan: dataquery.TaskPlan{
				Actions: []dataquery.DataAction{{
					ID:   "last_action",
					Kind: dataquery.DataActionCustomTransform,
				}},
			},
			Err: "opaque failure",
		}},
		State: WorkflowStateView{
			WorkflowViolations: []WorkflowViolation{{
				Code:          "action_param_violation",
				Severity:      "error",
				Repairability: RepairNeedsTypedAction,
				ActionID:      "derive_bad",
				ActionKind:    string(dataquery.DataActionDeriveFields),
				Param:         "operation",
				Reason:        "derive_fields operation is unsupported",
			}},
		},
	})
	if eval.Status != dataquery.EvalRepairNode ||
		eval.ActionID != "derive_bad" ||
		eval.ActionKind != string(dataquery.DataActionDeriveFields) ||
		eval.RepairLocus != "operation" ||
		!strings.Contains(eval.Reason, "typed_violation=action_param_violation") {
		t.Fatalf("eval=%+v, want repair from typed workflow violation", eval)
	}
}

func TestConservativeEvaluationFromWorkflowStateUsesLastError(t *testing.T) {
	eval := ConservativeEvaluationFromWorkflowState(ConservativeEvaluationInput{
		Records: []WorkflowRecord{{
			Plan: dataquery.TaskPlan{
				Actions: []dataquery.DataAction{{
					ID:   "compute_totals",
					Kind: dataquery.DataActionComputeContribs,
				}},
			},
			Err: "execute data task: action failed",
		}},
	})
	if eval.Status != dataquery.EvalRepairNode ||
		eval.ActionID != "compute_totals" ||
		eval.ActionKind != string(dataquery.DataActionComputeContribs) {
		t.Fatalf("eval=%+v, want repair from latest failed action", eval)
	}
}

func TestConservativeEvaluationFromWorkflowStateContinuesAfterArtifacts(t *testing.T) {
	eval := ConservativeEvaluationFromWorkflowState(ConservativeEvaluationInput{
		Records: []WorkflowRecord{{
			Result: &dataquery.Result{
				Artifacts: []dataquery.DataArtifact{{
					ID:   "records",
					Kind: string(dataquery.DataActionExtractRecords),
				}},
			},
		}},
		HadProse: true,
	})
	if eval.Status != dataquery.EvalContinueTransform ||
		eval.Confidence != "low" ||
		!strings.Contains(eval.Reason, "after prose response") {
		t.Fatalf("eval=%+v, want conservative transform continuation after artifacts", eval)
	}
}

func TestNormalizeEvaluationForWorkflowStateExpandsWhenCustomTransformDisabled(t *testing.T) {
	eval := NormalizeEvaluationForWorkflowState(WorkflowStateView{
		CustomTransformDisabled: true,
		NextStage:               "contribution",
		AllowedNextActions:      []string{string(dataquery.DataActionComputeContribs), string(dataquery.DataActionReconcile)},
	}, dataquery.Evaluation{
		Status:     dataquery.EvalContinueTransform,
		Confidence: "medium",
		Reason:     "continue with bounded transform",
	})
	if eval.Status != dataquery.EvalExpandGraph {
		t.Fatalf("Status=%q, want expand_graph", eval.Status)
	}
	if !strings.Contains(eval.Reason, "custom_transform_disabled=true") ||
		!strings.Contains(eval.Reason, "compute_contributions") ||
		!strings.Contains(eval.Reason, "reconcile_artifacts") {
		t.Fatalf("Reason=%q, want typed graph note", eval.Reason)
	}
}
