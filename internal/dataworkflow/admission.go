package dataworkflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ActionDAGAdmissionInput struct {
	Plan                   dataquery.TaskPlan
	Protect                func(dataquery.TaskPlan) dataquery.TaskPlan
	PrefixFallback         func(dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool)
	Guard                  func(dataquery.TaskPlan) GuardResult
	DeterministicFallback  func(dataquery.TaskPlan, GuardResult) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool)
	BlockedIdempotencyKeys []string
	MaxRewrites            int
}

type ActionDAGAdmissionDecision struct {
	Plan          dataquery.TaskPlan `json:"plan"`
	Original      dataquery.TaskPlan `json:"original,omitempty"`
	Remainder     dataquery.TaskPlan `json:"remainder,omitempty"`
	Guard         GuardResult        `json:"guard,omitempty"`
	FinalGuard    GuardResult        `json:"final_guard,omitempty"`
	GuardErr      string             `json:"guard_err,omitempty"`
	FinalGuardErr string             `json:"final_guard_err,omitempty"`
	Reason        string             `json:"reason,omitempty"`
	Rewritten     bool               `json:"rewritten,omitempty"`
}

const defaultAdmissionMaxRewrites = 3

func AdmitActionDAGPlan(input ActionDAGAdmissionInput) ActionDAGAdmissionDecision {
	protect := input.Protect
	if protect == nil {
		protect = func(plan dataquery.TaskPlan) dataquery.TaskPlan { return plan }
	}
	guard := input.Guard
	if guard == nil {
		guard = func(dataquery.TaskPlan) GuardResult { return GuardResult{} }
	}
	maxRewrites := input.MaxRewrites
	if maxRewrites <= 0 {
		maxRewrites = defaultAdmissionMaxRewrites
	}
	protected := protect(input.Plan)
	out := ActionDAGAdmissionDecision{Plan: protected, Original: protected}
	current := protected
	for i := 0; i < maxRewrites; i++ {
		if input.PrefixFallback != nil {
			if fallback, remainder, reason, ok := input.PrefixFallback(current); ok {
				out.Remainder = MergeAdmissionRemainder(remainder, out.Remainder)
				out.Reason = AppendAdmissionReason(out.Reason, reason)
				out.Rewritten = true
				current = protect(fallback)
				out.Plan = current
				continue
			}
		}
		if guardResult := duplicateActionEdgeGuard(current, input.BlockedIdempotencyKeys); !guardResult.Empty() {
			return admissionBlocked(out, current, guardResult)
		}
		guardResult := guard(current)
		if guardResult.Empty() {
			out.Plan = current
			return out
		}
		if input.DeterministicFallback == nil {
			return admissionBlocked(out, current, guardResult)
		}
		fallback, remainder, reason, ok := input.DeterministicFallback(current, guardResult)
		if !ok {
			return admissionBlocked(out, current, guardResult)
		}
		out.Remainder = MergeAdmissionRemainder(remainder, out.Remainder)
		out.Guard = guardResult
		out.GuardErr = guardResult.ErrorText()
		out.Reason = AppendAdmissionReason(out.Reason, reason)
		out.Rewritten = true
		current = protect(fallback)
		out.Plan = current
	}
	guardResult := guard(current)
	if !guardResult.Empty() {
		return admissionBlocked(out, current, guardResult)
	}
	out.Plan = current
	return out
}

func duplicateActionEdgeGuard(plan dataquery.TaskPlan, blockedKeys []string) GuardResult {
	if len(plan.Actions) == 0 || len(blockedKeys) == 0 {
		return GuardResult{}
	}
	blocked := make(map[string]bool, len(blockedKeys))
	for _, key := range blockedKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		blocked[key] = true
	}
	if len(blocked) == 0 {
		return GuardResult{}
	}
	for _, action := range plan.Actions {
		key := ActionIdempotencyKey(action)
		if key == "" || !blocked[key] {
			continue
		}
		violation := NewGenericViolation(GenericViolationInput{
			Code:          "duplicate_action_edge",
			Severity:      "error",
			Repairability: RepairNeedsTypedAction,
			Action:        action,
			Reason:        "candidate action repeats a previously blocked or failed workflow edge with the same idempotency key",
		})
		return NewGuardResult("duplicate_action_edge", "error", RepairNeedsTypedAction, violation.Reason, violation)
	}
	return GuardResult{}
}

func admissionBlocked(out ActionDAGAdmissionDecision, current dataquery.TaskPlan, guard GuardResult) ActionDAGAdmissionDecision {
	out.Plan = current
	out.Guard = guard
	out.FinalGuard = guard
	out.GuardErr = guard.ErrorText()
	out.FinalGuardErr = guard.ErrorText()
	return out
}

func AppendAdmissionReason(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" || strings.Contains(current, next) {
		return current
	}
	return current + "; " + next
}

func MergeAdmissionRemainder(first, second dataquery.TaskPlan) dataquery.TaskPlan {
	if len(first.Actions) == 0 {
		return second
	}
	if len(second.Actions) == 0 {
		return first
	}
	out := first
	out.Actions = append(append([]dataquery.DataAction(nil), first.Actions...), second.Actions...)
	out.Script = ""
	out.ContinueAfter = true
	out.InputPaths = mergeAdmissionInputPaths(first.InputPaths, second.InputPaths)
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = strings.TrimSpace(second.NextBatch)
	}
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = strings.TrimSpace(second.WhyThisBatch)
	}
	return out
}

func mergeAdmissionInputPaths(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	seen := map[string]bool{}
	for _, value := range append(append([]string(nil), a...), b...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
