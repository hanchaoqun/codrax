package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/types"
)

// scheduler.go implements the Analyzer-v3 DAG scheduler. The
// orchestrator uses it to walk BusContext.AnalysisIR.TaskGraph with
// criterion-aware EntryConditions + SuccessCriteria + validation
// feedback semantics.
//
// Single-dispatch window model: non-finalize ready nodes are still
// collapsed into a single explorer dispatch per round (splitting
// them would multiply LLM calls), but the membership test for
// "ready" is tighter than the old model:
//
//   - hard_dependency predecessors must all be nodeDone;
//   - criterion.EvalAll(n.EntryConditions, env) must return allOK;
//   - node state must be pending or requeued.
//
// After each dispatch, SuccessCriteria are evaluated per node. A
// validate node whose SuccessCriteria fail triggers
// requeueValidationTargets, which walks the
// EdgeValidationFeedback edges from the validate node and only
// requeues the named upstream evidence nodes — NOT every
// previously-done explore node. This is the fine-grained backtrack
// the pre-refactor scheduler never honored.

// nodeStatus is the per-node execution state.
type nodeStatus string

const (
	nodePending  nodeStatus = "pending"
	nodeRunning  nodeStatus = "running"
	nodeDone     nodeStatus = "done"
	nodeFailed   nodeStatus = "failed"
	nodeRequeued nodeStatus = "requeued"
)

// graphState tracks per-node status plus aggregate retry consumption.
type graphState struct {
	graph     types.TaskGraph
	status    map[string]nodeStatus
	retryUsed int

	// Session 11 F5 per-kind retry bookkeeping + yield check
	// state. retryUsedByKind tracks how many retries each
	// ViolationKind has consumed (drives the C6 per-kind budget);
	// lastYieldSnapshot holds the "before this retry window"
	// counters so the next retry decision can compute a delta;
	// yieldKillCount records how many times the yield gate
	// forced an early terminate for the fail-loud warning.
	retryUsedByKind   map[types.ViolationKind]int
	lastYieldSnapshot yieldSnapshot
	yieldKillCount    int
}

// yieldSnapshot captures the counters that drive F5's "did this
// retry window bring new information" decision. Four independent
// axes collectively encode "something changed":
//   - ForcedReads: the framework force-read files on the LLM's
//     behalf (I4 Lazy Auto-Read). Even a single forced read means
//     the next window sees a materially different ReadSet.
//   - ScannedSetSize: ranker pulled new files into scope (e.g. R5
//     ghost-anchor promotion in G7).
//   - ViolationsLogged: enforcers surfaced new structured
//     diagnostics (F1 ledger entries). Even without I/O, ledger
//     drift means the aggregator / patcher has new fuel.
//   - PatchesApplied: the F3 IRPatchEngine (G2) reconciled fields.
//     Any patch by definition resets downstream evaluation.
//
// Any one of these being non-zero at delta-compute time beats the
// default MinRetryYield=1 threshold; callers can raise the
// threshold to require multiple independent signals.
type yieldSnapshot struct {
	ForcedReads      int
	ScannedSetSize   int
	ViolationsLogged int
	PatchesApplied   int
}

// nodeBlock carries a node's failed criteria into retry hints.
type nodeBlock struct {
	NodeID         string
	FailedCriteria []criterion.Result
}

func newGraphState(g types.TaskGraph) *graphState {
	s := &graphState{
		graph:  g,
		status: make(map[string]nodeStatus, len(g.Nodes)),
	}
	for _, n := range g.Nodes {
		s.status[n.ID] = nodePending
	}
	return s
}

// readyExplorerWindow collects every non-finalize, non-counterfactual
// node that is still pending or requeued AND whose EntryConditions
// are all satisfied against env. Hard-dependency edges are NOT
// honored here: the merged-window schedule dispatches the entire
// non-finalize subgraph in a single explorer call, and the
// explorer's ReAct loop walks the declared objective order
// internally. blocked carries nodes whose entry conditions are not
// yet met so renderWindowHint can surface which criterion is
// stalling them.
func (s *graphState) readyExplorerWindow(env criterion.Env) (ready []*types.TaskNode, blocked []nodeBlock) {
	if s == nil || len(s.graph.Nodes) == 0 {
		return nil, nil
	}
	for i := range s.graph.Nodes {
		n := &s.graph.Nodes[i]
		if n.Type == types.NodeFinalize || n.IsCounterfactual {
			continue
		}
		st := s.status[n.ID]
		if st != nodePending && st != nodeRequeued {
			continue
		}
		if len(n.EntryConditions) > 0 {
			ok, failed := criterion.EvalAll(n.EntryConditions, env)
			if !ok {
				blocked = append(blocked, nodeBlock{NodeID: n.ID, FailedCriteria: failed})
				continue
			}
		}
		ready = append(ready, n)
	}
	sort.SliceStable(ready, func(i, j int) bool {
		return declOrder(s.graph, ready[i].ID) < declOrder(s.graph, ready[j].ID)
	})
	return ready, blocked
}

func declOrder(g types.TaskGraph, id string) int {
	for i, n := range g.Nodes {
		if n.ID == id {
			return i
		}
	}
	return 1 << 30
}

// markRunning / markDone / markFailed / requeue are unchanged state
// transitions on the per-node status map.
func (s *graphState) markRunning(id string) { s.status[id] = nodeRunning }
func (s *graphState) markDone(id string)    { s.status[id] = nodeDone }
func (s *graphState) markFailed(id string)  { s.status[id] = nodeFailed }
func (s *graphState) requeue(id string)     { s.status[id] = nodeRequeued }

// retryBudgetExhausted reports whether the cumulative cross-window
// retry count has hit ExecutionPolicy.RetryBudget.
func (s *graphState) retryBudgetExhausted() bool {
	if s == nil {
		return true
	}
	return s.retryUsed >= s.graph.ExecutionPolicy.RetryBudget
}

func (s *graphState) recordRetry() { s.retryUsed++ }

// recordRetryByKind bumps the Session 11 C6 per-kind counter so
// shouldKillRetryByKind can apply the configured per-kind cap on
// the next iteration.
func (s *graphState) recordRetryByKind(kind types.ViolationKind) {
	if s == nil {
		return
	}
	if s.retryUsedByKind == nil {
		s.retryUsedByKind = make(map[types.ViolationKind]int)
	}
	s.retryUsedByKind[kind]++
}

// retryUsedForKind returns the consumed count for kind; zero when
// unset.
func (s *graphState) retryUsedForKind(kind types.ViolationKind) int {
	if s == nil || s.retryUsedByKind == nil {
		return 0
	}
	return s.retryUsedByKind[kind]
}

// captureYieldSnapshot takes a "before this window" reading of
// the yield-relevant counters off closure. Callers persist the
// snapshot on graphState and compare against the post-window
// closure state when deciding whether the retry earned enough
// new information to continue.
func captureYieldSnapshot(closure *types.EvidenceClosure) yieldSnapshot {
	if closure == nil {
		return yieldSnapshot{}
	}
	stats := closure.Stats()
	return yieldSnapshot{
		ForcedReads:      stats.ForcedReads,
		ScannedSetSize:   len(closure.ScannedSet()),
		ViolationsLogged: stats.ViolationsLogged,
		// PatchesApplied is wired via the F3 patcher (G2); for G4
		// we leave it zero so the delta still returns meaningful
		// signal from the other three axes.
		PatchesApplied: 0,
	}
}

// yieldDelta computes the sum of per-axis increments between
// `prev` (captured before the window) and `now` (captured after
// the window). A zero return means the window produced no new
// information on any axis — exactly the condition F5's kill gate
// watches for.
func yieldDelta(prev, now yieldSnapshot) int {
	d := 0
	if now.ForcedReads > prev.ForcedReads {
		d += now.ForcedReads - prev.ForcedReads
	}
	if now.ScannedSetSize > prev.ScannedSetSize {
		d += now.ScannedSetSize - prev.ScannedSetSize
	}
	if now.ViolationsLogged > prev.ViolationsLogged {
		d += now.ViolationsLogged - prev.ViolationsLogged
	}
	if now.PatchesApplied > prev.PatchesApplied {
		d += now.PatchesApplied - prev.PatchesApplied
	}
	return d
}

// allDone returns true when every node is done or failed.
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

// firstFinalizeReadyMerged returns the finalize node when every
// other non-counterfactual node is marked done. Unchanged semantics
// from pre-refactor.
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

// requeueValidationTargets follows EdgeValidationFeedback from the
// named validate node back to its upstream evidence nodes. Only
// those upstream nodes (and the validate node itself) get requeued;
// other done nodes stay done.
func (s *graphState) requeueValidationTargets(validateID string) []string {
	if s == nil {
		return nil
	}
	var out []string
	for _, e := range s.graph.Edges {
		if e.EdgeType != types.EdgeValidationFeedback || e.From != validateID {
			continue
		}
		if s.status[e.To] == nodeDone {
			s.status[e.To] = nodeRequeued
			out = append(out, e.To)
		}
	}
	s.status[validateID] = nodeRequeued
	return out
}

// forceCloseExploreWindow marks every non-finalize, non-counterfactual
// node as done so the next firstFinalizeReadyMerged call pulls the
// finalize node through. Called when stopcond.ShouldStop fires.
func (s *graphState) forceCloseExploreWindow() {
	if s == nil {
		return
	}
	for _, n := range s.graph.Nodes {
		if n.Type == types.NodeFinalize || n.IsCounterfactual {
			continue
		}
		if s.status[n.ID] != nodeDone {
			s.status[n.ID] = nodeDone
		}
	}
}

// markSuccessCriteriaFailed evaluates the node's SuccessCriteria
// against env. Returns (ok=true, nil) when every criterion is
// satisfied; (ok=false, failed list) otherwise.
func (s *graphState) markSuccessCriteriaFailed(n *types.TaskNode, env criterion.Env) (ok bool, failed []criterion.Result) {
	if len(n.SuccessCriteria) == 0 {
		return true, nil
	}
	allOK, failedList := criterion.EvalAll(n.SuccessCriteria, env)
	return allOK, failedList
}

// stageMapping returns the pipeline stage for a given TaskNode type.
func stageMapping(g types.TaskGraph, n *types.TaskNode, writing bool) (types.PipelineStage, error) {
	_ = g
	_ = writing
	switch n.Type {
	case types.NodeProbe, types.NodeEvidence, types.NodeValidate, types.NodeReconcile:
		return types.StageExplore, nil
	case types.NodeFinalize:
		return types.StageFinalize, nil
	}
	return "", fmt.Errorf("scheduler: unknown task node type %q", n.Type)
}

// renderWindowHint builds the Retry Directive body for the current
// explorer dispatch. window is the set of nodes being dispatched;
// blocks is the set of nodes stalled at EntryConditions; validationTargets
// is the list of nodes requeued by a validation_feedback edge; prevViolation
// is the contract checker's diagnosis from a previously-failed finalize;
// repairs is the structured CGEC RepairDirective queue drained from
// MutableState.EvidenceClosure (each directive renders its own retry-
// hint section via RepairDirective.Render).
func renderWindowHint(
	window []*types.TaskNode,
	blocks []nodeBlock,
	validationTargets []string,
	termSurface func(string) string,
	prevViolation string,
	repairs []types.RepairDirective,
) string {
	if len(window) == 0 && len(blocks) == 0 && len(validationTargets) == 0 && prevViolation == "" && len(repairs) == 0 {
		return ""
	}
	var b strings.Builder

	if prevViolation != "" {
		b.WriteString("The previous final answer failed the answer-shape contract:\n\n")
		b.WriteString("  " + prevViolation + "\n\n")
		b.WriteString("Re-run the investigation focused on closing this gap.\n\n")
	}

	// CGEC D2: structured RepairDirective sections come BEFORE the
	// DAG window so the LLM sees the framework's specific demands
	// (forced reads, subject constraint, shape reconcile) before
	// the generic objectives. De-dup happens in MergeRepairs so two
	// enforcers raising the same directive surface only once.
	if mergedRepairs := types.MergeRepairs(repairs); len(mergedRepairs) > 0 {
		for _, r := range mergedRepairs {
			rendered := r.Render()
			if rendered == "" {
				continue
			}
			b.WriteString(rendered)
			b.WriteString("\n")
		}
	}
	if len(validationTargets) > 0 {
		b.WriteString("Validation rejected the previous verdicts. Re-run ONLY these upstream evidence nodes: ")
		b.WriteString(strings.Join(validationTargets, ", "))
		b.WriteString(".\n\n")
	}
	if len(window) > 0 {
		b.WriteString("DAG-scheduled investigation window. Cover every node objective below in this dispatch:\n\n")
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
	if len(blocks) > 0 {
		b.WriteString("\nEntry-condition gate: the following nodes are waiting on unmet criteria:\n")
		for _, bk := range blocks {
			for _, fc := range bk.FailedCriteria {
				fmt.Fprintf(&b, "  - %s: %s (%s) — %s\n", bk.NodeID, fc.Kind, fc.Expr, fc.Detail)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// termSurfaceLookup returns a closure that resolves a canonical term
// ID back to its surface string.
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
