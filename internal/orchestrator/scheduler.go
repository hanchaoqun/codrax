package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// scheduler.go implements the Analyzer-v3 DAG scheduler. The orchestrator
// uses it whenever BusContext.AnalysisIR carries a non-empty TaskGraph;
// when AnalysisIR is nil the legacy stage-machine path runs instead.
//
// P1.3 conservative-schedule note (P1.3-MERGED-SCHEDULE):
//
// The graphState abstraction below tracks node-level pending/done/failed
// state and walks validation_feedback edges, but the runTaskGraph driver
// in orchestrator.go currently MERGES every non-finalize TaskNode into a
// single explorer dispatch per round to keep the 35-cell baseline from
// blowing up under 4-5× more LLM calls per task. The 4 capabilities that
// stay deferred behind this merge are documented in
// memory/project_p1_3_deferred_items.md (D1, D2, D5, D7) — read it
// before relaxing the merge in any future P-item.
//
// Concretely, the value the merged schedule actually preserves from the
// IR is:
//
//   - hard_dependency edge ordering (the merged window only advances
//     once all its predecessors are done)
//   - the explicit Finalize node's separation from the explore phase
//     (so the contract checker has a clean "answer just produced" hook)
//   - validation_feedback edges as a backtrack signal (collapsed today
//     into "re-run the merged explore window with a retry hint")
//
// What the merged schedule defers:
//
//   - separate dispatches per non-finalize TaskNodeType (D1)
//   - selective re-entry of only the evidence node on contract failure (D2)
//   - counterfactual branches as independent explorer passes (D5)
//   - hypothesis status write-back from validate nodes (D7)

// nodeStatus is the per-node execution state the scheduler tracks.
// The values are private to internal/orchestrator: downstream agents
// must not learn to read sibling node state, otherwise the
// "analyzer is the only IR writer" invariant erodes.
type nodeStatus string

const (
	nodePending  nodeStatus = "pending"
	nodeRunning  nodeStatus = "running"
	nodeDone     nodeStatus = "done"
	nodeFailed   nodeStatus = "failed"
	nodeRequeued nodeStatus = "requeued" // backtrack target waiting for its window to re-fire
)

// graphState tracks per-node status plus aggregate retry consumption.
// It is created fresh for every runTaskGraph call so cross-Run state
// can never leak between tasks.
type graphState struct {
	graph types.TaskGraph

	// status[id] is the current node state. All node IDs in g.Nodes
	// start as nodePending.
	status map[string]nodeStatus

	// attempts[id] counts how many times the node has been entered.
	// Used by canBacktrack to refuse re-entry once a node has been
	// attempted more than its individual MaxRetries.
	attempts map[string]int

	// retryUsed counts the cumulative number of cross-window retries
	// driven by validation_feedback edges or contract violations. The
	// driver compares it against ExecutionPolicy.RetryBudget.
	retryUsed int
}

// newGraphState constructs the state for a given TaskGraph. The
// returned state is fresh — every node is pending, no attempts.
func newGraphState(g types.TaskGraph) *graphState {
	s := &graphState{
		graph:    g,
		status:   make(map[string]nodeStatus, len(g.Nodes)),
		attempts: make(map[string]int, len(g.Nodes)),
	}
	for _, n := range g.Nodes {
		s.status[n.ID] = nodePending
	}
	return s
}

// readyNodes returns every node whose hard_dependency predecessors are
// all marked done AND whose own status is pending or requeued. The
// returned slice is sorted by critical_path order first, then by node
// declaration order, so the caller's iteration is deterministic.
//
// Counterfactual nodes are intentionally returned LAST in each batch
// so the merged schedule processes the main path before any
// counterfactual side branch (D5 deferred).
func (s *graphState) readyNodes() []*types.TaskNode {
	if s == nil || len(s.graph.Nodes) == 0 {
		return nil
	}
	predDone := func(nodeID string) bool {
		for _, e := range s.graph.Edges {
			if e.EdgeType != types.EdgeHardDependency || e.To != nodeID {
				continue
			}
			if s.status[e.From] != nodeDone {
				return false
			}
		}
		return true
	}

	var out []*types.TaskNode
	for i := range s.graph.Nodes {
		n := &s.graph.Nodes[i]
		st := s.status[n.ID]
		if st != nodePending && st != nodeRequeued {
			continue
		}
		if !predDone(n.ID) {
			continue
		}
		out = append(out, n)
	}

	// Critical-path priority: nodes whose ID appears in
	// ExecutionPolicy.CriticalPath sort first, in CriticalPath order.
	cpRank := make(map[string]int, len(s.graph.ExecutionPolicy.CriticalPath))
	for i, id := range s.graph.ExecutionPolicy.CriticalPath {
		cpRank[id] = i + 1 // 0 = "not on critical path"
	}
	declRank := make(map[string]int, len(s.graph.Nodes))
	for i, n := range s.graph.Nodes {
		declRank[n.ID] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		// Counterfactual nodes always last.
		if out[i].IsCounterfactual != out[j].IsCounterfactual {
			return !out[i].IsCounterfactual
		}
		ri, rj := cpRank[out[i].ID], cpRank[out[j].ID]
		if ri != rj {
			if ri == 0 {
				return false
			}
			if rj == 0 {
				return true
			}
			return ri < rj
		}
		return declRank[out[i].ID] < declRank[out[j].ID]
	})
	return out
}

// markRunning flips the node into running state and bumps its
// attempts counter. The driver calls this immediately before
// dispatching the agent so a crash-mid-dispatch leaves a visible
// state.
func (s *graphState) markRunning(id string) {
	if s == nil {
		return
	}
	s.status[id] = nodeRunning
	s.attempts[id]++
}

// markDone flips the node into done state. Idempotent.
func (s *graphState) markDone(id string) {
	if s == nil {
		return
	}
	s.status[id] = nodeDone
}

// markFailed flips the node into failed state. The driver decides
// separately whether to backtrack via feedbackTarget.
func (s *graphState) markFailed(id string) {
	if s == nil {
		return
	}
	s.status[id] = nodeFailed
}

// requeue resets a node from done/failed back into requeued so the
// next readyNodes() call picks it up again. Used by the contract-
// check backtrack path.
func (s *graphState) requeue(id string) {
	if s == nil {
		return
	}
	s.status[id] = nodeRequeued
}

// feedbackTarget walks validation_feedback edges out of fromID and
// returns the first edge target whose own attempt count is below its
// per-node MaxRetries (default unlimited when MaxRetries == 0).
// Returns "" when no valid backtrack target exists.
//
// In the P1.3 merged schedule, the driver consults feedbackTarget at
// contract-check failure time but currently ignores the returned ID
// and simply re-runs the merged explore window. The walk is still
// implemented because (a) it keeps the abstraction honest, (b) it
// gives unit tests something to lock down, and (c) D2 in the
// deferred memo will use it once node-level dispatch lands.
func (s *graphState) feedbackTarget(fromID string) string {
	if s == nil {
		return ""
	}
	for _, e := range s.graph.Edges {
		if e.EdgeType != types.EdgeValidationFeedback || e.From != fromID {
			continue
		}
		// Find the target node and check per-node retry cap.
		for i := range s.graph.Nodes {
			n := &s.graph.Nodes[i]
			if n.ID != e.To {
				continue
			}
			if n.MaxRetries > 0 && s.attempts[n.ID] >= n.MaxRetries+1 {
				continue
			}
			return n.ID
		}
	}
	return ""
}

// retryBudgetExhausted reports whether the cumulative cross-window
// retry count has hit ExecutionPolicy.RetryBudget. A budget of 0 is
// treated as "no retries allowed" (single-attempt run) — matching
// the safest interpretation; the templates in compiler/templates.go
// always set a non-zero budget for any scenario that emits a
// validation_feedback edge.
func (s *graphState) retryBudgetExhausted() bool {
	if s == nil {
		return true
	}
	return s.retryUsed >= s.graph.ExecutionPolicy.RetryBudget
}

// recordRetry increments the cross-window retry counter. The driver
// calls this once per backtrack window dispatch.
func (s *graphState) recordRetry() {
	if s == nil {
		return
	}
	s.retryUsed++
}

// allDone returns true when every node has reached the nodeDone or
// nodeFailed terminal. The driver loops until this is true or the
// budget runs out.
func (s *graphState) allDone() bool {
	if s == nil {
		return true
	}
	for _, st := range s.status {
		if st == nodePending || st == nodeRunning || st == nodeRequeued {
			return false
		}
	}
	return true
}

// firstFinalizeReady returns the first NodeFinalize whose hard
// predecessors are all done, or nil. The merged schedule treats the
// finalize node as the dispatch boundary between "explore window"
// and "finalize window" so we pull it out explicitly rather than
// hoping the readyNodes ordering pushes it last.
func (s *graphState) firstFinalizeReady() *types.TaskNode {
	for _, n := range s.readyNodes() {
		if n.Type == types.NodeFinalize {
			return n
		}
	}
	return nil
}

// pendingExplorerWindow returns EVERY pending non-finalize node,
// regardless of hard_dependency edge state. The merged schedule
// collapses the entire non-finalize subgraph into one explorer
// dispatch — the explorer's monolithic ReAct loop handles intra-
// window ordering internally, so we don't need the scheduler to
// linearize edges that the agent itself will walk.
//
// Returns nil when no non-finalize work remains. Counterfactual
// nodes are excluded (D5 deferred — they only run when node-level
// dispatch lands in P2.1, see scheduler.go header comment).
//
// The returned ordering follows declaration order so the rendered
// hint reads top-to-bottom in the same order the templates emit
// nodes (probe → evidence → validate → reconcile etc.).
func (s *graphState) pendingExplorerWindow() []*types.TaskNode {
	if s == nil {
		return nil
	}
	var window []*types.TaskNode
	for i := range s.graph.Nodes {
		n := &s.graph.Nodes[i]
		if n.Type == types.NodeFinalize {
			continue
		}
		if n.IsCounterfactual {
			continue
		}
		st := s.status[n.ID]
		if st != nodePending && st != nodeRequeued {
			continue
		}
		window = append(window, n)
	}
	return window
}

// firstFinalizeReadyMerged returns the first NodeFinalize whose
// non-finalize predecessors are ALL marked done. Unlike
// firstFinalizeReady (strict edge walk), this is the merged-schedule
// variant: it ignores validation_feedback edges and only checks that
// every non-finalize, non-counterfactual node is done.
func (s *graphState) firstFinalizeReadyMerged() *types.TaskNode {
	if s == nil {
		return nil
	}
	for i := range s.graph.Nodes {
		n := &s.graph.Nodes[i]
		if n.Type != types.NodeFinalize {
			continue
		}
		if s.status[n.ID] != nodePending && s.status[n.ID] != nodeRequeued {
			continue
		}
		// Check every other node is done.
		allOtherDone := true
		for _, m := range s.graph.Nodes {
			if m.ID == n.ID || m.Type == types.NodeFinalize || m.IsCounterfactual {
				continue
			}
			if s.status[m.ID] != nodeDone {
				allOtherDone = false
				break
			}
		}
		if allOtherDone {
			return n
		}
	}
	return nil
}

// stageMapping resolves a TaskNodeType into the PipelineStage and
// stageMapping returns the pipeline stage that a given TaskNode
// maps to. After the 2026-04-14 simplification codrax supports
// only read-only analysis node types (probe / evidence / validate
// / reconcile / finalize); the write node types (design / implement
// / review / verify) were deleted along with the write agents.
func stageMapping(g types.TaskGraph, n *types.TaskNode, writing bool) (types.PipelineStage, error) {
	switch n.Type {
	case types.NodeProbe, types.NodeEvidence, types.NodeValidate, types.NodeReconcile:
		// The four analysis node types collapse into the explorer
		// dispatch (the merged-window schedule).
		return types.StageExplore, nil
	case types.NodeFinalize:
		return types.StageFinalize, nil
	}
	return "", fmt.Errorf("scheduler: unknown task node type %q", n.Type)
}

// renderWindowHint builds a single Retry Directive (READ FIRST)
// content body that the explorer dispatch sees as ctx.RetryHint.
// The content is structural (objective + search hint surfaces) and
// contains no curated word lists.
//
// When prevViolation is non-empty, it is rendered first as the
// "previous answer rejected" preamble — that is how the contract-
// check backtrack path injects its diagnosis without touching any
// other state field.
func renderWindowHint(window []*types.TaskNode, termSurface func(string) string, prevViolation string) string {
	if len(window) == 0 && prevViolation == "" {
		return ""
	}
	var b strings.Builder
	if prevViolation != "" {
		b.WriteString("The previous final answer failed the answer-shape contract:\n\n")
		b.WriteString("  " + prevViolation + "\n\n")
		b.WriteString("Re-run the investigation focused on closing this gap.\n\n")
	}
	if len(window) > 0 {
		b.WriteString("DAG-scheduled investigation window. Cover every node objective below in this single dispatch:\n\n")
		for i, n := range window {
			fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, n.Type, n.Objective)
			if len(n.SearchHints.KeywordIDs)+len(n.SearchHints.EntityIDs) > 0 {
				surfaces := make([]string, 0, len(n.SearchHints.KeywordIDs)+len(n.SearchHints.EntityIDs))
				seen := make(map[string]bool)
				for _, id := range n.SearchHints.KeywordIDs {
					if s := termSurface(id); s != "" && !seen[s] {
						seen[s] = true
						surfaces = append(surfaces, s)
					}
				}
				for _, id := range n.SearchHints.EntityIDs {
					if s := termSurface(id); s != "" && !seen[s] {
						seen[s] = true
						surfaces = append(surfaces, s)
					}
				}
				if len(surfaces) > 0 {
					b.WriteString("   hints: " + strings.Join(surfaces, ", ") + "\n")
				}
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// termSurfaceLookup returns a closure that resolves a canonical term
// ID back to its surface string by walking the IR's term graph. A
// missing ID maps to the empty string so the caller can skip it
// without producing a placeholder. Kept private to this file because
// it's a scheduler-internal rendering helper.
func termSurfaceLookup(ir *types.AnalysisIR) func(string) string {
	if ir == nil {
		return func(string) string { return "" }
	}
	idx := make(map[string]string, len(ir.RequestModel.TermGraph.Canonical))
	for _, c := range ir.RequestModel.TermGraph.Canonical {
		idx[c.ID] = c.Surface
	}
	return func(id string) string {
		return idx[id]
	}
}
