package tracequery

// rank_semantic_edge_anchor_r3.go — R3-IMPL: a NON-target thread's
// deterministic semantic span anchors on the HOST's own in-window typed
// wakeup edge toward the analysis target (§29.88.1 user ruling 2026-07-14;
// §29.88.2 R4 排他通则; ledger docs/design/real_trace_campaign_20260705.md).
//
// Ruling: 宿主线程对目标线程存在窗内 typed 唤醒边(直接或经链上多跳间接),
// 且 span 段位于该唤醒边之前(影响解除前)→ 该部分算链上席;无边、或位于
// 边后(影响已解除)的部分 → ◇ 邻近。与 R2 同一锚定语义(边=凭证,边前=
// 有效,边后=解除);span 跨边时按边界二分。
//
// Credential inventory — EXISTING typed edge infrastructure only (禁自造第二
// 套边判定):
//   - direct: the raw wakeup-edge census pair host→target
//     (ChainResult.WakeupEdgeCensus, WAKE-CENSUS-D 2A — window-total raw
//     sched_wakeup/sched_waking counting; target-wakee pairs are pair-cap
//     IMMUNE, so a direct credential can never fall to census overflow).
//     SCAN-3 positive sentinel: tieba 61839→59566 裸边 34579.496810 行5639 —
//     an edge the chain expansion never consumed (it woke an already-runnable
//     target), invisible to the chain-window intersection lane.
//   - multi-hop: the host's own edge in ChainResult.Edges (凭证沿链传递 — a
//     chain edge's wakee is a chain node whose path reaches the target by
//     construction; the 60595 depth-2 判例形). The credential is always the
//     host's OWN edge: a span merely straddling SOMEONE ELSE's edge earns
//     nothing (donghu 17267 decoy — span2 crosses binder:496_9→17267 while
//     its host 17284 holds no edge → no seat).
//
// Boundary semantics: an instant t is 边前 iff SOME credential edge has
// ts ≥ t, so the anchored share of an interval is its part at or before the
// LATEST in-window credential edge (最晚相关边 — per instant the operative
// edge is the nearest one AFTER it, and the union of those per-edge
// pre-windows is exactly [windowStart, maxEdgeTs]).
// An edge BEFORE the whole span grants nothing (边后=解除): the SCAN-3
// negative sentinel window ends 0.9ms before the 496810 edge and the six
// earlier 61839→59566 edges (…465879) all precede the span — no credential.
//
// Carve-outs (typed, structural):
//   - the analysis TARGET's own spans never take this lane (SELF-SEM
//     §29.61.1 owns them — checked before this arm; the predicate also
//     excludes the target defensively);
//   - a genuine chain-window intersection keeps the legacy overlap lane
//     byte-identically (checked before this arm — the overlap value IS the
//     R2 jump-window anchoring, witness-pinned since §24.10).

import (
	"fmt"
	"strings"
)

// semanticHostEdgeAnchor is the typed credential verdict for one host thread
// against one chain universe.
type semanticHostEdgeAnchor struct {
	// boundaryTs — the LATEST in-window credential edge timestamp (the
	// bisection boundary; > 0 iff a credential exists).
	boundaryTs float64
	// direct / viaChainHop — which typed inventories supplied credentials.
	direct      bool
	viaChainHop bool
}

// via returns the closed-set disclosure token (HostWakeupEdgeAnchorVia*).
func (a semanticHostEdgeAnchor) via() string {
	switch {
	case a.direct && a.viaChainHop:
		return HostWakeupEdgeAnchorViaDirectChainHop
	case a.viaChainHop:
		return HostWakeupEdgeAnchorViaChainHop
	case a.direct:
		return HostWakeupEdgeAnchorViaDirect
	default:
		return ""
	}
}

// hostSemanticSpanEdgeAnchor collects the host's own in-window typed wakeup
// credentials toward the target. ok=false means NO credential (the span keeps
// its legacy lane byte-identically — 宁漏勿猜). All-typed gates; never a
// label/prose heuristic.
func hostSemanticSpanEdgeAnchor(chain *ChainResult, host ThreadRef) (semanticHostEdgeAnchor, bool) {
	var anchor semanticHostEdgeAnchor
	if chain == nil {
		return anchor, false
	}
	if len(chain.Nodes) == 0 && len(chain.Edges) == 0 && len(chain.CausalImpacts) == 0 {
		return anchor, false
	}
	if chain.Target.PID <= 0 && strings.TrimSpace(chain.Target.Comm) == "" {
		return anchor, false
	}
	if host.PID <= 0 && strings.TrimSpace(host.Comm) == "" {
		return anchor, false
	}
	// R4 carve (§29.88.2): the target's own rows have no 「影响目标的边」
	// concept — SELF-SEM/SELF-ALL own them; this lane is host-only.
	if sameThreadRef(host, chain.Target) {
		return anchor, false
	}
	inWindow := func(ts float64) bool {
		if chain.Window.EndTs > chain.Window.StartTs {
			return ts >= chain.Window.StartTs && ts <= chain.Window.EndTs
		}
		// 修复轮件5 (冷读观察①): a degenerate/absent chain window proves
		// NOTHING about edge membership — fail closed (宁漏勿猜; the former
		// ts>0 fallback was a fail-open door this hard credential gate must
		// not have).
		return false
	}
	// Direct credential: raw census pair host→target. The census rows are
	// window-total by construction; the LastTs window check stays as an
	// honest belt (a clamped/degenerate window never over-grants).
	for _, pair := range chain.WakeupEdgeCensus {
		if pair.Count <= 0 || !sameThreadRef(pair.Waker, host) || !sameThreadRef(pair.Wakee, chain.Target) {
			continue
		}
		if pair.LastTs <= 0 || !inWindow(pair.LastTs) {
			continue
		}
		anchor.direct = true
		if pair.LastTs > anchor.boundaryTs {
			anchor.boundaryTs = pair.LastTs
		}
	}
	// Multi-hop credential: the host's OWN chain edge (its wakee sits on the
	// causal path to the target by chain construction — 凭证沿链传递).
	for _, edge := range chain.Edges {
		if !sameThreadRef(edge.Waker, host) {
			continue
		}
		if edge.WakeupTs <= 0 || !inWindow(edge.WakeupTs) {
			continue
		}
		anchor.viaChainHop = true
		if edge.WakeupTs > anchor.boundaryTs {
			anchor.boundaryTs = edge.WakeupTs
		}
	}
	return anchor, anchor.boundaryTs > 0
}

// semanticEdgeAnchorSplit bisects one (already window-clipped) interval at
// the credential boundary (跨边按边界二分): the pre-edge share
// [start, min(end, boundary)] stays on the chain seat, the post-edge share
// [max(start, boundary), end] rides the ◇ remainder. Either share may be
// empty (zero-width); the two shares are disjoint and sum back to the
// interval exactly (the ONLY additive relation form — RSPA 同源二分 shared
// semantics).
func semanticEdgeAnchorSplit(start, end, boundary float64) (preStart, preEnd, postStart, postEnd float64) {
	if end <= start || boundary <= 0 {
		return 0, 0, 0, 0
	}
	if start < boundary {
		preStart, preEnd = start, minFloat(end, boundary)
	}
	if end > boundary {
		postStart, postEnd = maxFloat(start, boundary), end
	}
	return preStart, preEnd, postStart, postEnd
}

// semanticEdgeAnchorRemainderSeat clones the ◇ remainder seat from an
// edge-anchored semantic seat (跨边二分的边后半席). It reuses the RSPA
// remainder typed trio (ChainAnchoredMs/ChainAnchorFullMs/
// ChainAnchorRemainderSeat) so every downstream face — the 行2 同源二分
// sentence, the ◇ side-lane capacity, the same-fact absorb key fork and the
// twin-visibility sweep — consumes it through the existing plumbing. The
// remainder value is the post-boundary in-window extent; the seat pair
// partitions the span's full in-window projection exactly.
func semanticEdgeAnchorRemainderSeat(seat RootCauseRankItem) (RootCauseRankItem, bool) {
	remStart, remEnd := seat.hostEdgeRemainderStartTs, seat.hostEdgeRemainderEndTs
	if remEnd <= remStart {
		return RootCauseRankItem{}, false
	}
	remMs := (remEnd - remStart) * 1000
	if remMs <= 0 {
		return RootCauseRankItem{}, false
	}
	rem := seat
	rem.ChainRelevance = "adjacent"
	rem.Causality = "adjacent_to_wakeup_chain"
	rem.OnChainBasis = ""
	rem.StartTs, rem.EndTs = remStart, remEnd
	rem.ActualStartTs, rem.ActualEndTs = remStart, remEnd
	// B829: this clone is the raw post-edge half of a host-edge semantic
	// account. The source edge proves relation only, not semantic completion
	// causality, so neither half may enter a priced causal election.
	rem.EffectiveImpactMs = 0
	rem.ProjectedImpactMs = remMs
	rem.CumulativeImpactMs = remMs
	rem.ActualImpactMs = remMs
	rem.ActualTotalMs = remMs
	rem.RankSortBoostedEffectiveMs = 0
	rem.ChainAnchorRemainderSeat = true
	// The bipartition pair mirrors the seat's own stamps (full = pre + post).
	rem.ChainAnchoredMs = seat.ChainAnchoredMs
	rem.ChainAnchorFullMs = seat.ChainAnchorFullMs
	rem.hostEdgeRemainderStartTs, rem.hostEdgeRemainderEndTs = 0, 0
	rem.Score = rootCauseRankScoreBasisMs(rem) * rem.Confidence * rootCauseItemScoreWeight(rem)
	rem.Summary = fmt.Sprintf("%s span %q post-edge raw-occupancy remainder %.3fms window=%.6f..%.6f — the share AFTER the host's latest in-window wakeup edge toward the analysis target (edge=relation credential; semantic completion/delay binding=unproven; anchored pre-edge raw share %.3fms and this remainder partition the span's in-window projection %.3fms exactly; effective_impact=0.000ms)",
		firstNonEmpty(rem.SemanticClass, rem.Type), rem.SpanName, remMs, remStart, remEnd, rem.ChainAnchoredMs, rem.ChainAnchorFullMs)
	return rem, true
}
