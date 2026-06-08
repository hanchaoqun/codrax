package dataworkflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ConservativeEvaluationInput struct {
	Records  []WorkflowRecord
	State    WorkflowStateView
	HadProse bool
}

// ConservativeEvaluationFromWorkflowState returns the deterministic fallback
// evaluation used when an evaluator fails to emit a structured tool call. It
// consumes only workflow IR: typed violations, the latest record shape, and
// state-derived action constraints. Model prose may change the human-readable
// reason, but it never drives the hard decision.
func ConservativeEvaluationFromWorkflowState(input ConservativeEvaluationInput) dataquery.Evaluation {
	reason := "evaluator produced no structured tool call; continuing conservatively from deterministic workflow state"
	if input.HadProse {
		reason = "evaluator produced no structured tool call after prose response; continuing conservatively"
	}
	base := dataquery.Evaluation{Status: dataquery.EvalContinueData, Confidence: "low", Reason: reason}
	if gated, ok := GateEvaluationWithWorkflowViolations(base, input.State.WorkflowViolations); ok {
		return gated
	}
	if len(input.Records) == 0 {
		return NormalizeEvaluationForWorkflowState(input.State, base)
	}
	last := input.Records[len(input.Records)-1]
	if strings.TrimSpace(last.Err) != "" {
		eval := dataquery.Evaluation{Status: dataquery.EvalRepairNode, Confidence: "low", Reason: reason}
		if len(last.Plan.Actions) > 0 {
			action := last.Plan.Actions[len(last.Plan.Actions)-1]
			eval.ActionID = action.ID
			eval.ActionKind = string(action.Kind)
		}
		return NormalizeEvaluationForWorkflowState(input.State, eval)
	}
	if last.Result != nil && len(last.Result.Artifacts) > 0 {
		return NormalizeEvaluationForWorkflowState(input.State, dataquery.Evaluation{Status: dataquery.EvalContinueTransform, Confidence: "low", Reason: reason})
	}
	return NormalizeEvaluationForWorkflowState(input.State, base)
}

func NormalizeEvaluationForWorkflowState(state WorkflowStateView, eval dataquery.Evaluation) dataquery.Evaluation {
	if gated, ok := GateEvaluationWithWorkflowViolations(eval, state.WorkflowViolations); ok {
		return gated
	}
	if eval.Status != dataquery.EvalContinueTransform {
		return eval
	}
	allowed := state.ComputedAllowedNextActions()
	if !state.CustomTransformDisabled || len(allowed) == 0 {
		return eval
	}
	eval.Status = dataquery.EvalExpandGraph
	note := "workflow custom_transform_disabled=true; continue with typed allowed_next_actions [" + strings.Join(allowed, ", ") + "] at next_stage=" + strings.TrimSpace(state.NextStage)
	eval.Reason = appendEvaluationReason(eval.Reason, note)
	return eval
}

type EvaluationDecisionAction string

const (
	EvaluationDecisionReturnAnswer     EvaluationDecisionAction = "return_answer"
	EvaluationDecisionReturnEvaluation EvaluationDecisionAction = "return_evaluation"
	EvaluationDecisionContinuePlan     EvaluationDecisionAction = "continue_plan"
	EvaluationDecisionRepairPlan       EvaluationDecisionAction = "repair_plan"
	EvaluationDecisionFallbackPlan     EvaluationDecisionAction = "fallback_plan"
	EvaluationDecisionFail             EvaluationDecisionAction = "fail"
)

type EvaluationFallbackCandidate struct {
	Source    string             `json:"source,omitempty"`
	Plan      dataquery.TaskPlan `json:"plan,omitempty"`
	Reason    string             `json:"reason,omitempty"`
	Available bool               `json:"available,omitempty"`
}

type EvaluationDecisionInput struct {
	Evaluation               dataquery.Evaluation        `json:"evaluation,omitempty"`
	CompletionSatisfied      bool                        `json:"completion_satisfied,omitempty"`
	CompletionGuard          GuardResult                 `json:"completion_guard,omitempty"`
	CompletionFallback       EvaluationFallbackCandidate `json:"completion_fallback,omitempty"`
	RepairFallback           EvaluationFallbackCandidate `json:"repair_fallback,omitempty"`
	WorkflowFallback         EvaluationFallbackCandidate `json:"workflow_fallback,omitempty"`
	ContinuationPlannerReady bool                        `json:"continuation_planner_ready,omitempty"`
	RepairPlannerReady       bool                        `json:"repair_planner_ready,omitempty"`
	RepairBudgetAvailable    bool                        `json:"repair_budget_available,omitempty"`
}

type EvaluationDecision struct {
	Action EvaluationDecisionAction `json:"action,omitempty"`
	Status string                   `json:"status,omitempty"`
	Source string                   `json:"source,omitempty"`
	Plan   dataquery.TaskPlan       `json:"plan,omitempty"`
	Guard  GuardResult              `json:"guard,omitempty"`
	Reason string                   `json:"reason,omitempty"`
}

func (d EvaluationDecision) HasPlan() bool {
	return taskPlanHasExecutableShape(d.Plan)
}

func DecideEvaluation(input EvaluationDecisionInput) EvaluationDecision {
	eval := input.Evaluation
	if input.CompletionSatisfied {
		return EvaluationDecision{
			Action: EvaluationDecisionReturnAnswer,
			Status: "complete",
			Source: "completion_gate",
			Reason: strings.TrimSpace(eval.Reason),
		}
	}
	switch eval.Status {
	case dataquery.EvalComplete:
		if input.CompletionGuard.Empty() {
			return EvaluationDecision{
				Action: EvaluationDecisionReturnAnswer,
				Status: "complete",
				Reason: strings.TrimSpace(eval.Reason),
			}
		}
		if input.CompletionFallback.Available && taskPlanHasExecutableShape(input.CompletionFallback.Plan) {
			return EvaluationDecision{
				Action: EvaluationDecisionFallbackPlan,
				Status: "continue",
				Source: firstNonEmpty(strings.TrimSpace(input.CompletionFallback.Source), "completion_gate"),
				Plan:   cloneTaskPlanValue(input.CompletionFallback.Plan),
				Guard:  input.CompletionGuard,
				Reason: firstNonEmpty(strings.TrimSpace(input.CompletionFallback.Reason), input.CompletionGuard.ErrorText()),
			}
		}
		if input.RepairPlannerReady && input.RepairBudgetAvailable {
			return EvaluationDecision{
				Action: EvaluationDecisionRepairPlan,
				Status: "repair",
				Source: "completion_gate",
				Guard:  input.CompletionGuard,
				Reason: input.CompletionGuard.ErrorText(),
			}
		}
		return EvaluationDecision{
			Action: EvaluationDecisionFail,
			Status: "failed",
			Source: "completion_gate",
			Guard:  input.CompletionGuard,
			Reason: input.CompletionGuard.ErrorText(),
		}
	case dataquery.EvalContinueData, dataquery.EvalExpandGraph, dataquery.EvalContinueTransform:
		if input.ContinuationPlannerReady {
			return EvaluationDecision{
				Action: EvaluationDecisionContinuePlan,
				Status: "continue",
				Reason: strings.TrimSpace(eval.Reason),
			}
		}
		return EvaluationDecision{
			Action: EvaluationDecisionReturnAnswer,
			Status: "partial_answer_possible",
			Reason: appendEvaluationReason("data task evaluator requested continuation but continuation planner is unavailable", eval.Reason),
		}
	case dataquery.EvalRepairNode:
		if input.RepairFallback.Available && taskPlanHasExecutableShape(input.RepairFallback.Plan) {
			return EvaluationDecision{
				Action: EvaluationDecisionFallbackPlan,
				Status: "continue",
				Source: firstNonEmpty(strings.TrimSpace(input.RepairFallback.Source), "repair_fallback"),
				Plan:   cloneTaskPlanValue(input.RepairFallback.Plan),
				Reason: strings.TrimSpace(input.RepairFallback.Reason),
			}
		}
		if input.RepairPlannerReady && input.RepairBudgetAvailable {
			return EvaluationDecision{
				Action: EvaluationDecisionRepairPlan,
				Status: "repair",
				Source: "evaluation_repair_node",
				Reason: EvaluationRepairReason(eval),
			}
		}
		return EvaluationDecision{
			Action: EvaluationDecisionReturnAnswer,
			Status: "partial_answer_possible",
			Reason: appendEvaluationReason("data task evaluator requested node repair but repair planner is unavailable or repair budget is exhausted", eval.Reason),
		}
	case dataquery.EvalNeedsClarification:
		if input.WorkflowFallback.Available && taskPlanHasExecutableShape(input.WorkflowFallback.Plan) {
			return EvaluationDecision{
				Action: EvaluationDecisionFallbackPlan,
				Status: "continue",
				Source: firstNonEmpty(strings.TrimSpace(input.WorkflowFallback.Source), "workflow_fallback"),
				Plan:   cloneTaskPlanValue(input.WorkflowFallback.Plan),
				Reason: firstNonEmpty(strings.TrimSpace(input.WorkflowFallback.Reason), strings.TrimSpace(eval.Reason)),
			}
		}
		return EvaluationDecision{
			Action: EvaluationDecisionReturnEvaluation,
			Status: "needs_clarification",
			Reason: strings.TrimSpace(eval.Reason),
		}
	case dataquery.EvalBlocked:
		if input.WorkflowFallback.Available && taskPlanHasExecutableShape(input.WorkflowFallback.Plan) {
			return EvaluationDecision{
				Action: EvaluationDecisionFallbackPlan,
				Status: "continue",
				Source: firstNonEmpty(strings.TrimSpace(input.WorkflowFallback.Source), "workflow_fallback"),
				Plan:   cloneTaskPlanValue(input.WorkflowFallback.Plan),
				Reason: firstNonEmpty(strings.TrimSpace(input.WorkflowFallback.Reason), strings.TrimSpace(eval.Reason)),
			}
		}
		return EvaluationDecision{
			Action: EvaluationDecisionReturnEvaluation,
			Status: "blocked",
			Reason: strings.TrimSpace(eval.Reason),
		}
	case dataquery.EvalBudgetExhausted, dataquery.EvalPartialAnswerPossible:
		return EvaluationDecision{
			Action: EvaluationDecisionReturnAnswer,
			Status: string(eval.Status),
			Reason: strings.TrimSpace(eval.Reason),
		}
	default:
		return EvaluationDecision{
			Action: EvaluationDecisionReturnAnswer,
			Status: "partial_answer_possible",
			Reason: "data task evaluator returned unrecognized status: " + string(eval.Status),
		}
	}
}

func EvaluationRepairReason(eval dataquery.Evaluation) string {
	parts := []string{"evaluation requested node repair"}
	if id := strings.TrimSpace(eval.ActionID); id != "" {
		parts = append(parts, "action_id="+id)
	}
	if kind := strings.TrimSpace(eval.ActionKind); kind != "" {
		parts = append(parts, "action_kind="+kind)
	}
	if locus := strings.TrimSpace(eval.RepairLocus); locus != "" {
		parts = append(parts, "repair_locus="+locus)
	}
	if reason := strings.TrimSpace(eval.Reason); reason != "" {
		parts = append(parts, "reason="+reason)
	}
	return strings.Join(parts, "; ")
}

// GateEvaluationWithWorkflowViolations makes reducer-owned blockers
// authoritative over model-produced evaluator statuses.
func GateEvaluationWithWorkflowViolations(eval dataquery.Evaluation, violations []WorkflowViolation) (dataquery.Evaluation, bool) {
	violation, ok := FirstHardWorkflowViolation(violations)
	if !ok {
		return eval, false
	}
	blockerEval := EvaluationFromWorkflowViolation(eval.Reason, string(eval.Status), violation)
	if blockerEval.Status != dataquery.EvalRepairNode || eval.Status != dataquery.EvalRepairNode {
		return blockerEval, true
	}
	out := eval
	if strings.TrimSpace(out.ActionID) == "" {
		out.ActionID = blockerEval.ActionID
	}
	if strings.TrimSpace(out.ActionKind) == "" {
		out.ActionKind = blockerEval.ActionKind
	}
	if strings.TrimSpace(out.RepairLocus) == "" {
		out.RepairLocus = blockerEval.RepairLocus
	}
	if strings.TrimSpace(out.Confidence) == "" {
		out.Confidence = blockerEval.Confidence
	}
	out.Reason = appendEvaluationReason(out.Reason, workflowViolationReason(violation))
	return out, true
}

func FirstHardWorkflowViolation(violations []WorkflowViolation) (WorkflowViolation, bool) {
	for _, violation := range violations {
		if strings.TrimSpace(violation.Code) == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(violation.Severity)) {
		case "warning", "warn":
			continue
		default:
			return violation, true
		}
	}
	return WorkflowViolation{}, false
}

func EvaluationFromWorkflowViolation(baseReason, originalStatus string, violation WorkflowViolation) dataquery.Evaluation {
	status := dataquery.EvalRepairNode
	if violation.Repairability == RepairNeedsClarification {
		status = dataquery.EvalNeedsClarification
	}
	reason := strings.TrimSpace(baseReason)
	if reason == "" {
		reason = "deterministic workflow state has a typed blocker"
	}
	if originalStatus = strings.TrimSpace(originalStatus); originalStatus != "" {
		reason = appendEvaluationReason(reason, "original_status="+originalStatus)
	}
	reason = appendEvaluationReason(reason, workflowViolationReason(violation))
	return dataquery.Evaluation{
		Status:      status,
		Confidence:  "low",
		Reason:      reason,
		ActionID:    strings.TrimSpace(violation.ActionID),
		ActionKind:  strings.TrimSpace(violation.ActionKind),
		RepairLocus: firstNonEmpty(strings.TrimSpace(violation.InputAlias), strings.TrimSpace(violation.Param), strings.TrimSpace(violation.Field), strings.TrimSpace(violation.OutputAlias)),
	}
}

func workflowViolationReason(violation WorkflowViolation) string {
	reason := ""
	if code := strings.TrimSpace(violation.Code); code != "" {
		reason = "typed_violation=" + code
	}
	if detail := strings.TrimSpace(violation.Reason); detail != "" {
		reason = appendEvaluationReason(reason, detail)
	}
	return reason
}

func appendEvaluationReason(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; " + next
}
