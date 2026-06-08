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
