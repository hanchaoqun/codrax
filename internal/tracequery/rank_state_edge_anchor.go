package tracequery

// rank_state_edge_anchor.go — ONCHAIN-3c: the R3 host-edge credential arm's
// reach extends from semantic SPANS to the runnable / D-IO STATE seats of
// bare-census-edge hosts (mint audit docs/design/onchain_mint_audit_20260718.md
// 反向缺口5; evaluation docs/design/edge3be_eval_20260719.md §3c; R3 precedent
// §29.88.1/§29.88.2, ledger docs/design/real_trace_campaign_20260705.md).
//
// Shape: a NON-chain-member host thread holds a REAL typed wakeup edge toward
// the analysis target (the raw census pair host→target — the SCAN-3 61839
// sentinel: a bare edge that woke an already-runnable target, invisible to
// every chain-window intersection lane), yet only its deterministic semantic
// spans could ride the chain tier (R3 span basis). Its runnable / D-IO state
// seats had no door: RSPA re-anchoring only runs for pids with depth>0 chain
// anchor windows, and the generic enrich lane sees no node-window overlap —
// the whole state account rode ◇/background while its pre-edge share is
// direct supply-side causality (边前 runnable = the waker itself was
// CPU-starved and DELAYED the wakeup it provably delivered; 边前 D/IO = the
// waker was blocked on IO before delivering it).
//
// Mechanism (existing templates only, zero new credential machinery):
//   - credential  = hostSemanticSpanEdgeAnchor (R3 function, zero-change
//     reuse — typed census/chain-edge inventories, degenerate-window
//     fail-closed, target carve);
//   - bisection   = semanticEdgeAnchorSplit per TRUE segment (the seat's
//     validated close-site inventory: runnableIntervals /
//     dioSegmentIntervals* — never the StartTs..EndTs hull; HULL-CRED
//     discipline), pre-edge share ⛓ / post-edge share ◇;
//   - remainder   = the RSPA bipartition trio + ChainAnchorRemainderSeat
//     clone (side-lane plumbing, twin-visibility verbatim anchors, fold
//     anchor-form keys — all existing consumers);
//   - disclosure  = the R4 family sentence (边=凭证,边前=有效,边后=解除)
//     with the typed HostWakeupEdgeAnchorTs/Via pair; span/state co-billing
//     of one host is disclosed by the AXIOM-V2 件2 cross-direction mutual
//     pointers (runnable=scheduling_supply, D-IO=io_dependency, semantic
//     spans=self_workload — different directions, same thread, both support
//     inventories present ⇒ the existing pass fires with zero new code).
//
// Ownership boundary (one lane, one vocabulary owner): chain-member pids are
// structurally excluded — RSPA owns chain members' state-credential
// vocabulary (§29.61.10 处置矩阵, RNB-1 §29.88 R2/R4); re-anchoring a chain
// member's ◇ remainder through the R3 boundary would re-litigate the RNB-1
// customer escapes. The analysis target is excluded (self-causality, R8).
//
// Fail-closed forms (宁漏勿假指): no credential / degenerate window (R3
// gate), all segments post-edge (边后=解除 grants nothing, SCAN-3 negative
// sentinel semantics), missing or Σ-broken segment inventory, MAX-fallback
// folds (producer disjointness lost), inversion-rewritten or gated-algebra
// seats (indivisible composite — the RSPA R4 arm's reasoning), periodic-
// discounted rows. Every skipped row keeps its lane and bytes untouched.

import (
	"fmt"
	"sort"
	"strings"
)

// stateEdgeSegmentSplit is one inventory's exact bisection at the credential
// boundary: the two clipped segment lists partition the in-window inventory,
// and preMs + postMs reproduces the inventory Σ exactly (同源二分).
type stateEdgeSegmentSplit struct {
	preMs, postMs      float64
	pre, post          []foldInterval
	preStart, preEnd   float64
	postStart, postEnd float64
}

// splitFoldIntervalsAtBoundary bisects a validated segment inventory at the
// credential boundary via the R3 split primitive (semanticEdgeAnchorSplit).
func splitFoldIntervalsAtBoundary(intervals []foldInterval, boundary float64) stateEdgeSegmentSplit {
	var out stateEdgeSegmentSplit
	for _, iv := range intervals {
		preS, preE, postS, postE := semanticEdgeAnchorSplit(iv.start, iv.end, boundary)
		if preE > preS {
			ms := (preE - preS) * 1000
			out.preMs += ms
			out.pre = append(out.pre, foldInterval{start: preS, end: preE, valueMs: ms})
			if out.preStart == 0 || preS < out.preStart {
				out.preStart = preS
			}
			if preE > out.preEnd {
				out.preEnd = preE
			}
		}
		if postE > postS {
			ms := (postE - postS) * 1000
			out.postMs += ms
			out.post = append(out.post, foldInterval{start: postS, end: postE, valueMs: ms})
			if out.postStart == 0 || postS < out.postStart {
				out.postStart = postS
			}
			if postE > out.postEnd {
				out.postEnd = postE
			}
		}
	}
	return out
}

// foldIntervalsLengthMs sums a segment inventory's clipped lengths; ok=false
// on any degenerate segment (fail-closed — a broken inventory proves nothing).
func foldIntervalsLengthMs(intervals []foldInterval) (float64, bool) {
	total := 0.0
	for _, iv := range intervals {
		if iv.end <= iv.start {
			return 0, false
		}
		total += (iv.end - iv.start) * 1000
	}
	return total, true
}

// stateEdgeAnchorFamilyWord — the board family word (shared with the RSPA
// sentence family so one thread's accounts speak one vocabulary).
func stateEdgeAnchorFamilyWord(typ string) string {
	if typ == "runnable_wait" {
		return "runnable (scheduling-pressure candidate)"
	}
	return "D/IO blocking"
}

// stateEdgeAnchoredSummary — the ⛓ pre-edge seat's engine-side account for a
// boundary-crossing inventory. The twin-pointer claim reuses the RSPA
// verbatim anchor so rspaPatchSummariesForTwinVisibility downgrades it
// honestly when the ◇ clone dies at the cap.
// 备案 (o3c fixround): this family hangs the twin parenthetical directly
// after "post-edge remainder", while the RSPA sister sentence
// (rspaAnchoredSummary) interposes "outside the dependency windows"
// first — a known cross-family micro-divergence; each family is internally
// self-consistent and the shared verbatim anchor is what twin-visibility
// matches.
func stateEdgeAnchoredSummary(thread ThreadRef, family string, pre, full, post, boundary float64, via string) string {
	return fmt.Sprintf("%s %s pre-edge share %.3fms anchored by the host's own in-window typed wakeup edge toward the analysis target (edge=credential, pre-edge=effective, post-edge=released; latest credential edge %.6f, via=%s); full-window account %.3fms = this pre-edge share + %.3fms post-edge remainder %s — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, pre, boundary, via, full, post, rspaSummaryRemainderTwinPublished)
}

// stateEdgeFullyAnchoredSummary — the whole-account conversion form (every
// segment pre-edge; no bipartition, no twin claim).
func stateEdgeFullyAnchoredSummary(thread ThreadRef, family string, full, boundary float64, via string) string {
	return fmt.Sprintf("%s %s %.3fms fully pre-edge: every segment lies before the host's latest in-window typed wakeup edge toward the analysis target (edge=credential, pre-edge=effective, post-edge=released; latest credential edge %.6f, via=%s)",
		threadLabel(thread), family, full, boundary, via)
}

// stateEdgeRemainderSummary — the ◇ post-edge clone's account. The ownership
// claim reuses the RSPA verbatim anchor (the ⛓ pre-edge twin IS a clipped
// ChainAnchorFullMs seat, so the existing twin-visibility pass verifies it).
func stateEdgeRemainderSummary(thread ThreadRef, family string, post, full, pre, boundary float64, via string) string {
	return fmt.Sprintf("%s %s post-edge remainder %.3fms — the share AFTER the host's latest in-window typed wakeup edge toward the analysis target (edge=credential, pre-edge=effective, post-edge=released; latest credential edge %.6f, via=%s); full-window account %.3fms = %.3fms pre-edge share %s + this remainder — same segment set, mutually disjoint, additive back to the full account",
		threadLabel(thread), family, post, boundary, via, full, pre, rspaSummaryOwnedByChainSeat)
}

// bareCensusEdgeHostRunnableMintSet (ONCHAIN-3c mint-domain widening — 帽基
// 当全量 fifth instance) returns the pids of bare-census-edge hosts whose
// runnable census carries a POSITIVE pre-edge share: exactly the rows the
// state-seat credential arm will examine, so the widening never strands a
// background row it will not examine (admission ≠ conversion: an examined row
// may still be refused and hold its home lane — the tieba 23088 live form,
// minted then inversion-recast then R4-mirror refused). Gates (all typed):
// the ordered-stream premise (a regressed trace cannot prove the bisection Σ
// — same premise as the RSPA decision gate 4), the R3 credential function
// verbatim (single edge-judgment authority, 禁自造第二套边判定 — covers the
// degenerate-window fail-close and the target carve), host ∉ chain thread
// set (RSPA vocabulary ownership), and Σ over the pid's census segment
// inventories of the pre-boundary clip > tolerance.
func bareCensusEdgeHostRunnableMintSet(chain ChainResult, chainThreads map[int]bool,
	census map[string]ThreadDuration, producerDisjoint bool) map[int]bool {
	if !producerDisjoint || len(census) == 0 {
		return nil
	}
	if len(chain.WakeupEdgeCensus) == 0 {
		return nil
	}
	var out map[int]bool
	seen := map[int]bool{}
	for _, pair := range chain.WakeupEdgeCensus {
		host := pair.Waker
		if pair.Count <= 0 || host.PID <= 0 || seen[host.PID] {
			continue
		}
		seen[host.PID] = true
		if threadInSet(chainThreads, host) || sameThreadRef(host, chain.Target) {
			continue
		}
		anchor, ok := hostSemanticSpanEdgeAnchor(&chain, host)
		if !ok {
			continue
		}
		preMs := 0.0
		for _, td := range census {
			if td.Thread.PID != host.PID {
				continue
			}
			for _, iv := range td.runnableIntervals {
				preS, preE, _, _ := semanticEdgeAnchorSplit(iv.start, iv.end, anchor.boundaryTs)
				if preE > preS {
					preMs += (preE - preS) * 1000
				}
			}
		}
		if preMs <= rspaAnchorIdentityTolMs {
			continue
		}
		if out == nil {
			out = map[int]bool{}
		}
		out[host.PID] = true
	}
	return out
}

// bareCensusEdgeStateSeatEligible is the typed admission gate (see the file
// header's fail-closed roster). PRECISE signals only.
func bareCensusEdgeStateSeatEligible(item *RootCauseRankItem, chainThreads map[int]bool, target ThreadRef) bool {
	if item.Thread.PID <= 0 || sameThreadRef(item.Thread, target) || item.SubjectIsAnalysisTarget {
		return false
	}
	if threadInSet(chainThreads, item.Thread) {
		return false // RSPA owns chain members' state-credential vocabulary
	}
	if rootCauseItemIsOnChain(*item) {
		return false // already on the chain tier — nothing to extend
	}
	if strings.TrimSpace(item.OnChainBasis) != "" || item.ChainAnchorRemainderSeat ||
		item.ChainAnchorFullMs > 0 || item.AbsorbedByRankFamily ||
		item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat ||
		item.GatedShareConstituentSeat || item.ChainIdentityInheritance {
		return false // already adjudicated / processed forms
	}
	if item.PeriodicSource {
		return false // cadence-discounted account — indivisible here
	}
	return true
}

// anchorBareCensusEdgeStateSeats is the ONCHAIN-3c pass. Runs right after
// reanchorOnChainStateSeats in BOTH rank passes (build + scheduler enrich) —
// idempotent: converted rows carry the state basis token / the remainder
// marker and are never re-processed (the enrich pass mints no new formal
// window state seats, so the second call is structurally a no-op). Returns
// the item slice (bisections append the ◇ remainder twin).
func anchorBareCensusEdgeStateSeats(chain ChainResult, items []RootCauseRankItem) []RootCauseRankItem {
	if len(items) == 0 {
		return items
	}
	if len(chain.Nodes) == 0 && len(chain.Edges) == 0 && len(chain.CausalImpacts) == 0 {
		return items
	}
	chainThreads := wakeupChainThreadSet(chain)
	anchorByPID := map[int]*semanticHostEdgeAnchor{}
	anchorFor := func(host ThreadRef) (semanticHostEdgeAnchor, bool) {
		if cached, ok := anchorByPID[host.PID]; ok {
			if cached == nil {
				return semanticHostEdgeAnchor{}, false
			}
			return *cached, true
		}
		anchor, ok := hostSemanticSpanEdgeAnchor(&chain, host)
		if !ok {
			anchorByPID[host.PID] = nil
			return semanticHostEdgeAnchor{}, false
		}
		anchorByPID[host.PID] = &anchor
		return anchor, true
	}
	var appended []RootCauseRankItem
	for i := range items {
		item := &items[i]
		if !bareCensusEdgeStateSeatEligible(item, chainThreads, chain.Target) {
			continue
		}
		switch strings.TrimSpace(item.Type) {
		case "priority_inversion_runnable_wait":
			// RSPA R4 mirror arm (§29.88 R4; §29.83 件③ narrowing): the
			// inversion-rewritten seat's gated eff is an INDIVISIBLE composite
			// that cannot be split along the credential boundary without
			// minting a value equal to neither the measurement nor any
			// partition term. A FULLY pre-edge account changes lane whole with
			// every published value untouched (the inversion claim is pre-edge
			// too — the R4 「fully-anchored keeps the chain lane」 form in the
			// promotion direction); any post-edge share leaves the whole seat
			// on its home lane untouched (宁漏勿假指).
			if item.Source != "window_stats" || item.DominantState != string(StateRunnable) {
				continue
			}
			if len(item.runnableIntervals) == 0 || !item.memberSegmentsProducerDisjoint {
				continue
			}
			lengthMs, ok := foldIntervalsLengthMs(item.runnableIntervals)
			if !ok || !rspaWithinTol(lengthMs, item.RunnableMs) {
				continue
			}
			anchor, ok := anchorFor(item.Thread)
			if !ok {
				continue
			}
			split := splitFoldIntervalsAtBoundary(item.runnableIntervals, anchor.boundaryTs)
			if split.preMs <= rspaAnchorIdentityTolMs {
				continue // 边后=解除 grants nothing (negative sentinel semantics)
			}
			if split.postMs > rspaAnchorIdentityTolMs {
				// PARTSPLIT-1 (§29.150④, 2026-07-19) — EVOLUTION of the bare
				// refusal: the R4-mirror gate still refuses the conversion
				// (whole-seat floor — value/lane/ordinal byte-identical), but
				// the refusal now goes ON RECORD with its disclosure-only
				// bisection measures (X pre-edge + Y post-edge, X+Y == the
				// runnable census account to the µs). Stamped atomically at
				// this single site; deterministic re-stamp on the second pass
				// (idempotent — same chain, same inventory, same boundary).
				item.GatedCompositeEdgePreShareMs = split.preMs
				item.GatedCompositeEdgePostShareMs = split.postMs
				item.GatedCompositeEdgeAnchorTs = anchor.boundaryTs
				item.GatedCompositeEdgeAnchorVia = anchor.via()
				continue
			}
			item.ChainRelevance = "on_chain"
			item.Causality = "on_wakeup_chain"
			item.OnChainBasis = RootCauseOnChainBasisHostWakeupEdgeState
			item.HostWakeupEdgeAnchorTs = anchor.boundaryTs
			item.HostWakeupEdgeAnchorVia = anchor.via()
			item.Summary = appendRootCauseSummaryDetail(item.Summary,
				fmt.Sprintf("edge-anchored (host→target): the whole runnable account lies before the host's latest in-window typed wakeup edge toward the analysis target at %.6f (edge=credential, pre-edge=effective, post-edge=released; via=%s) — every published value unchanged, the gated composite is never split",
					anchor.boundaryTs, anchor.via()))
		case "runnable_wait":
			if item.Source != "window_stats" || item.DominantState != string(StateRunnable) {
				continue
			}
			if item.GatedRunnableMs > 0 || item.GatedRunningDeficitMs > 0 {
				continue // gated composite algebra — indivisible (RSPA R4 reasoning)
			}
			if len(item.runnableIntervals) == 0 || !item.memberSegmentsProducerDisjoint {
				continue
			}
			lengthMs, ok := foldIntervalsLengthMs(item.runnableIntervals)
			if !ok || !rspaWithinTol(lengthMs, item.RunnableMs) {
				continue // inventory does not reproduce the account — fail closed
			}
			anchor, ok := anchorFor(item.Thread)
			if !ok {
				continue
			}
			split := splitFoldIntervalsAtBoundary(item.runnableIntervals, anchor.boundaryTs)
			if split.preMs <= rspaAnchorIdentityTolMs {
				continue // 边后=解除 grants nothing (negative sentinel semantics)
			}
			full := split.preMs + split.postMs
			family := stateEdgeAnchorFamilyWord(item.Type)
			if split.postMs <= rspaAnchorIdentityTolMs {
				// Whole-account conversion: every segment pre-edge — the seat
				// changes lane with its published account intact (值通道重铸为
				// 全账,无二分 trio,无余席 — the span basis' fully-pre form).
				convertStateSeatToEdgeAnchored(item, anchor, item.RunnableMs, item.RunnableMs, 0, 0,
					item.runnableIntervals, nil, nil, split,
					stateEdgeFullyAnchoredSummary(item.Thread, family, item.RunnableMs, anchor.boundaryTs, anchor.via()))
				continue
			}
			clone := *item
			convertStateSeatToEdgeAnchored(item, anchor, split.preMs, split.preMs, 0, 0,
				split.pre, nil, nil, split,
				stateEdgeAnchoredSummary(item.Thread, family, split.preMs, full, split.postMs, anchor.boundaryTs, anchor.via()))
			item.ChainAnchoredMs = split.preMs
			item.ChainAnchorFullMs = full
			appended = append(appended, mintStateEdgeRemainderClone(clone, anchor, split.preMs, full,
				split.postMs, 0, 0, split.post, nil, nil, split,
				stateEdgeRemainderSummary(item.Thread, family, split.postMs, full, split.preMs, anchor.boundaryTs, anchor.via())))
		case "d_state_or_io_wait", "io_wait":
			if item.Source != "window_stats" && item.Source != "window_stats.io_wait_top" {
				continue
			}
			switch item.DominantState {
			case string(StateDSleep), string(StateIOWait):
			default:
				continue
			}
			if len(item.dioSegmentIntervals) == 0 {
				continue // no true-segment carrier (缺证≠证无 — fail closed)
			}
			dLen, okD := foldIntervalsLengthMs(item.dioSegmentIntervalsD)
			ioLen, okIO := foldIntervalsLengthMs(item.dioSegmentIntervalsIO)
			if !okD || !okIO || !rspaWithinTol(dLen, item.DStateMs) || !rspaWithinTol(ioLen, item.IOWaitMs) {
				continue // per-state inventories must reproduce both channels
			}
			anchor, ok := anchorFor(item.Thread)
			if !ok {
				continue
			}
			splitD := splitFoldIntervalsAtBoundary(item.dioSegmentIntervalsD, anchor.boundaryTs)
			splitIO := splitFoldIntervalsAtBoundary(item.dioSegmentIntervalsIO, anchor.boundaryTs)
			splitAll := splitFoldIntervalsAtBoundary(item.dioSegmentIntervals, anchor.boundaryTs)
			preMs := splitD.preMs + splitIO.preMs
			postMs := splitD.postMs + splitIO.postMs
			if preMs <= rspaAnchorIdentityTolMs {
				continue
			}
			full := preMs + postMs
			family := stateEdgeAnchorFamilyWord(item.Type)
			if postMs <= rspaAnchorIdentityTolMs {
				convertStateSeatToEdgeAnchored(item, anchor, item.DStateMs+item.IOWaitMs, 0, item.DStateMs, item.IOWaitMs,
					nil, item.dioSegmentIntervalsD, item.dioSegmentIntervalsIO, splitAll,
					stateEdgeFullyAnchoredSummary(item.Thread, family, item.DStateMs+item.IOWaitMs, anchor.boundaryTs, anchor.via()))
				continue
			}
			clone := *item
			convertStateSeatToEdgeAnchored(item, anchor, preMs, 0, splitD.preMs, splitIO.preMs,
				nil, splitD.pre, splitIO.pre, splitAll,
				stateEdgeAnchoredSummary(item.Thread, family, preMs, full, postMs, anchor.boundaryTs, anchor.via()))
			item.ChainAnchoredMs = preMs
			item.ChainAnchorFullMs = full
			appended = append(appended, mintStateEdgeRemainderClone(clone, anchor, preMs, full,
				postMs, splitD.postMs, splitIO.postMs, nil, splitD.post, splitIO.post, splitAll,
				stateEdgeRemainderSummary(item.Thread, family, postMs, full, preMs, anchor.boundaryTs, anchor.via())))
		}
	}
	return append(items, appended...)
}

// convertStateSeatToEdgeAnchored rewrites one bare-census state seat into the
// ⛓ pre-edge seat: lane/causality/basis/typed edge pair, value channels =
// the pre-edge account (per-state for D/IO — exact, never apportioned), the
// seat's segment inventories replaced by their pre-edge clips (so the
// AXIOM-V2 direction support union reproduces the published value — the
// support==claim identity holds on both halves by construction), Score
// re-derived from the published value (§7.30 S1 discipline). The per-(thread,
// cpu) group hulls (familyMemberIntervals) are CLEARED on both halves: they
// describe the pre-split full account and would let a cross-type exact recon
// match a half seat against a full-account interval identity (偏离备案:
// halves carry their own precise clipped inventories instead).
func convertStateSeatToEdgeAnchored(item *RootCauseRankItem, anchor semanticHostEdgeAnchor,
	totalMs, runnableMs, dMs, ioMs float64,
	runnableSegs, dSegs, ioSegs []foldInterval, split stateEdgeSegmentSplit, summary string) {
	item.ChainRelevance = "on_chain"
	item.Causality = "on_wakeup_chain"
	item.OnChainBasis = RootCauseOnChainBasisHostWakeupEdgeState
	item.HostWakeupEdgeAnchorTs = anchor.boundaryTs
	item.HostWakeupEdgeAnchorVia = anchor.via()
	item.RunnableMs = runnableMs
	item.DStateMs = dMs
	item.IOWaitMs = ioMs
	item.ImpactMs = totalMs
	item.ProjectedImpactMs = totalMs
	item.CumulativeImpactMs = totalMs
	item.EffectiveImpactMs = totalMs
	item.Score = totalMs * item.Confidence * rootCauseItemScoreWeight(*item)
	item.runnableIntervals = runnableSegs
	item.dioSegmentIntervalsD = dSegs
	item.dioSegmentIntervalsIO = ioSegs
	item.dioSegmentIntervals = nil
	if len(dSegs) > 0 || len(ioSegs) > 0 {
		item.dioSegmentIntervals = append(append([]foldInterval(nil), dSegs...), ioSegs...)
	}
	item.familyMemberIntervals = nil
	if split.preEnd > split.preStart {
		item.StartTs, item.EndTs = split.preStart, split.preEnd
	}
	item.Summary = summary
}

// mintStateEdgeRemainderClone mints the ◇ post-edge twin from the pre-rewrite
// seat copy: the RSPA remainder trio + marker (side-lane, fold anchor-form
// and twin-visibility plumbing all existing), values = the post-edge account,
// segment inventories = the post-edge clips, typed edge pair kept as the
// disclosure of WHICH boundary split this account (the span remainder clone
// precedent keeps the pair too).
func mintStateEdgeRemainderClone(clone RootCauseRankItem, anchor semanticHostEdgeAnchor,
	preMs, fullMs, postMs, dMs, ioMs float64,
	runnableSegs, dSegs, ioSegs []foldInterval, split stateEdgeSegmentSplit, summary string) RootCauseRankItem {
	clone.Rank = 0
	clone.BackgroundRank = 0
	clone.Tier = ""
	clone.ChainRelevance = "adjacent"
	clone.Causality = "adjacent_to_wakeup_chain"
	clone.OnChainBasis = ""
	clone.ChainAnchorRemainderSeat = true
	clone.ChainAnchoredMs = preMs
	clone.ChainAnchorFullMs = fullMs
	clone.HostWakeupEdgeAnchorTs = anchor.boundaryTs
	clone.HostWakeupEdgeAnchorVia = anchor.via()
	var runnableMs float64
	if len(runnableSegs) > 0 {
		runnableMs = postMs
	}
	clone.RunnableMs = runnableMs
	clone.DStateMs = dMs
	clone.IOWaitMs = ioMs
	clone.ImpactMs = postMs
	clone.ProjectedImpactMs = postMs
	clone.CumulativeImpactMs = postMs
	clone.EffectiveImpactMs = postMs
	clone.Score = postMs * clone.Confidence * rootCauseItemScoreWeight(clone)
	clone.runnableIntervals = runnableSegs
	clone.dioSegmentIntervalsD = dSegs
	clone.dioSegmentIntervalsIO = ioSegs
	clone.dioSegmentIntervals = nil
	if len(dSegs) > 0 || len(ioSegs) > 0 {
		clone.dioSegmentIntervals = append(append([]foldInterval(nil), dSegs...), ioSegs...)
	}
	clone.familyMemberIntervals = nil
	if split.postEnd > split.postStart {
		clone.StartTs, clone.EndTs = split.postStart, split.postEnd
	}
	clone.Summary = summary
	return clone
}

// GatedCompositeEdgeShareDisclosure (PARTSPLIT-1, §29.150④ user ruling
// 2026-07-19) is one R4-mirror-refused gated composite seat's NON-SEAT
// pre-edge-share disclosure record — the result-level side channel the ◎
// overview mentions it through (no ordinal, never in a section maximum,
// never additive to the seat's published value). PreMs + PostMs == AccountMs
// (the seat's runnable census account) to the µs by construction.
type GatedCompositeEdgeShareDisclosure struct {
	Thread ThreadRef `json:"thread"`
	// PreMs / PostMs — the X/Y bisection measures (disclosure only).
	PreMs  float64 `json:"pre_ms"`
	PostMs float64 `json:"post_ms"`
	// AccountMs — the seat's runnable census account the identity holds
	// against (NOT the gated eff: the composite eff is exactly what the R4
	// floor refuses to split).
	AccountMs float64 `json:"account_ms"`
	// BoundaryTs / Via — WHICH credential edge bisected the account (the R3
	// closed-set via vocabulary).
	BoundaryTs float64 `json:"boundary_ts"`
	Via        string  `json:"via"`
	// SeatPublished — the refused seat itself survived the publication cap
	// (typed honesty input for the ◎ mention's off-board clause; the tieba
	// 23088 live form is false: the seat lives only in the pool).
	SeatPublished bool `json:"seat_published"`
	// LineStart / LineEnd — the refused seat's own trace-line evidence range
	// (verbatim from the stamped item; the wire record's grounding span).
	LineStart int `json:"line_start,omitempty"`
	LineEnd   int `json:"line_end,omitempty"`
}

// harvestGatedCompositeEdgeShareDisclosures builds the result-level side
// channel from the pre-truncation pool ∪ the published board (the pool is the
// build+enrich union and may hold one seat twice — first stamped occurrence
// wins per (pid, boundary)). Admission is the typed stamp itself (all four
// fields minted atomically at the single R4-mirror refusal site); absence
// harvests nothing. Runs at BOTH rank-pass tails (idempotent overwrite: the
// enrich harvest re-reads the union, so publishedness reflects the final
// board).
//
// POOL2-1 件③ (§29.160③ user ruling 2026-07-20 「复用 SPANVIS 双分量地板…+
// 行序改值降序;微真值降入 typed 记号(审计保留)」): a row whose PRE-EDGE
// share sits below the SPANVIS two-component significance floor —
// max(BusinessSpanMentionDustFloorMs, BusinessSpanMentionWindowShareFloor ×
// window) — is NOT issued on the disclosure channel (noise discipline; the
// share components reuse the SPANVIS constants verbatim, zero second table).
// 宁降不删: the refused seat's typed four-field stamp (GatedCompositeEdge*)
// stays on the item and serializes with the engine result — the audit record
// survives, only the reader-facing row is withheld; a windowless query keeps
// the dust component alone (demote, never delete). Rows are ordered by
// pre-edge share DESC (ties: line start asc, then pid asc — a deterministic
// READING order, not an ordinal).
func harvestGatedCompositeEdgeShareDisclosures(q Query, pool, published []RootCauseRankItem) []GatedCompositeEdgeShareDisclosure {
	floorMs := BusinessSpanMentionDustFloorMs
	if windowMs := (q.TimeEnd - q.TimeStart) * 1000; windowMs > 0 {
		if share := windowMs * BusinessSpanMentionWindowShareFloor; share > floorMs {
			floorMs = share
		}
	}
	type key struct {
		pid      int
		boundary float64
	}
	publishedSet := map[key]bool{}
	for i := range published {
		item := &published[i]
		if item.GatedCompositeEdgePreShareMs > 0 && item.GatedCompositeEdgePostShareMs > 0 {
			publishedSet[key{item.Thread.PID, item.GatedCompositeEdgeAnchorTs}] = true
		}
	}
	var out []GatedCompositeEdgeShareDisclosure
	seen := map[key]bool{}
	for _, items := range [][]RootCauseRankItem{pool, published} {
		for i := range items {
			item := &items[i]
			if item.GatedCompositeEdgePreShareMs <= 0 || item.GatedCompositeEdgePostShareMs <= 0 ||
				item.GatedCompositeEdgeAnchorTs <= 0 {
				continue
			}
			if item.GatedCompositeEdgePreShareMs < floorMs {
				// 件③ floor: micro pre-edge shares stay on the typed stamp
				// (audit) and never mint a disclosure row.
				continue
			}
			k := key{item.Thread.PID, item.GatedCompositeEdgeAnchorTs}
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, GatedCompositeEdgeShareDisclosure{
				Thread:        item.Thread,
				PreMs:         item.GatedCompositeEdgePreShareMs,
				PostMs:        item.GatedCompositeEdgePostShareMs,
				AccountMs:     item.RunnableMs,
				BoundaryTs:    item.GatedCompositeEdgeAnchorTs,
				Via:           item.GatedCompositeEdgeAnchorVia,
				SeatPublished: publishedSet[k],
				LineStart:     item.LineStart,
				LineEnd:       item.LineEnd,
			})
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].PreMs != out[b].PreMs {
			return out[a].PreMs > out[b].PreMs
		}
		if out[a].LineStart != out[b].LineStart {
			return out[a].LineStart < out[b].LineStart
		}
		return out[a].Thread.PID < out[b].Thread.PID
	})
	return out
}
