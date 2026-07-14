package tracequery

// rank_self_wall_clock_selfall.go — SELF-ALL: the analysis target's own
// wall-clock seats enter the on-chain channel (§29.61.2 user ruling
// 2026-07-13, extending SELF-SEM §29.61.1; effective-attribution ladder per
// §29.61.2a; ledger docs/design/real_trace_campaign_20260705.md).
//
// Ruling shape: 「◇ 内关注线程」不仅仅是显示问题 — the target's OWN wall-clock
// seat inside the query window participates in the on-chain root-cause
// ordering. The precise admission gate is ALL-typed:
//
//   - a chain universe exists (nodes/edges/causal impacts — no chain, no
//     on-chain identity to grant);
//   - chain.Target is resolved (PID or comm — absence never guesses);
//   - the row's subject thread IS the target (sameThreadRef, the engine's
//     tid-first identity comparator — the SubjectIsAnalysisTarget semantics at
//     the lane-decision point, which runs before the stamp pass);
//   - the row carries a typed wall-clock interval that lies inside the query
//     window (EndTs>StartTs ∧ overlap with chain.Window when bounded);
//   - the row's caliber is a WALL-CLOCK seat family: registry
//     Additivity==wall_clock_per_thread ∧ Subject!=aggregate_only ∧ the V2-P0
//     caliber-side class is None (composite scores / count equivalents keep
//     the ⌗ 口径旁栏 and never take this basis — 非墙钟口径照旧不参赛), and
//     the token belongs to the existing seat-family closed sets (the
//     effective-ladder running/runnable/D-IO caliber switches plus the IOFAM
//     union facets — reused, never a second hand list);
//   - the token is NOT a wait-on-counterpart symptom family (registry lanes
//     wakeup_chain / lock_contention — the SYM-2 closed set: those rows stay
//     on the symptom lane, their root cause lives at the counterpart);
//   - the token is NOT a deterministic semantic span class — those rows ride
//     the SELF-SEM basis minted at family-fold/mint time (§23.1
//     lane-decided-once; two bases never re-decide each other).
//
// Admitted rows flip adjacent→on_chain with
// OnChainBasis=RootCauseOnChainBasisSelfWallClockInterval and
// Causality=RootCauseCausalitySelfWallClock — no wakeup edge is minted, no
// cross-thread relation is claimed, and OverlapMs stays 0 (不伪造重叠).
// Effective attribution is deliberately NOT touched here: the promoted row
// consumes the SAME per-state ladder as every on-chain row
// (rootCauseEffectiveImpactMsUncapped: running=supply-fold deficit,
// runnable=RunnableMs in full, D/IO=DStateMs+IOWaitMs, IO facets=their own
// measured wall clock) — §29.61.2a 零特判 by construction.
//
// §23.1 道别红线 stays byte-identical for every NON-target thread: the third
// condition is exactly the red line's protected boundary (its witness is an
// other-process span — structurally different; ruling ⑤ 负向 pin 同款).

import "strings"

// rootCauseOnChainBasisIsSelf reports whether a basis token belongs to the
// self family (SELF-SEM + SELF-ALL) — the ONE membership read the enrich keep
// arm and the causality minter consult (closed set; "" = overlap basis).
func rootCauseOnChainBasisIsSelf(basis string) bool {
	switch strings.TrimSpace(basis) {
	case RootCauseOnChainBasisSelfDeterministicSpan, RootCauseOnChainBasisSelfWallClockInterval:
		return true
	default:
		return false
	}
}

// selfWallClockSeatTokenAdmits is the registry arm of the SELF-ALL gate: does
// this cause token denote a per-thread WALL-CLOCK seat family eligible for the
// self basis? Every read is an existing typed registry/ladder signal — no
// second hand-copied token list (§29.38 M-family discipline):
//   - CausalTokenSpecFor: Additivity / Subject / Lane columns;
//   - CausalTokenCaliberSideClass: the V2-P0 composite-score/count verdict;
//   - rootCauseTypeIsSemanticSpanWork: the SELF-SEM-owned closed set;
//   - the running/runnable/D-IO caliber switches + IOFAM union facets decide
//     seat-family membership on the item arm below.
func selfWallClockSeatTokenAdmits(token string) bool {
	token = strings.TrimSpace(token)
	spec, ok := CausalTokenSpecFor(token)
	if !ok {
		return false // absence never promotes
	}
	if spec.Additivity != CausalAdditivityWallClockPerThread || spec.Subject == CausalSubjectAggregateOnly {
		return false
	}
	if CausalTokenCaliberSideClass(token) != CausalCaliberSideNone {
		return false // ⌗ 口径旁栏: composite score / count equivalents 不参赛 (V2-P0)
	}
	switch spec.Lane {
	case CausalLaneWakeupChain, CausalLaneLockContention:
		// SYM-2 wait-symptom closed set (registry lane derivation): the
		// target's own wait-on-counterpart rows stay symptom-lane — their
		// root cause lives at the counterpart, not in this seat.
		return false
	}
	if rootCauseTypeIsSemanticSpanWork(token) {
		// SELF-SEM owns the deterministic semantic classes at mint time
		// (§23.1 lane-decided-once; the two self bases never re-decide each
		// other).
		return false
	}
	return true
}

// rootCauseItemIsSelfWallClockSeat is the item arm: the row must belong to one
// of the existing WALL-CLOCK seat families — the effective-ladder caliber
// switches (running / runnable / D-IO) or the IOFAM union facets (io_wait /
// io_latency / io_burst_episode). Generic wall-clock context tokens (e.g.
// trace_span) belong to none of these families and are structurally excluded
// (they are zero-effective context, not seats).
func rootCauseItemIsSelfWallClockSeat(item RootCauseRankItem) bool {
	if !selfWallClockSeatTokenAdmits(item.Type) {
		return false
	}
	if rootCauseItemIsRunningCaliber(item) || rootCauseItemIsRunnableCaliber(item) || rootCauseItemIsDStateOrIOCaliber(item) {
		return true
	}
	return CausalIOFacetFamilyToken(strings.TrimSpace(item.Type)) && adjacentIOFacetUnionFacet(strings.TrimSpace(item.Type))
}

// selfWallClockSeatTokenIsSeatFamily is the token-only shape of the item arm
// for faces that carry no rank item (critical_blocking / io_burst context
// enrich): the probe item engages exactly the token-driven switch arms of the
// caliber families (state-scalar default arms stay false on a bare token —
// per-instance state families on those faces all carry dedicated tokens).
func selfWallClockSeatTokenIsSeatFamily(token string) bool {
	return rootCauseItemIsSelfWallClockSeat(RootCauseRankItem{Type: strings.TrimSpace(token)})
}

// selfWallClockIntervalInWindow is the typed-interval half of the gate: a real
// wall-clock interval (EndTs>StartTs) that overlaps the chain's query window
// when the window is bounded. A zero/reversed interval never admits (真无区间
// rows keep their lane — the §29.58.3 display-reposition residual class).
func selfWallClockIntervalInWindow(chain *ChainResult, startTs, endTs float64) bool {
	if endTs <= startTs {
		return false
	}
	if chain.Window.EndTs > chain.Window.StartTs {
		if _, _, ok := overlapTimeWindow(startTs, endTs, chain.Window.StartTs, chain.Window.EndTs); !ok {
			return false
		}
	}
	return true
}

// selfWallClockSeatLane is THE shared SELF-ALL admission predicate (§29.61.2 —
// ONE implementation, three lane-decision consumers: the rank enrich pass, the
// critical_blocking context enrich and the io_burst context enrich). It judges
// the SELF-identity + interval half; callers supply their face's typed token
// admission (item arm or token arm) alongside.
func selfWallClockSeatLane(chain *ChainResult, thread ThreadRef, startTs, endTs float64) bool {
	if chain == nil {
		return false
	}
	if len(chain.Nodes) == 0 && len(chain.Edges) == 0 && len(chain.CausalImpacts) == 0 {
		return false
	}
	if chain.Target.PID <= 0 && strings.TrimSpace(chain.Target.Comm) == "" {
		return false
	}
	if !sameThreadRef(thread, chain.Target) {
		return false
	}
	return selfWallClockIntervalInWindow(chain, startTs, endTs)
}
