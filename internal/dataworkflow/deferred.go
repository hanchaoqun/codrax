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
	BlockedCode   string               `json:"blocked_code,omitempty"`
	BlockedReason string               `json:"blocked_reason,omitempty"`
}

type DeferredDispatchInput struct {
	Plan               dataquery.TaskPlan        `json:"plan"`
	Candidates         []DeferredActionCandidate `json:"candidates,omitempty"`
	AllowedNextActions []string                  `json:"allowed_next_actions,omitempty"`
}

type DeferredDispatchStatus struct {
	Actions         int          `json:"actions"`
	ReadyActions    int          `json:"ready_actions,omitempty"`
	BlockedActions  int          `json:"blocked_actions,omitempty"`
	FirstActionID   string       `json:"first_action_id,omitempty"`
	FirstActionKind string       `json:"first_action_kind,omitempty"`
	Ready           bool         `json:"ready,omitempty"`
	ReasonCode      string       `json:"reason_code,omitempty"`
	Reason          string       `json:"reason,omitempty"`
	GuardCode       string       `json:"guard_code,omitempty"`
	Guard           *GuardResult `json:"guard,omitempty"`
}

const (
	DeferredBlockEmpty             = "deferred_empty"
	DeferredBlockNoAllowedActions  = "no_allowed_next_actions"
	DeferredBlockInputUnavailable  = "input_unavailable"
	DeferredBlockFieldContract     = "field_contract_violation"
	DeferredBlockStageNotAllowed   = "stage_not_allowed"
	DeferredBlockAdmissionRejected = "admission_rejected"
	DeferredBlockNotReady          = "not_ready"
	DeferredQueueLifecycleNone     = "none"
	DeferredQueueLifecycleRetain   = "retain"
	DeferredQueueLifecycleDiscard  = "discard"
)

type DeferredQueueLifecycleDecision struct {
	Action     string `json:"action"`
	ReasonCode string `json:"reason_code,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type DeferredQueueState struct {
	Plan   dataquery.TaskPlan   `json:"plan,omitempty"`
	Events []DeferredQueueEvent `json:"events,omitempty"`
}

type DeferredQueueEvent struct {
	Action           string `json:"action,omitempty"`
	Round            int    `json:"round,omitempty"`
	Actions          int    `json:"actions,omitempty"`
	RemainingActions int    `json:"remaining_actions,omitempty"`
	FirstActionID    string `json:"first_action_id,omitempty"`
	FirstActionKind  string `json:"first_action_kind,omitempty"`
	ReasonCode       string `json:"reason_code,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Ready            bool   `json:"ready,omitempty"`
}

const (
	DeferredQueueTransitionEnqueue  = "enqueue"
	DeferredQueueTransitionDispatch = "dispatch"
	DeferredQueueTransitionRetain   = "retain"
	DeferredQueueTransitionDiscard  = "discard"
	DeferredQueueTransitionClear    = "clear"
)

func NewDeferredQueue(plan dataquery.TaskPlan) DeferredQueueState {
	if len(plan.Actions) == 0 {
		return DeferredQueueState{}
	}
	copied := cloneTaskPlan(plan)
	if copied == nil {
		return DeferredQueueState{}
	}
	return DeferredQueueState{Plan: *copied}
}

func DeferredQueuePlan(queue DeferredQueueState) dataquery.TaskPlan {
	copied := cloneTaskPlan(queue.Plan)
	if copied == nil {
		return dataquery.TaskPlan{}
	}
	return *copied
}

func EnqueueDeferredQueue(queue DeferredQueueState, round int, plan dataquery.TaskPlan, reason string) DeferredQueueState {
	next := cloneDeferredQueue(queue)
	copied := cloneTaskPlan(plan)
	if copied == nil {
		next.Plan = dataquery.TaskPlan{}
		return next
	}
	next.Plan = *copied
	next.Events = appendDeferredQueueEvent(next.Events, buildDeferredQueueEvent(DeferredQueueTransitionEnqueue, round, next.Plan, DeferredDispatchStatus{}, reason))
	return next
}

func DispatchDeferredQueue(queue DeferredQueueState, round int, dispatched, remainder dataquery.TaskPlan, status DeferredDispatchStatus, reason string) DeferredQueueState {
	next := cloneDeferredQueue(queue)
	if copied := cloneTaskPlan(remainder); copied != nil {
		next.Plan = *copied
	} else {
		next.Plan = dataquery.TaskPlan{}
	}
	next.Events = appendDeferredQueueEvent(next.Events, buildDeferredQueueDispatchEvent(round, dispatched, remainder, status, reason))
	return next
}

func RetainDeferredQueue(queue DeferredQueueState, round int, status DeferredDispatchStatus) DeferredQueueState {
	next := cloneDeferredQueue(queue)
	next.Events = appendDeferredQueueEvent(next.Events, buildDeferredQueueEvent(DeferredQueueTransitionRetain, round, next.Plan, status, status.Reason))
	return next
}

func DiscardDeferredQueue(queue DeferredQueueState, round int, status DeferredDispatchStatus, reason string) DeferredQueueState {
	next := cloneDeferredQueue(queue)
	eventReason := firstNonEmpty(reason, status.Reason)
	next.Events = appendDeferredQueueEvent(next.Events, buildDeferredQueueEvent(DeferredQueueTransitionDiscard, round, next.Plan, status, eventReason))
	next.Plan = dataquery.TaskPlan{}
	return next
}

func ClearDeferredQueue(queue DeferredQueueState, round int, reason string) DeferredQueueState {
	next := cloneDeferredQueue(queue)
	if len(next.Plan.Actions) == 0 {
		return DeferredQueueState{}
	}
	next.Events = appendDeferredQueueEvent(next.Events, buildDeferredQueueEvent(DeferredQueueTransitionClear, round, next.Plan, DeferredDispatchStatus{}, reason))
	next.Plan = dataquery.TaskPlan{}
	return next
}

func BuildDeferredDispatchPlan(input DeferredDispatchInput) (dataquery.TaskPlan, dataquery.TaskPlan, DeferredDispatchStatus, bool) {
	status := DeferredDispatchStatus{Actions: len(input.Plan.Actions)}
	if len(input.Plan.Actions) == 0 {
		status.ReasonCode = DeferredBlockEmpty
		status.Reason = "deferred queue is empty"
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, status, false
	}
	first := input.Plan.Actions[0]
	status.FirstActionID = strings.TrimSpace(first.ID)
	status.FirstActionKind = string(NormalizeActionKind(first.Kind))
	allowed := allowedActionSet(input.AllowedNextActions)
	if len(allowed) == 0 {
		status.ReasonCode = DeferredBlockNoAllowedActions
		status.Reason = "workflow has no allowed next typed actions"
		status.BlockedActions = len(input.Plan.Actions)
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, status, false
	}
	keep, rest, reasonCode, reason := selectDeferredReadyRank(input.Plan.Actions, input.Candidates, allowed)
	status.ReadyActions = len(keep)
	status.BlockedActions = len(rest)
	if len(keep) == 0 {
		status.ReasonCode = firstNonEmpty(reasonCode, DeferredBlockNotReady)
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

func DecideDeferredQueueLifecycle(status DeferredDispatchStatus) DeferredQueueLifecycleDecision {
	if status.Actions == 0 || status.Ready {
		return DeferredQueueLifecycleDecision{Action: DeferredQueueLifecycleNone, ReasonCode: status.ReasonCode, Reason: status.Reason}
	}
	code := firstNonEmpty(status.ReasonCode, DeferredBlockNotReady)
	switch code {
	case DeferredBlockNoAllowedActions, DeferredBlockInputUnavailable, DeferredBlockFieldContract, DeferredBlockStageNotAllowed, DeferredBlockNotReady:
		return DeferredQueueLifecycleDecision{Action: DeferredQueueLifecycleRetain, ReasonCode: code, Reason: status.Reason}
	default:
		return DeferredQueueLifecycleDecision{Action: DeferredQueueLifecycleDiscard, ReasonCode: code, Reason: status.Reason}
	}
}

func DeferredQueueSnapshotForPlan(plan dataquery.TaskPlan) DeferredQueueSnapshot {
	status := DeferredDispatchStatus{Actions: len(plan.Actions)}
	if len(plan.Actions) > 0 {
		first := plan.Actions[0]
		status.FirstActionID = strings.TrimSpace(first.ID)
		status.FirstActionKind = string(NormalizeActionKind(first.Kind))
	}
	return DeferredQueueSnapshotForStatus(status, DeferredQueueLifecycleDecision{})
}

func DeferredQueueSnapshotForStatus(status DeferredDispatchStatus, decision DeferredQueueLifecycleDecision) DeferredQueueSnapshot {
	return DeferredQueueSnapshot{
		Actions:         status.Actions,
		ReadyActions:    status.ReadyActions,
		BlockedActions:  status.BlockedActions,
		FirstActionID:   strings.TrimSpace(status.FirstActionID),
		FirstActionKind: strings.TrimSpace(status.FirstActionKind),
		Ready:           status.Ready,
		ReasonCode:      strings.TrimSpace(status.ReasonCode),
		Reason:          strings.TrimSpace(status.Reason),
		LifecycleAction: strings.TrimSpace(decision.Action),
	}
}

func selectDeferredReadyRank(actions []dataquery.DataAction, candidates []DeferredActionCandidate, allowed map[string]bool) ([]dataquery.DataAction, []dataquery.DataAction, string, string) {
	if len(actions) == 0 {
		return nil, nil, DeferredBlockEmpty, "deferred queue is empty"
	}
	candidateByIndex := map[int]DeferredActionCandidate{}
	for _, candidate := range candidates {
		if candidate.Index < 0 || candidate.Index >= len(actions) {
			continue
		}
		candidateByIndex[candidate.Index] = candidate
	}
	firstBlockedCode := ""
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
				firstBlockedCode = firstNonEmpty(candidate.BlockedCode, DeferredBlockNotReady)
				firstBlockedReason = firstNonEmpty(candidate.BlockedReason, deferredBlockedReason(actions[i], "not ready"))
			}
			continue
		}
		if !deferredActionAllowed(action, allowed) {
			if firstBlockedReason == "" {
				firstBlockedCode = DeferredBlockStageNotAllowed
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
		return keep, rest, "", ""
	}
	return nil, append([]dataquery.DataAction(nil), actions...), firstNonEmpty(firstBlockedCode, DeferredBlockNotReady), firstBlockedReason
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

func cloneDeferredQueue(queue DeferredQueueState) DeferredQueueState {
	out := DeferredQueueState{}
	if copied := cloneTaskPlan(queue.Plan); copied != nil {
		out.Plan = *copied
	}
	out.Events = append([]DeferredQueueEvent(nil), queue.Events...)
	return out
}

func appendDeferredQueueEvent(events []DeferredQueueEvent, event DeferredQueueEvent) []DeferredQueueEvent {
	if event.Action == "" {
		return events
	}
	const maxDeferredQueueEvents = 32
	out := append(append([]DeferredQueueEvent(nil), events...), event)
	if len(out) > maxDeferredQueueEvents {
		out = out[len(out)-maxDeferredQueueEvents:]
	}
	return out
}

func buildDeferredQueueEvent(action string, round int, plan dataquery.TaskPlan, status DeferredDispatchStatus, reason string) DeferredQueueEvent {
	event := DeferredQueueEvent{
		Action:           strings.TrimSpace(action),
		Round:            round,
		Actions:          len(plan.Actions),
		RemainingActions: len(plan.Actions),
		FirstActionID:    strings.TrimSpace(status.FirstActionID),
		FirstActionKind:  strings.TrimSpace(status.FirstActionKind),
		ReasonCode:       strings.TrimSpace(status.ReasonCode),
		Reason:           strings.TrimSpace(firstNonEmpty(reason, status.Reason)),
		Ready:            status.Ready,
	}
	if len(plan.Actions) > 0 {
		first := plan.Actions[0]
		if event.FirstActionID == "" {
			event.FirstActionID = strings.TrimSpace(first.ID)
		}
		if event.FirstActionKind == "" {
			event.FirstActionKind = string(NormalizeActionKind(first.Kind))
		}
	}
	return event
}

func buildDeferredQueueDispatchEvent(round int, dispatched, remainder dataquery.TaskPlan, status DeferredDispatchStatus, reason string) DeferredQueueEvent {
	event := buildDeferredQueueEvent(DeferredQueueTransitionDispatch, round, dispatched, status, reason)
	event.Actions = len(dispatched.Actions)
	event.RemainingActions = len(remainder.Actions)
	event.Ready = true
	if len(dispatched.Actions) > 0 {
		first := dispatched.Actions[0]
		if event.FirstActionID == "" {
			event.FirstActionID = strings.TrimSpace(first.ID)
		}
		if event.FirstActionKind == "" {
			event.FirstActionKind = string(NormalizeActionKind(first.Kind))
		}
	}
	return event
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
