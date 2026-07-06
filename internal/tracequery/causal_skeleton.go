package tracequery

import (
	"fmt"
	"strings"
)

// causal_skeleton.go — RCX③ model-facing root-cause skeleton (ledger
// docs/design/real_trace_campaign_20260705.md §12.3 ruling 1 item ③, user
// adjudication 2026-07-06). The engine publishes a TYPED causal spine — target
// dominant wait → direct explainer → upstream chain head → supply background —
// and the model configures the narrative over it. This is the third of the
// §12.3-1 three-piece set (① drill-debt and ② rank layering already landed in
// P0-E1); the skeleton is the last piece so the model narrates a structured
// spine instead of re-sorting incommensurable numbers.
//
// Discipline (feedback_no_system_backfill / feedback_precise_signals):
//   - the engine gives STRUCTURE (layer tags, measured ms, typed drill/source),
//     never a prose verdict — the model writes the words;
//   - MeasuredMs is per-layer wall clock, NEVER summed across layers (墙钟不可
//     加和, v3 projection ruling);
//   - nodes are ordered by causal LAYER, never re-sorted by the ms values
//     (cross-layer ms are incommensurable — §12.3-1 ② no-flat-sort rule);
//   - every input is a precise typed signal already computed elsewhere in the
//     bundle (rank items, blocking rows, chain nodes, supply summary) — the
//     skeleton is a projection, it computes nothing new.

// buildCausalSkeleton assembles the RCX③ typed skeleton from the already-built
// bundle components. Returns nil when there is no target-anchored evidence to
// skeletonize (an empty spine would be noise, not structure).
func buildCausalSkeleton(idx *Index, q Query, target ThreadRef, window TimeWindow, blocking *CriticalBlockingResult, chain *ChainResult, supply *SupplyPressureSummary) *CausalSkeleton {
	skel := &CausalSkeleton{Target: target, Window: window}

	// Layer 1 — target dominant wait: the TARGET's own window state
	// decomposition, dominant WAIT lane (F3 rework — see
	// skeletonTargetStateNode for why rank rows are the wrong source here).
	if node, ok := skeletonTargetStateNode(idx, q, target, window); ok {
		skel.Nodes = append(skel.Nodes, node)
	}

	// Layer 2 — direct explainer: the top resolved blocking row (lock holder /
	// binder peer + holder point), carrying the P0-E1/P0-E2a drill + source
	// verdicts so the model can say whether the explainer was itself examined
	// and whether it is payload-direct or a wakeup-edge inference.
	if node, ok := skeletonDirectBlockerNode(blocking); ok {
		skel.Nodes = append(skel.Nodes, node)
	}

	// Layer 3 — upstream chain head: the deepest node of the wakeup dependency
	// chain (the head the target ultimately waits on).
	if node, ok := skeletonUpstreamChainNode(target, chain); ok {
		skel.Nodes = append(skel.Nodes, node)
	}

	// Layer 4 — supply background: the window/CPU-scoped supply pressure. Never
	// a per-thread cause — the aggregate is explicitly a background node so the
	// model does not promote it into the direct chain.
	if node, ok := skeletonBackgroundNode(supply); ok {
		skel.Nodes = append(skel.Nodes, node)
	}

	if len(skel.Nodes) == 0 {
		return nil
	}
	return skel
}

// skeletonTargetStateNode (F3 rework, P0-E2b review): the target_state layer is
// the TARGET's own window state decomposition — its dominant WAIT lane
// (runnable / sleep / d_state / io_wait; running is excluded because the layer
// IS "the wait being explained").
//
// Why not rank rows (the original design, refuted): after P0-E1 the target's
// true dominant-wait row (e.g. its sleep_wait) frequently rides
// chain_relevance=adjacent, so an "on_chain ∧ subject==target" filter lands on
// sub-ms runnable fragments (500× distortion, empty state), and a rank-head
// fallback can even put the lock HOLDER into the target_state layer. The
// target's own timeline decomposition is the precise, filter-free source: the
// layer's semantics ("目标主导等待") are a property of the target's window,
// not of any candidate row. No fallback — when the target has no measurable
// wait in the window, the layer is honestly absent.
func skeletonTargetStateNode(idx *Index, q Query, target ThreadRef, window TimeWindow) (CausalSkeletonNode, bool) {
	if idx == nil || (target.PID <= 0 && strings.TrimSpace(target.Comm) == "") {
		return CausalSkeletonNode{}, false
	}
	if window.EndTs <= window.StartTs {
		return CausalSkeletonNode{}, false
	}
	tq := q
	tq.View = ""
	tq.PID = target.PID
	tq.Thread = target.Comm
	tq.ThreadInput = ""
	tq.TimeStart = window.StartTs
	tq.TimeEnd = window.EndTs
	bd := summarizeThreadStateBreakdown(ThreadTimeline(idx, tq))
	if bd == nil {
		return CausalSkeletonNode{}, false
	}
	// Dominant WAIT lane only: running is the target consuming CPU, not a wait
	// to explain — pass 0 for the running lane so it can never win.
	state, ms := dominantStateFromLanes(0, bd.RunnableMs, bd.SleepMs, bd.DStateMs, bd.IOWaitMs)
	if ms <= 0 {
		return CausalSkeletonNode{}, false
	}
	return CausalSkeletonNode{
		Layer:      CausalSkeletonLayerTargetState,
		Thread:     target,
		State:      state,
		MeasuredMs: ms,
		Note:       "the wait being explained (the target's own dominant wait state over the analysis window, from its timeline decomposition)",
	}, true
}

// skeletonDirectBlockerNode picks the direct explainer: the top blocking row
// whose counterpart is resolved, preferring on_chain resolved-contention rows
// (lock holder / binder peer) by measured duration.
func skeletonDirectBlockerNode(blocking *CriticalBlockingResult) (CausalSkeletonNode, bool) {
	if blocking == nil || len(blocking.Items) == 0 {
		return CausalSkeletonNode{}, false
	}
	var chosen *CriticalBlockingCandidate
	better := func(cand *CriticalBlockingCandidate) bool {
		if !threadRefResolved(cand.Peer) {
			return false
		}
		if chosen == nil {
			return true
		}
		// Prefer an on_chain resolved-contention row; then larger measured
		// duration. Pure typed comparisons, no prose.
		ca, cc := skeletonBlockingRowRank(*cand), skeletonBlockingRowRank(*chosen)
		if ca != cc {
			return ca > cc
		}
		return cand.DurationMs > chosen.DurationMs
	}
	for i := range blocking.Items {
		if better(&blocking.Items[i]) {
			chosen = &blocking.Items[i]
		}
	}
	if chosen == nil {
		return CausalSkeletonNode{}, false
	}
	node := CausalSkeletonNode{
		Layer:             CausalSkeletonLayerDirectBlocker,
		Thread:            chosen.Peer,
		State:             skeletonBlockerStateLabel(*chosen),
		MeasuredMs:        chosen.DurationMs,
		HolderSite:        chosen.HolderSite,
		DrillStatus:       chosen.DrillStatus,
		CounterpartSource: firstNonEmpty(chosen.HolderSource, chosen.PeerSource),
		Note:              skeletonBlockerNote(*chosen),
	}
	return node, true
}

// skeletonBlockingRowRank scores a blocking row's fitness as the direct
// explainer: on_chain resolved-contention beats off-chain / payload-less.
func skeletonBlockingRowRank(cand CriticalBlockingCandidate) int {
	score := 0
	if cand.ChainRelevance == "on_chain" {
		score += 2
	}
	if cand.BlockingKind != "" {
		score++
	}
	return score
}

func skeletonBlockerStateLabel(cand CriticalBlockingCandidate) string {
	if cand.BlockingKind != "" {
		return cand.BlockingKind
	}
	if cand.WaitObject != "" {
		return "blocked on " + cand.WaitObject
	}
	return cand.Type
}

func skeletonBlockerNote(cand CriticalBlockingCandidate) string {
	parts := []string{"direct explainer of the target's wait"}
	if cand.OwnerTidRaw > 0 {
		parts = append(parts, fmt.Sprintf("payload owner tid %d not in trace — counterpart inferred", cand.OwnerTidRaw))
	}
	return strings.Join(parts, "; ")
}

// skeletonUpstreamChainNode names the head of the wakeup dependency chain — the
// deepest chain node (largest ChainDepth), which is the upstream thread the
// target ultimately waits on. Skips the target's own node. F7: a head node
// with no measured duration or no state word is a noise node (a bare name adds
// no structure to the spine) — the layer is honestly absent instead.
func skeletonUpstreamChainNode(target ThreadRef, chain *ChainResult) (CausalSkeletonNode, bool) {
	if chain == nil || len(chain.Nodes) == 0 {
		return CausalSkeletonNode{}, false
	}
	var head *ChainNode
	headDepth := -1
	for i := range chain.Nodes {
		node := &chain.Nodes[i]
		if threadRefsSameSubject(node.Thread, target) {
			continue
		}
		// F7 noise gate: only nodes with a real measured duration AND a state
		// word can carry the layer.
		if node.DurationMs <= 0 || strings.TrimSpace(string(node.Dominant)) == "" {
			continue
		}
		depth := 0
		if node.Impact != nil {
			depth = node.Impact.ChainDepth
		}
		if head == nil || depth > headDepth {
			head, headDepth = node, depth
		}
	}
	if head == nil {
		return CausalSkeletonNode{}, false
	}
	return CausalSkeletonNode{
		Layer:      CausalSkeletonLayerUpstreamChain,
		Thread:     head.Thread,
		State:      string(head.Dominant),
		MeasuredMs: head.DurationMs,
		Note:       fmt.Sprintf("upstream chain head (depth %d) the target ultimately waits on", headDepth),
	}, true
}

// skeletonBackgroundNode surfaces the window/CPU-scoped supply pressure as an
// explicit BACKGROUND node (never a per-thread cause). MeasuredMs is the
// cross-thread cpu·ms aggregate — flagged in the note as an aggregate so the
// model does not read it as wall clock comparable to the per-layer values.
func skeletonBackgroundNode(supply *SupplyPressureSummary) (CausalSkeletonNode, bool) {
	if supply == nil || supply.CPUPressureMs <= 0 {
		return CausalSkeletonNode{}, false
	}
	note := "window-scoped supply pressure (cross-thread cpu·ms aggregate — background, never a per-thread cause; not comparable to the per-layer wall-clock values)"
	if len(supply.LowFrequencyCPUs) > 0 {
		note += fmt.Sprintf("; low-frequency cpus=%v", supply.LowFrequencyCPUs)
	}
	return CausalSkeletonNode{
		Layer:      CausalSkeletonLayerBackground,
		State:      supply.Signal,
		MeasuredMs: supply.CPUPressureMs,
		Note:       note,
	}, true
}

// threadRefsSameSubject is the precise subject-equality check shared by the
// skeleton layers: equal PID when both carry one, else equal trimmed comm.
func threadRefsSameSubject(a, b ThreadRef) bool {
	if a.PID > 0 && b.PID > 0 {
		return a.PID == b.PID
	}
	ca, cb := strings.TrimSpace(a.Comm), strings.TrimSpace(b.Comm)
	return ca != "" && ca == cb
}
