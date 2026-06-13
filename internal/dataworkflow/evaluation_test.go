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

func TestNormalizeEvaluationForWorkflowStateIgnoresStaleViolationsWhenComplete(t *testing.T) {
	state := completedWorkflowStateForEvaluationTest([]WorkflowViolation{
		actionDependencyViolationForEvaluationTest(),
	})
	eval := NormalizeEvaluationForWorkflowState(state, dataquery.Evaluation{
		Status:     dataquery.EvalComplete,
		Confidence: "high",
		Reason:     "current ledger and projection graph are complete",
	})
	if eval.Status != dataquery.EvalComplete || eval.ActionID != "" || eval.ActionKind != "" {
		t.Fatalf("eval=%+v, want completed state to retire stale violation lineage", eval)
	}
	if strings.Contains(eval.Reason, "typed_violation=action_dependency_violation") {
		t.Fatalf("Reason=%q, stale workflow violation should stay out of terminal evaluation reason", eval.Reason)
	}
}

func TestNormalizeEvaluationForWorkflowStatePreservesActionableRepairWhenComplete(t *testing.T) {
	state := completedWorkflowStateForEvaluationTest(nil)
	eval := NormalizeEvaluationForWorkflowState(state, dataquery.Evaluation{
		Status:      dataquery.EvalRepairNode,
		Confidence:  "high",
		Reason:      "final projection is structurally wrong",
		ActionID:    "assemble_final",
		ActionKind:  string(dataquery.DataActionAssembleAnswer),
		RepairLocus: "result.answer",
	})
	if eval.Status != dataquery.EvalRepairNode ||
		eval.ActionID != "assemble_final" ||
		eval.ActionKind != string(dataquery.DataActionAssembleAnswer) ||
		eval.RepairLocus != "result.answer" {
		t.Fatalf("eval=%+v, want completed state to preserve actionable typed repair", eval)
	}
}

func TestNormalizeEvaluationForWorkflowStateStillGatesIncompleteState(t *testing.T) {
	state := completedWorkflowStateForEvaluationTest([]WorkflowViolation{
		actionDependencyViolationForEvaluationTest(),
	})
	state.NextStage = StageReconcileArtifacts
	state.LedgerGraph.NextStage = StageReconcileArtifacts
	state.OutputProjectionGraph.Status = OutputProjectionStatusMissingProjection
	eval := NormalizeEvaluationForWorkflowState(state, dataquery.Evaluation{
		Status:     dataquery.EvalComplete,
		Confidence: "high",
		Reason:     "model says complete",
	})
	if eval.Status != dataquery.EvalRepairNode ||
		eval.ActionID != "old_compute" ||
		eval.ActionKind != string(dataquery.DataActionComputeContribs) ||
		!strings.Contains(eval.Reason, "typed_violation=action_dependency_violation") {
		t.Fatalf("eval=%+v, want incomplete state gated by typed workflow violation", eval)
	}
}

func TestConservativeEvaluationFromWorkflowStateIgnoresStaleViolationsWhenComplete(t *testing.T) {
	eval := ConservativeEvaluationFromWorkflowState(ConservativeEvaluationInput{
		Records: []WorkflowRecord{{
			Plan: dataquery.TaskPlan{
				Actions: []dataquery.DataAction{{
					ID:   "old_compute",
					Kind: dataquery.DataActionComputeContribs,
				}},
			},
			Err: "older action failed before later recovery",
		}},
		State: completedWorkflowStateForEvaluationTest([]WorkflowViolation{
			actionDependencyViolationForEvaluationTest(),
		}),
	})
	if eval.Status != dataquery.EvalComplete ||
		eval.ActionID != "" ||
		eval.ActionKind != "" ||
		!strings.Contains(eval.Reason, "current typed workflow state is complete") ||
		!strings.Contains(eval.Reason, "original_status=repair_node") {
		t.Fatalf("eval=%+v, want conservative fallback normalized by completed typed state", eval)
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

func completedWorkflowStateForEvaluationTest(violations []WorkflowViolation) WorkflowStateView {
	return WorkflowStateView{
		MaterialCoverageSufficient: true,
		NextStage:                  StageComplete,
		HasAnswer:                  true,
		LedgerGraph: LedgerGraph{
			NextStage: StageComplete,
			Dependencies: []LedgerDependency{{
				Ledger:   string(LedgerFinalProjection),
				Required: true,
				Present:  true,
				Status:   LedgerStatusSatisfied,
				Stage:    StageEmitOutputContractAnswer,
			}},
		},
		OutputProjectionGraph: OutputProjectionGraph{
			Status:                    OutputProjectionStatusSatisfied,
			AnswerPresent:             true,
			ProjectionArtifactPresent: true,
			ReferenceComplete:         true,
		},
		WorkflowViolations: violations,
	}
}

func actionDependencyViolationForEvaluationTest() WorkflowViolation {
	return WorkflowViolation{
		Code:          "action_dependency_violation",
		Severity:      "error",
		Repairability: RepairNeedsTypedAction,
		ActionID:      "old_compute",
		ActionKind:    string(dataquery.DataActionComputeContribs),
		InputAlias:    "stale_input",
		Reason:        "older action was outside the current allowed stage",
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

func TestDecideEvaluationCompleteUsesCompletionFallback(t *testing.T) {
	guard := NewGuardResult("missing_contributions", "error", RepairNeedsTypedAction, "contribution ledger missing", WorkflowViolation{Code: "missing_contributions"})
	fallback := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:   "compute",
			Kind: dataquery.DataActionComputeContribs,
		}},
	}
	decision := DecideEvaluation(EvaluationDecisionInput{
		Evaluation:      dataquery.Evaluation{Status: dataquery.EvalComplete, Reason: "model says complete"},
		CompletionGuard: guard,
		CompletionFallback: EvaluationFallbackCandidate{
			Source:    "ledger",
			Plan:      fallback,
			Reason:    "repair missing ledger",
			Available: true,
		},
		RepairPlannerReady:    true,
		RepairBudgetAvailable: true,
	})
	if decision.Action != EvaluationDecisionFallbackPlan || decision.Source != "ledger" || !decision.HasPlan() {
		t.Fatalf("decision=%#v", decision)
	}
	if decision.Reason != "repair missing ledger" {
		t.Fatalf("Reason=%q", decision.Reason)
	}
}

func TestDecideEvaluationCompletionSatisfiedOverridesNoisyRepair(t *testing.T) {
	decision := DecideEvaluation(EvaluationDecisionInput{
		Evaluation: dataquery.Evaluation{
			Status: dataquery.EvalRepairNode,
			Reason: "model still sees an older failed node",
		},
		CompletionSatisfied:   true,
		RepairPlannerReady:    true,
		RepairBudgetAvailable: true,
	})
	if decision.Action != EvaluationDecisionReturnAnswer || decision.Status != "complete" || decision.Source != "completion_gate" {
		t.Fatalf("decision=%#v, want deterministic completion to return answer", decision)
	}
}

func TestDecideEvaluationCompletionSatisfiedPreservesActionableRepair(t *testing.T) {
	decision := DecideEvaluation(EvaluationDecisionInput{
		Evaluation: dataquery.Evaluation{
			Status:      dataquery.EvalRepairNode,
			Confidence:  "high",
			Reason:      "projection shape mismatch",
			ActionID:    "assemble_final",
			ActionKind:  string(dataquery.DataActionAssembleAnswer),
			RepairLocus: "result.answer",
		},
		CompletionSatisfied:   true,
		RepairPlannerReady:    true,
		RepairBudgetAvailable: true,
	})
	if decision.Action != EvaluationDecisionRepairPlan ||
		decision.Status != "repair" ||
		decision.Source != "evaluation_repair_node" {
		t.Fatalf("decision=%#v, want actionable typed repair to survive completion gate", decision)
	}
	for _, want := range []string{"action_id=assemble_final", "action_kind=assemble_answer", "repair_locus=result.answer"} {
		if !strings.Contains(decision.Reason, want) {
			t.Fatalf("Reason=%q, want %q", decision.Reason, want)
		}
	}
}

func TestDecideEvaluationRepairUsesCompletionFallbackBeforePlanner(t *testing.T) {
	guard := NewGuardResult("missing_reconcile", "error", RepairNeedsTypedAction, "repair missing reconcile", WorkflowViolation{Code: "missing_required_ledger"})
	decision := DecideEvaluation(EvaluationDecisionInput{
		Evaluation: dataquery.Evaluation{
			Status: dataquery.EvalRepairNode,
			Reason: "older node still asks for repair",
		},
		CompletionGuard: guard,
		CompletionFallback: EvaluationFallbackCandidate{
			Source: "ledger",
			Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
				ID:   "continue_reconcile",
				Kind: dataquery.DataActionReconcile,
			}}},
			Reason:    guard.ErrorText(),
			Available: true,
		},
		RepairPlannerReady:    true,
		RepairBudgetAvailable: true,
	})
	if decision.Action != EvaluationDecisionFallbackPlan || decision.Source != "ledger" || !decision.HasPlan() {
		t.Fatalf("decision=%#v, want completion fallback before repair planner", decision)
	}
	if decision.Plan.Actions[0].Kind != dataquery.DataActionReconcile {
		t.Fatalf("Plan=%+v, want reconcile fallback", decision.Plan)
	}
}

func TestDecideEvaluationCompleteRepairsWhenNoFallback(t *testing.T) {
	guard := NewGuardResult("missing_answer", "error", RepairNeedsTypedAction, "final answer missing", WorkflowViolation{Code: "missing_answer"})
	decision := DecideEvaluation(EvaluationDecisionInput{
		Evaluation:            dataquery.Evaluation{Status: dataquery.EvalComplete},
		CompletionGuard:       guard,
		RepairPlannerReady:    true,
		RepairBudgetAvailable: true,
	})
	if decision.Action != EvaluationDecisionRepairPlan || decision.Source != "completion_gate" || decision.Reason != "final answer missing" {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestDecideEvaluationContinueNeedsPlanner(t *testing.T) {
	decision := DecideEvaluation(EvaluationDecisionInput{
		Evaluation:               dataquery.Evaluation{Status: dataquery.EvalExpandGraph, Reason: "more graph work"},
		ContinuationPlannerReady: true,
	})
	if decision.Action != EvaluationDecisionContinuePlan || decision.Status != "continue" {
		t.Fatalf("decision=%#v", decision)
	}
	decision = DecideEvaluation(EvaluationDecisionInput{
		Evaluation: dataquery.Evaluation{Status: dataquery.EvalContinueData, Reason: "need more"},
	})
	if decision.Action != EvaluationDecisionReturnAnswer || decision.Status != "partial_answer_possible" {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestDecideEvaluationTerminalStatusUsesWorkflowFallback(t *testing.T) {
	fallback := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:   "cover",
			Kind: dataquery.DataActionExtractRecords,
		}},
	}
	decision := DecideEvaluation(EvaluationDecisionInput{
		Evaluation: dataquery.Evaluation{Status: dataquery.EvalNeedsClarification, Reason: "missing local material"},
		WorkflowFallback: EvaluationFallbackCandidate{
			Source:    "coverage",
			Plan:      fallback,
			Reason:    "cover missing local material",
			Available: true,
		},
	})
	if decision.Action != EvaluationDecisionFallbackPlan || decision.Status != "continue" || decision.Source != "coverage" || !decision.HasPlan() {
		t.Fatalf("decision=%#v, want workflow fallback", decision)
	}
	if decision.Reason != "cover missing local material" {
		t.Fatalf("Reason=%q", decision.Reason)
	}
}

func TestDecideEvaluationRepairUsesFallbackBeforePlanner(t *testing.T) {
	fallback := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:   "enrich",
			Kind: dataquery.DataActionEnrichRecords,
		}},
	}
	decision := DecideEvaluation(EvaluationDecisionInput{
		Evaluation: dataquery.Evaluation{Status: dataquery.EvalRepairNode, Reason: "field missing"},
		RepairFallback: EvaluationFallbackCandidate{
			Source:    "field_contract",
			Plan:      fallback,
			Reason:    "materialize missing field",
			Available: true,
		},
		RepairPlannerReady:    true,
		RepairBudgetAvailable: true,
	})
	if decision.Action != EvaluationDecisionFallbackPlan || decision.Source != "field_contract" || !decision.HasPlan() {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestEvaluationRepairReasonIncludesTypedLocator(t *testing.T) {
	reason := EvaluationRepairReason(dataquery.Evaluation{
		Status:      dataquery.EvalRepairNode,
		ActionID:    "a1",
		ActionKind:  string(dataquery.DataActionFilterRecords),
		RepairLocus: "status",
		Reason:      "zero match",
	})
	for _, want := range []string{"action_id=a1", "action_kind=filter_records", "repair_locus=status", "reason=zero match"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("reason %q missing %q", reason, want)
		}
	}
}
