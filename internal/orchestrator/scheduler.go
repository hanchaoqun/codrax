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

	// transientRetryUsed counts retries triggered by transient
	// dispatch errors (stream stall / first-byte timeout / 429 / 5xx
	// / network blip). Decoupled from retryUsed because the budget
	// consumer semantics differ — see Orchestrator.transientRetryBudget
	// for the rationale. Driven by recordTransientRetry; capped by
	// the orchestrator's transientRetryBudget at the call site (the
	// graph itself does not carry this cap because read-mode graphs
	// emitted by the analyzer do not know about it; the orchestrator
	// owns the policy).
	transientRetryUsed int

	// transientStallSignatures records the most recent transient-failed
	// attempt's tool-call signature per node ID. The stall plateau
	// detector compares the current attempt's signature against this
	// to decide whether the LLM is making progress between transient
	// retries — if two consecutive transient failures produce the
	// SAME signature with no terminal emit in between, the stage is
	// structurally stuck and further retry is suppressed.
	//
	// Signature format: "|"-joined tool names in call order, or "" when
	// a terminal emit (emit_change_plan / emit_test_results / etc.)
	// fired, in which case the attempt made meaningful progress and
	// the next transient retry is allowed regardless of comparison.
	transientStallSignatures map[string]string

	// transientNoEmitStreak counts consecutive transient stalls per
	// node where the agent did NOT reach a terminal emit
	// (emit_change_plan / emit_test_results / emit_answer_document /
	// apply_patch). It is intentionally advisory. The signature-
	// equality check above is too narrow on its own — production trace
	// /home/chatpp/pytest 2026-04-29 06:03 showed an LLM that
	// changed tool tactics between rounds (round 1 ran repo_map +
	// list_files, round 2 ran list_files only) so signatures
	// differed. The streak counter is signature-agnostic, but because
	// bursty EOF / dial failures can produce the same no-emit shape,
	// callers must not use it alone as a hard gate. Reset to 0
	// whenever a terminal emit fires (computeStallSignature returns "").
	transientNoEmitStreak map[string]int

	// visitCount tracks how many times each node has been dispatched
	// (incremented on markRunning). Used by the write scheduler to
	// honour TaskNode.SkipOnFirstVisit — when true, the FIRST entry
	// dispatches as a no-op (markDone immediately) and only LATER
	// entries (post-retry) actually invoke the agent. Read-mode
	// scheduler ignores this counter; write-mode plan node uses it
	// to preserve R8a (don't regenerate the user-reviewed plan)
	// without losing access to retry-driven plan regeneration.
	visitCount map[string]int

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

	// upstreamFallbacksUsed (Block 3 architecture overhaul 2026-05-02)
	// counts every contract-failure retry that pushed the fallback
	// target deeper than FallbackFinalizerOnly (i.e. BackToExtract
	// or BackToExplore). Capped by the orchestrator's
	// maxUpstreamFallbacksPerRun so a Run cannot endlessly
	// re-investigate. Reaching the cap forces every subsequent
	// fallback to FailLoud.
	upstreamFallbacksUsed int
}

// yieldSnapshot captures the counters that drive F5's "did this
// retry window bring new information" decision. Four independent
// axes collectively encode "something changed":
//   - ForcedReads: the framework force-read files on the LLM's
//     behalf (I4 Lazy Auto-Read). Even a single forced read means
//     the next window sees a materially different ReadSet.
//   - ScannedSetSize: ranker pulled new files into scope (e.g. R5
//     ghost-anchor promotion in G7).
//   - DistinctViolationKindCount (A.2 audit, 2026-05-02): replaces
//     the historical ViolationsLogged scalar with a cardinality
//     measure of unique kinds in the ledger. Reason: ViolationsLogged
//     auto-bumps on every AppendViolation, so the "same kind keeps
//     re-firing" pathology trivially beat MinRetryYield=1 and the
//     kill gate became unreachable. Distinct-kind count grows ONLY
//     when a NEW kind appears, so re-firing the same kind yields
//     zero and the kill gate fires correctly.
//   - PatchesApplied: the F3 IRPatchEngine (G2) reconciled fields.
//     Any patch by definition resets downstream evaluation.
//
// Any one of these being non-zero at delta-compute time beats the
// default MinRetryYield=1 threshold; callers can raise the
// threshold to require multiple independent signals.
type yieldSnapshot struct {
	ForcedReads                int
	ScannedSetSize             int
	DistinctViolationKindCount int
	PatchesApplied             int
}

// nodeBlock carries a node's failed criteria into retry hints.
type nodeBlock struct {
	NodeID         string
	FailedCriteria []criterion.Result
}

func newGraphState(g types.TaskGraph) *graphState {
	s := &graphState{
		graph:      g,
		status:     make(map[string]nodeStatus, len(g.Nodes)),
		visitCount: make(map[string]int, len(g.Nodes)),
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
// transitions on the per-node status map. markRunning ALSO bumps the
// visitCount used by SkipOnFirstVisit logic — keep these in sync if
// any caller introduces a separate transition into the running state.
func (s *graphState) markRunning(id string) {
	s.status[id] = nodeRunning
	if s.visitCount != nil {
		s.visitCount[id]++
	}
}
func (s *graphState) markDone(id string)   { s.status[id] = nodeDone }
func (s *graphState) markFailed(id string) { s.status[id] = nodeFailed }
func (s *graphState) requeue(id string)    { s.status[id] = nodeRequeued }

// visits returns how many times the node has been dispatched. Zero
// before the first dispatch, 1 during the first run, etc. Used by
// the write scheduler to honour TaskNode.SkipOnFirstVisit.
func (s *graphState) visits(id string) int {
	if s == nil || s.visitCount == nil {
		return 0
	}
	return s.visitCount[id]
}

// retryBudgetExhausted reports whether the cumulative cross-window
// retry count has hit ExecutionPolicy.RetryBudget.
func (s *graphState) retryBudgetExhausted() bool {
	if s == nil {
		return true
	}
	return s.retryUsed >= s.graph.ExecutionPolicy.RetryBudget
}

func (s *graphState) recordRetry() { s.retryUsed++ }

// recordTransientRetry bumps the transient-dispatch retry counter
// (decoupled from recordRetry to keep SC / content retries isolated
// from network-blip retries). Caller must check
// transientRetryBudgetExhausted against the orchestrator's configured
// budget BEFORE calling — graphState does not know the cap.
func (s *graphState) recordTransientRetry() { s.transientRetryUsed++ }

// rememberTransientSignature stores the just-failed attempt's tool-
// call signature for nodeID so the next transient-retry decision can
// detect plateau (identical signature twice in a row). Empty signature
// is recorded verbatim — see computeStallSignature for the empty-
// means-progress contract.
func (s *graphState) rememberTransientSignature(nodeID, sig string) {
	if s == nil {
		return
	}
	if s.transientStallSignatures == nil {
		s.transientStallSignatures = map[string]string{}
	}
	s.transientStallSignatures[nodeID] = sig
}

// transientStallPlateau reports whether the just-failed attempt's
// signature matches the previous transient-failed attempt's signature
// for nodeID, i.e. the stage is stuck on the same wall. Empty
// signatures never count as a plateau (empty means "made progress")
// so a single emit between two stalls breaks the chain.
func (s *graphState) transientStallPlateau(nodeID, sig string) bool {
	if s == nil || s.transientStallSignatures == nil {
		return false
	}
	prev, ok := s.transientStallSignatures[nodeID]
	if !ok {
		return false
	}
	if prev == "" || sig == "" {
		return false
	}
	return prev == sig
}

// recordTransientNoEmitStall increments the consecutive-no-emit
// counter for nodeID. Called by the write scheduler whenever a
// transient retry is about to fire AND the just-finished attempt
// did not reach a terminal emit (signature is non-empty).
func (s *graphState) recordTransientNoEmitStall(nodeID string) {
	if s == nil {
		return
	}
	if s.transientNoEmitStreak == nil {
		s.transientNoEmitStreak = make(map[string]int)
	}
	s.transientNoEmitStreak[nodeID]++
}

// resetTransientNoEmitStreak clears the consecutive-no-emit counter
// for nodeID. Called when a stage attempt reaches a terminal emit
// (signature == "") so a subsequent stall starts the streak fresh.
func (s *graphState) resetTransientNoEmitStreak(nodeID string) {
	if s == nil || s.transientNoEmitStreak == nil {
		return
	}
	delete(s.transientNoEmitStreak, nodeID)
}

// transientNoEmitPlateau is the signature-AGNOSTIC advisory plateau predicate.
// Returns true when the node has accumulated >= 2 consecutive
// transient stalls without ever reaching a terminal emit. This
// catches the production-trace pattern where the LLM rotated
// navigation tactics between rounds (different tool sequences) but
// failed to reach emit_change_plan in either. Because bursty EOF /
// dial failures can produce the same no-emit shape, callers must not
// use this predicate alone as a hard gate.
//
// The threshold is fixed at 2 (one initial stall + one retry stall
// that also fails the same way). Configurable would be over-design
// for an advisory signal.
func (s *graphState) transientNoEmitPlateau(nodeID string) bool {
	if s == nil || s.transientNoEmitStreak == nil {
		return false
	}
	return s.transientNoEmitStreak[nodeID] >= 2
}

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
		ForcedReads:                stats.ForcedReads,
		ScannedSetSize:             len(closure.ScannedSet()),
		DistinctViolationKindCount: closure.DistinctViolationKindCount(),
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
//
// A.2 audit followup (2026-05-02): the historical ViolationsLogged
// axis was retired in favour of DistinctViolationKindCount.
// AppendViolation auto-bumped the global counter, so MinRetryYield=1
// was unreachable for any pathology that re-fired the same kind.
// Distinct-kind count grows ONLY on a new kind, so the kill gate now
// fires correctly when the same kind re-fires across windows. Other
// axes (ForcedReads, ScannedSetSize, PatchesApplied) keep their
// "any positive delta" semantics.
func yieldDelta(prev, now yieldSnapshot) int {
	d := 0
	if now.ForcedReads > prev.ForcedReads {
		d += now.ForcedReads - prev.ForcedReads
	}
	if now.ScannedSetSize > prev.ScannedSetSize {
		d += now.ScannedSetSize - prev.ScannedSetSize
	}
	if now.DistinctViolationKindCount > prev.DistinctViolationKindCount {
		d += now.DistinctViolationKindCount - prev.DistinctViolationKindCount
	}
	if now.PatchesApplied > prev.PatchesApplied {
		d += now.PatchesApplied - prev.PatchesApplied
	}
	return d
}

func shouldStopForLowRetryYield(minYield, retryUsed, delta int, fallback FallbackTarget) bool {
	if minYield <= 0 || retryUsed <= 0 || delta >= minYield {
		return false
	}
	// Finalizer-only repairs are prose/block rewrites using existing
	// evidence. Requiring a fresh evidence delta here kills the exact
	// retry window that should fix must_include, facet anchoring, or
	// topic_mismatch defects.
	return fallback != FallbackFinalizerOnly
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

// requeueToStage (Block 3 architecture overhaul 2026-05-02) is the
// selective companion to graphState.requeue + the legacy "requeue
// every done node" dance the contract-failure path used.
// requeueToStage walks every node in the graph, computes its
// scheduler stage via stageMapping, and requeues ONLY the nodes
// whose stage matches the target.
//
// Plus the finalize node always requeues because it sits at the
// bottom of every contract-failure retry — re-emitting the answer
// document is the universal output regardless of how deep the
// fallback rolled back.
//
// Returns the IDs of requeued nodes for the caller's logging /
// retry-hint render. The graph's writing flag is passed through
// to stageMapping so write-mode and read-mode share the API.
func (s *graphState) requeueToStage(target types.PipelineStage, writing bool) []string {
	if s == nil {
		return nil
	}
	var requeued []string
	for i := range s.graph.Nodes {
		n := &s.graph.Nodes[i]
		// Always requeue finalize — every contract-failure retry
		// re-emits the answer document.
		if n.Type == types.NodeFinalize {
			if s.status[n.ID] == nodeDone {
				s.status[n.ID] = nodeRequeued
				requeued = append(requeued, n.ID)
			}
			continue
		}
		stage, err := stageMapping(s.graph, n, writing)
		if err != nil {
			continue
		}
		// Selective requeue: only the target stage's nodes go back
		// to nodeRequeued. Everything else preserves its current
		// status so the next scheduler pass does NOT re-investigate
		// already-grounded evidence.
		if stage != target {
			continue
		}
		if s.status[n.ID] == nodeDone {
			s.status[n.ID] = nodeRequeued
			requeued = append(requeued, n.ID)
		}
	}
	return requeued
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
// Read-mode node types (Probe / Evidence / Validate / Reconcile) all
// route to StageExplore — the read scheduler batches them into a
// single explorer dispatch per round. Write-mode node types each map
// to their own stage so the write scheduler dispatches one at a time
// with stage-specific pre/post hooks.
func stageMapping(g types.TaskGraph, n *types.TaskNode, writing bool) (types.PipelineStage, error) {
	_ = g
	_ = writing
	switch n.Type {
	case types.NodeProbe, types.NodeEvidence, types.NodeValidate, types.NodeReconcile:
		return types.StageExplore, nil
	case types.NodeFinalize:
		return types.StageFinalize, nil
	case types.NodePlan:
		return types.StagePlan, nil
	case types.NodeApply:
		return types.StageApply, nil
	case types.NodeVerify:
		return types.StageVerify, nil
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
	prevStageRetry string,
	repairs []types.RepairDirective,
) string {
	if len(window) == 0 && len(blocks) == 0 && len(validationTargets) == 0 && prevViolation == "" && prevStageRetry == "" && len(repairs) == 0 {
		return ""
	}
	var b strings.Builder

	if prevViolation != "" {
		b.WriteString("The previous final answer failed the answer-shape contract:\n\n")
		b.WriteString("  " + prevViolation + "\n\n")
		b.WriteString("Re-run the investigation focused on closing this gap.\n\n")
	}
	if prevStageRetry != "" {
		b.WriteString("The previous explore attempt reported unresolved evidence gaps:\n\n")
		b.WriteString("  " + prevStageRetry + "\n\n")
		b.WriteString("Re-run the same stage focused on closing this structured handoff gap before extract/finalize.\n\n")
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
		if surface := idx[id]; surface != "" {
			return surface
		}
		if strings.Contains(id, ":") {
			return ""
		}
		return strings.TrimSpace(id)
	}
}

// envShape is a cheap structural fingerprint of the inputs that feed
// criterion.Env. The scheduler's tight loop uses it to decide whether
// a pure-read predicate (stopcond.ShouldStop, SuccessCriteria
// evaluation) would meaningfully re-evaluate vs. just return the same
// verdict it did last tick.
//
// The rule the struct enforces: any branch in the scheduler that
// evaluates a pure function over Env MUST compare the current shape
// against a snapshot captured at the previous evaluation. If the
// shape has not changed, re-evaluation is redundant at best and a
// hot-loop risk at worst (when the branch body mutates only graph
// state, not Env inputs, the predicate will fire indefinitely without
// progress).
//
// Fields are all int so the struct is naturally Go-comparable with
// `==` and `!=`, dedup/fingerprint without hashing. Every field is a
// cursor on an append-only or monotone state source:
//
//   - EvidenceCount:      len(BusContext.EvidenceItems)
//   - AnswerSymbolCount:  len(BusContext.AnswerSymbols)
//   - AnswerChainCount:   len(BusContext.AnswerChains)
//   - ToolResultCount:    len(BusContext.ToolResults)
//   - ReadSetSize:        |EvidenceClosure.ReadSet|
//   - PendingReadsSize:   |EvidenceClosure.PendingReads|
//   - DecidedHypotheses:  count of HypothesisSet entries with a
//     non-unknown non-empty Status
//   - PrescanBytes:       len(PrescanSummaryBlob)
//
// A change in any field means "something Env-visible advanced since
// last snapshot" — the predicate may now return a different verdict
// and re-evaluation is legitimate. All fields zero is the valid empty
// shape (used as the sentinel "never evaluated" value via pointer nil
// at call sites rather than zero-value confusion).
type envShape struct {
	EvidenceCount     int
	AnswerSymbolCount int
	AnswerChainCount  int
	ToolResultCount   int
	ReadSetSize       int
	PendingReadsSize  int
	DecidedHypotheses int
	PrescanBytes      int
}

// computeEnvShape captures the current cursor positions of every
// state source that feeds criterion.Env. Pure function; safe to call
// from anywhere that has a BusContext + Env. Nil bus is tolerated for
// unit-test ergonomics and returns the zero shape.
func computeEnvShape(bus *types.BusContext, env criterion.Env) envShape {
	s := envShape{
		EvidenceCount:     len(env.Evidence),
		AnswerSymbolCount: len(env.AnswerSymbols),
		AnswerChainCount:  len(env.AnswerChains),
		ToolResultCount:   len(env.ToolResults),
		PrescanBytes:      len(env.PrescanBlob),
	}
	if env.IR != nil {
		for _, h := range env.IR.HypothesisSet {
			if h.Status != "" && h.Status != types.HypUnknown {
				s.DecidedHypotheses++
			}
		}
	}
	if bus != nil && bus.Mutable != nil {
		if closure := bus.Mutable.EvidenceClosure(); closure != nil {
			s.ReadSetSize = len(closure.ReadSet())
			s.PendingReadsSize = len(closure.PendingReads())
		}
	}
	return s
}

// equals reports whether two shapes represent the same Env cursor
// position. Struct comparison is sufficient since every field is a
// scalar int and the fields cover all Env inputs the scheduler cares
// about.
func (a envShape) equals(b envShape) bool {
	return a == b
}

// hypProgress is the per-validate-node hypothesis-scope progress
// fingerprint. Complements envShape — session 22 extension.
//
// The bug envShape alone cannot catch: the explorer keeps emitting
// evidence that does not satisfy any unknown hypothesis's
// RequiredEvidence (e.g. traceback pasted with paths outside the
// repo → explorer fishes in codrax's own infrastructure). envShape's
// EvidenceCount / ToolResultCount / ReadSetSize advance each round,
// so envShape.equals() never triggers and the validate loop runs
// until step budget drains.
//
// hypProgress collapses the progress picture to two integers that
// only move when an unknown hypothesis actually inches toward
// decidable:
//
//   - UnknownCount:     count of hypotheses still in HypUnknown (or "")
//   - SatisfiedReqSum:  sum over those unknowns of criterion.EvalAll
//     (h.RequiredEvidence, env) hits — the same
//     primitive runAutoVerdicts uses to promote
//     HypUnknown → HypInconclusive
//
// Two SC failures with equal hypProgress ⇒ no unknown hypothesis
// advanced its RequiredEvidence satisfaction between them, even if
// global env shape did. Scheduler treats this as stuck (identical
// philosophy to envShape's full-env stall), with the OR semantics
// applied at the call site.
//
// Reused primitive: criterion.EvalAll is the same function the
// orchestrator already calls from runAutoVerdicts; adding a second
// fingerprint is a composition over existing capability, not a new
// evaluation pathway.
type hypProgress struct {
	UnknownCount    int
	SatisfiedReqSum int
}

// computeHypProgress captures per-validate-node hypothesis progress
// from the current Env. Pure function over the same Env the
// scheduler already hands to criterion.EvalAll; safe to call from
// any call site holding a buildEnv result.
func computeHypProgress(env criterion.Env) hypProgress {
	if env.IR == nil {
		return hypProgress{}
	}
	p := hypProgress{}
	for _, h := range env.IR.HypothesisSet {
		if h.Status != types.HypUnknown && h.Status != "" {
			continue
		}
		p.UnknownCount++
		if len(h.RequiredEvidence) == 0 {
			continue
		}
		for _, c := range h.RequiredEvidence {
			if criterion.Eval(c, env).Satisfied {
				p.SatisfiedReqSum++
			}
		}
	}
	return p
}

// equals reports whether two hypProgress fingerprints describe the
// same hypothesis-scope progress cursor. Cheap struct ==.
func (a hypProgress) equals(b hypProgress) bool {
	return a == b
}
