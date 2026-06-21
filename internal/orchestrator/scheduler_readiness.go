package orchestrator

import (
	"context"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/types"
)

type nodeReadinessReason string

const (
	nodeReadinessReasonReady                 nodeReadinessReason = "ready"
	nodeReadinessReasonNilNode               nodeReadinessReason = "nil_node"
	nodeReadinessReasonStageExcluded         nodeReadinessReason = "stage_excluded"
	nodeReadinessReasonCounterfactual        nodeReadinessReason = "counterfactual"
	nodeReadinessReasonStatusNotDispatchable nodeReadinessReason = "status_not_dispatchable"
	nodeReadinessReasonWaitingDependency     nodeReadinessReason = "waiting_on_dependency"
	nodeReadinessReasonFailedDependency      nodeReadinessReason = "failed_dependency"
	nodeReadinessReasonEntryCondition        nodeReadinessReason = "entry_condition_unmet"
	nodeReadinessReasonMaxRetriesExhausted   nodeReadinessReason = "max_retries_exhausted"
)

// nodeReadiness is the scheduler-local authority for whether a read DAG node
// can be dispatched in the current explore-family ready window. It consumes
// only typed graph/status/criterion state; rendered hints and model prose are
// downstream projections of this decision, never inputs.
type nodeReadiness struct {
	NodeID             string
	Status             nodeStatus
	Ready              bool
	Blocked            bool
	Skip               bool
	ReasonCode         nodeReadinessReason
	WaitingOn          []string
	FailedPredecessors []string
	FailedCriteria     []criterion.Result
	AttemptsUsed       int
	MaxAttempts        int
}

func (r nodeReadiness) toNodeBlock() nodeBlock {
	return nodeBlock{
		NodeID:                 r.NodeID,
		ReasonCode:             r.ReasonCode,
		FailedCriteria:         append([]criterion.Result(nil), r.FailedCriteria...),
		DependencyBlockers:     append([]string(nil), r.WaitingOn...),
		FailedDependencyBlocks: append([]string(nil), r.FailedPredecessors...),
		AttemptsUsed:           r.AttemptsUsed,
		MaxAttempts:            r.MaxAttempts,
	}
}

func (s *graphState) evaluateNodeReadinessContext(ctx context.Context, n *types.TaskNode, env criterion.Env) (nodeReadiness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nodeReadiness{}, err
	}
	if n == nil {
		return nodeReadiness{Skip: true, ReasonCode: nodeReadinessReasonNilNode}, nil
	}
	out := nodeReadiness{
		NodeID:       strings.TrimSpace(n.ID),
		Status:       s.nodeStatus(n.ID),
		AttemptsUsed: s.nodeAttempt(n.ID),
		MaxAttempts:  maxDispatchAttemptsForNode(n),
	}
	if n.IsCounterfactual {
		out.Skip = true
		out.ReasonCode = nodeReadinessReasonCounterfactual
		return out, nil
	}
	if n.Type == types.NodeExtract || n.Type == types.NodeFinalize {
		out.Skip = true
		out.ReasonCode = nodeReadinessReasonStageExcluded
		return out, nil
	}
	if out.Status != nodePending && out.Status != nodeRequeued {
		out.Skip = true
		out.ReasonCode = nodeReadinessReasonStatusNotDispatchable
		return out, nil
	}
	if out.Status == nodeRequeued && out.MaxAttempts > 0 && out.AttemptsUsed >= out.MaxAttempts {
		if n.Optional {
			out.Skip = true
		} else {
			out.Blocked = true
		}
		out.ReasonCode = nodeReadinessReasonMaxRetriesExhausted
		return out, nil
	}
	waiting, failed := s.hardDependencyPredecessorStateForNode(out.NodeID)
	if len(waiting) > 0 || len(failed) > 0 {
		out.WaitingOn = waiting
		out.FailedPredecessors = failed
		if n.Optional {
			out.Skip = true
		} else {
			out.Blocked = true
		}
		if len(failed) > 0 {
			out.ReasonCode = nodeReadinessReasonFailedDependency
		} else {
			out.ReasonCode = nodeReadinessReasonWaitingDependency
		}
		return out, nil
	}
	if len(n.EntryConditions) > 0 {
		ok, failedCriteria := criterion.EvalAll(n.EntryConditions, env)
		if !ok {
			out.FailedCriteria = append([]criterion.Result(nil), failedCriteria...)
			if n.Optional {
				out.Skip = true
			} else {
				out.Blocked = true
			}
			out.ReasonCode = nodeReadinessReasonEntryCondition
			return out, nil
		}
	}
	out.Ready = true
	out.ReasonCode = nodeReadinessReasonReady
	return out, nil
}

func maxDispatchAttemptsForNode(n *types.TaskNode) int {
	if n == nil || n.MaxRetries <= 0 {
		return 0
	}
	return n.MaxRetries + 1
}
