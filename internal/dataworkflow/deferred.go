package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type DeferredActionCandidate struct {
	Index         int                  `json:"index"`
	Action        dataquery.DataAction `json:"action"`
	Ready         bool                 `json:"ready"`
	BlockedReason string               `json:"blocked_reason,omitempty"`
}

type DeferredDispatchInput struct {
	Plan               dataquery.TaskPlan        `json:"plan"`
	Candidates         []DeferredActionCandidate `json:"candidates,omitempty"`
	AllowedNextActions []string                  `json:"allowed_next_actions,omitempty"`
}

type DeferredDispatchStatus struct {
	Actions         int    `json:"actions"`
	ReadyActions    int    `json:"ready_actions,omitempty"`
	BlockedActions  int    `json:"blocked_actions,omitempty"`
	FirstActionID   string `json:"first_action_id,omitempty"`
	FirstActionKind string `json:"first_action_kind,omitempty"`
	Ready           bool   `json:"ready,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

func BuildDeferredDispatchPlan(input DeferredDispatchInput) (dataquery.TaskPlan, dataquery.TaskPlan, DeferredDispatchStatus, bool) {
	status := DeferredDispatchStatus{Actions: len(input.Plan.Actions)}
	if len(input.Plan.Actions) == 0 {
		status.Reason = "deferred queue is empty"
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, status, false
	}
	first := input.Plan.Actions[0]
	status.FirstActionID = strings.TrimSpace(first.ID)
	status.FirstActionKind = string(NormalizeActionKind(first.Kind))
	allowed := allowedActionSet(input.AllowedNextActions)
	if len(allowed) == 0 {
		status.Reason = "workflow has no allowed next typed actions"
		status.BlockedActions = len(input.Plan.Actions)
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, status, false
	}
	keep, rest, reason := selectDeferredReadyRank(input.Plan.Actions, input.Candidates, allowed)
	status.ReadyActions = len(keep)
	status.BlockedActions = len(rest)
	if len(keep) == 0 {
		status.Reason = firstNonEmpty(reason, "no deferred action is executable from current artifact aliases and field contracts")
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, status, false
	}
	next := input.Plan
	next.Actions = keep
	next.Script = ""
	next.ContinueAfter = len(rest) > 0 || input.Plan.ContinueAfter
	if strings.TrimSpace(next.WhyThisBatch) == "" {
		next.WhyThisBatch = "continue the deferred typed data action rank from the accepted plan graph"
	}
	if strings.TrimSpace(next.NextBatch) == "" && len(rest) > 0 {
		next.NextBatch = "continue with the remaining deferred typed data action ranks after this batch materializes artifacts"
	}
	remainder := input.Plan
	remainder.Actions = rest
	remainder.Script = ""
	remainder.ContinueAfter = len(rest) > 0 || input.Plan.ContinueAfter
	status.Ready = true
	return next, remainder, status, true
}

func selectDeferredReadyRank(actions []dataquery.DataAction, candidates []DeferredActionCandidate, allowed map[string]bool) ([]dataquery.DataAction, []dataquery.DataAction, string) {
	if len(actions) == 0 {
		return nil, nil, "deferred queue is empty"
	}
	candidateByIndex := map[int]DeferredActionCandidate{}
	for _, candidate := range candidates {
		if candidate.Index < 0 || candidate.Index >= len(actions) {
			continue
		}
		candidateByIndex[candidate.Index] = candidate
	}
	firstBlockedReason := ""
	for i := range actions {
		candidate, ok := candidateByIndex[i]
		if !ok {
			candidate = DeferredActionCandidate{Index: i, Action: actions[i]}
		}
		action := candidate.Action
		if strings.TrimSpace(string(action.Kind)) == "" {
			action = actions[i]
		}
		if !candidate.Ready {
			if firstBlockedReason == "" {
				firstBlockedReason = firstNonEmpty(candidate.BlockedReason, deferredBlockedReason(actions[i], "not ready"))
			}
			continue
		}
		if !deferredActionAllowed(action, allowed) {
			if firstBlockedReason == "" {
				firstBlockedReason = deferredBlockedReason(action, "not allowed in current workflow stage")
			}
			continue
		}
		firstRank := actionDependencyRank(action)
		selected := map[int]bool{i: true}
		keep := []dataquery.DataAction{action}
		for j := i + 1; j < len(actions); j++ {
			nextCandidate, ok := candidateByIndex[j]
			if !ok {
				nextCandidate = DeferredActionCandidate{Index: j, Action: actions[j]}
			}
			next := nextCandidate.Action
			if strings.TrimSpace(string(next.Kind)) == "" {
				next = actions[j]
			}
			if !nextCandidate.Ready || !deferredActionAllowed(next, allowed) {
				break
			}
			nextRank := actionDependencyRank(next)
			if firstRank > 0 && nextRank > 0 && nextRank != firstRank {
				break
			}
			selected[j] = true
			keep = append(keep, next)
		}
		rest := make([]dataquery.DataAction, 0, len(actions)-len(keep))
		for j, action := range actions {
			if selected[j] {
				continue
			}
			rest = append(rest, action)
		}
		return keep, rest, ""
	}
	return nil, append([]dataquery.DataAction(nil), actions...), firstBlockedReason
}

func deferredActionAllowed(action dataquery.DataAction, allowed map[string]bool) bool {
	kind := string(NormalizeActionKind(action.Kind))
	if kind == "" || !allowed[kind] {
		return false
	}
	if NormalizeActionKind(action.Kind) == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" {
		return false
	}
	return true
}

func actionDependencyRank(action dataquery.DataAction) int {
	return DependencyRank(NormalizeActionKind(action.Kind))
}

func deferredBlockedReason(action dataquery.DataAction, reason string) string {
	name := firstNonEmpty(strings.TrimSpace(action.ID), string(NormalizeActionKind(action.Kind)), "deferred action")
	if strings.TrimSpace(reason) == "" {
		return name
	}
	return fmt.Sprintf("%s: %s", name, reason)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
