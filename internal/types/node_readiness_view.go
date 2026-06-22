package types

import (
	"sort"
	"strings"
)

// NodeReadinessReasonCode is the closed reason family for a typed read DAG
// readiness projection. Scheduler logic owns the decision; this surface is a
// read-only projection for status cards, replay, and audit consumers.
type NodeReadinessReasonCode string

const (
	NodeReadinessReasonUnknown               NodeReadinessReasonCode = "unknown"
	NodeReadinessReasonReady                 NodeReadinessReasonCode = "ready"
	NodeReadinessReasonNilNode               NodeReadinessReasonCode = "nil_node"
	NodeReadinessReasonStageExcluded         NodeReadinessReasonCode = "stage_excluded"
	NodeReadinessReasonCounterfactual        NodeReadinessReasonCode = "counterfactual"
	NodeReadinessReasonStatusNotDispatchable NodeReadinessReasonCode = "status_not_dispatchable"
	NodeReadinessReasonWaitingDependency     NodeReadinessReasonCode = "waiting_on_dependency"
	NodeReadinessReasonFailedDependency      NodeReadinessReasonCode = "failed_dependency"
	NodeReadinessReasonEntryCondition        NodeReadinessReasonCode = "entry_condition_unmet"
	NodeReadinessReasonMaxRetriesExhausted   NodeReadinessReasonCode = "max_retries_exhausted"
)

// NodeReadinessCriterionFailure is a prose-free summary of a failed typed
// criterion. Detail is emitted by deterministic criterion evaluators; no
// prompt text, model rationale, or user keyword matching belongs here.
type NodeReadinessCriterionFailure struct {
	Kind   string `json:"kind,omitempty"`
	Expr   string `json:"expr,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// NodeReadinessView is a normalized projection of the scheduler-local
// readiness authority. It must not become a second decision source: producers
// derive it from the scheduler result, and consumers render/audit it.
type NodeReadinessView struct {
	NodeID             string                          `json:"node_id,omitempty"`
	Status             NodeExecStatus                  `json:"status,omitempty"`
	Ready              bool                            `json:"ready,omitempty"`
	Blocked            bool                            `json:"blocked,omitempty"`
	Skip               bool                            `json:"skip,omitempty"`
	ReasonCode         NodeReadinessReasonCode         `json:"reason_code,omitempty"`
	WaitingOn          []string                        `json:"waiting_on,omitempty"`
	FailedPredecessors []string                        `json:"failed_predecessors,omitempty"`
	FailedCriteria     []NodeReadinessCriterionFailure `json:"failed_criteria,omitempty"`
	AttemptsUsed       int                             `json:"attempts_used,omitempty"`
	MaxAttempts        int                             `json:"max_attempts,omitempty"`
}

func NormalizeNodeReadinessReasonCode(reason NodeReadinessReasonCode) NodeReadinessReasonCode {
	switch NodeReadinessReasonCode(strings.TrimSpace(string(reason))) {
	case NodeReadinessReasonReady:
		return NodeReadinessReasonReady
	case NodeReadinessReasonNilNode:
		return NodeReadinessReasonNilNode
	case NodeReadinessReasonStageExcluded:
		return NodeReadinessReasonStageExcluded
	case NodeReadinessReasonCounterfactual:
		return NodeReadinessReasonCounterfactual
	case NodeReadinessReasonStatusNotDispatchable:
		return NodeReadinessReasonStatusNotDispatchable
	case NodeReadinessReasonWaitingDependency:
		return NodeReadinessReasonWaitingDependency
	case NodeReadinessReasonFailedDependency:
		return NodeReadinessReasonFailedDependency
	case NodeReadinessReasonEntryCondition:
		return NodeReadinessReasonEntryCondition
	case NodeReadinessReasonMaxRetriesExhausted:
		return NodeReadinessReasonMaxRetriesExhausted
	default:
		return NodeReadinessReasonUnknown
	}
}

func NormalizeNodeReadinessView(view NodeReadinessView) NodeReadinessView {
	view.NodeID = strings.TrimSpace(view.NodeID)
	view.Status = NormalizeNodeExecStatus(view.Status)
	view.ReasonCode = NormalizeNodeReadinessReasonCode(view.ReasonCode)
	view.WaitingOn = normalizeNodeReadinessStringSet(view.WaitingOn)
	view.FailedPredecessors = normalizeNodeReadinessStringSet(view.FailedPredecessors)
	view.FailedCriteria = normalizeNodeReadinessCriterionFailures(view.FailedCriteria)
	if view.AttemptsUsed < 0 {
		view.AttemptsUsed = 0
	}
	if view.MaxAttempts < 0 {
		view.MaxAttempts = 0
	}
	return view
}

func CloneNodeReadinessView(view NodeReadinessView) NodeReadinessView {
	return NormalizeNodeReadinessView(view)
}

func NormalizeNodeReadinessViews(views []NodeReadinessView) []NodeReadinessView {
	if len(views) == 0 {
		return nil
	}
	out := make([]NodeReadinessView, 0, len(views))
	for _, view := range views {
		view = NormalizeNodeReadinessView(view)
		if view.NodeID == "" && view.ReasonCode == NodeReadinessReasonUnknown {
			continue
		}
		out = append(out, view)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].NodeID == out[j].NodeID {
			return out[i].ReasonCode < out[j].ReasonCode
		}
		return out[i].NodeID < out[j].NodeID
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeNodeReadinessStringSet(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		value := strings.TrimSpace(raw)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeNodeReadinessCriterionFailures(in []NodeReadinessCriterionFailure) []NodeReadinessCriterionFailure {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]NodeReadinessCriterionFailure, 0, len(in))
	for _, failure := range in {
		failure.Kind = strings.TrimSpace(failure.Kind)
		failure.Expr = strings.TrimSpace(failure.Expr)
		failure.Detail = strings.TrimSpace(failure.Detail)
		key := failure.Kind + "\x00" + failure.Expr + "\x00" + failure.Detail
		if key == "\x00\x00" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, failure)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ki := out[i].Kind + "\x00" + out[i].Expr + "\x00" + out[i].Detail
		kj := out[j].Kind + "\x00" + out[j].Expr + "\x00" + out[j].Detail
		return ki < kj
	})
	if len(out) == 0 {
		return nil
	}
	return out
}
