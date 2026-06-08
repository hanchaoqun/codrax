package dataworkflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

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
