package tracequery

// rank_levelmerge_split.go — LEVELMERGE-1 件2 (方案 P 区间分账, user ruling
// 2026-07-18 「按推荐的来」, ledger real_trace_campaign_20260705.md §29.104
// residual channel; scout report levelkey_verify §2.4).
//
// Disease (the runnable2 E26+E28 self-consistent mechanism): a (pid, running)
// priority-inversion chain seat counts the dependency thread's RUNNABLE time
// inside its branch window AT FULL VALUE through the R5d gated composite
// (GatedRunnableMs), while the SAME pid's (pid, runnable) chain aggregate seat
// counts the same runnable seconds at full value under its own group key. The
// two group keys never meet (ORD-A / twin fold / cross-type recon / XLANE-1
// are all out of range — different dominant states, different lanes), so when
// the branch windows physically overlap (the PTV6 envelope-overlap shape:
// reconcileWakeupAggregateOccurrenceOverlap reconciles only INSIDE one group,
// never across groups) one physical runnable second is published at full value
// twice and the same-thread Σ exceeds the physical wall clock (customer
// witness: E26 17.635 + E28 8.608 = 26.243 > 23.471 physical runnable).
//
// Fix (Plan P, all three faces user-approved 2026-07-18):
//  1. the inversion seat (the REFINED seat) keeps its full gated composite —
//     正席序 (XLANE-1): inversion candidate outranks the chain aggregate;
//  2. the (pid, runnable) aggregate seat splits its account in two rows:
//     A = the share already attributed through the inversion seat(s) — a
//     demoted CONSTITUENT row on the adjacent lane (never competes, points at
//     the inversion seat), and B = the residual share, which keeps the
//     aggregate's competing seat;
//  3. A = |∪(claiming inversion seats' branch windows) ∩ ∪(aggregate
//     occurrence windows)| — a pure interval measure over merged unions
//     (multiple claimants union FIRST, subtract ONCE — never per-claimant),
//     clamped to the aggregate's published account (a window measure may
//     exceed the runnable time inside it);
//  4. identity: A + B == the pre-split aggregate value (GATED-CAL three-way
//     identity precedent) — pinned;
//  5. fail-open (§29.104.13 非致命不硬拦): a missing / ambiguous typed
//     interval carrier on EITHER side degrades to the ruling-④ disclosure
//     clause (「其中 X ms 与[E#]重叠」 form: overlap measured over the real
//     segments that ARE available) with the published value untouched; no
//     real segments at all → byte-identical (absence never guesses).
//
// 「链上」hard discipline (user ruling 2026-07-18):
//  - claim eligibility reads typed signals only: the inversion seat must
//    actually hold a competing seat (not absorbed / demoted / zero-seat),
//    publish a positive gated value AND a positive GatedRunnableMs (a pure
//    running-deficit composite claims no runnable seconds), and present a
//    REAL SEGMENT inventory (HULL-CRED §29.126 per-segment credential:
//    family member intervals / typed occurrence windows / true singleton
//    StartTs..EndTs — an envelope NEVER claims, hull endpoints are noise);
//  - the B row's chain credential is re-established from ITS OWN residual
//    segments (∪occ − ∪claim), never inherited from the pre-split aggregate;
//    if no residual segment survives, B honestly demotes to the adjacent
//    lane (◇ + disclosure);
//  - the A row is a demoted constituent row: adjacent lane, no on-chain
//    full-seat markers (链上面与降道面不得同行共存, E11 rider §29.125);
//  - Self* rows are exempt on both sides (XLANE-1 既裁: self-basis rows never
//    enter the population).
//
// Cross-board guard: both sides of the split are chain-lane rows of the SAME
// ChainResult inside one rank build, so the board identity triple is shared
// by construction; no cross-board pair can reach this pass (XLANE-3 让位红线
// structurally satisfied — noted, not gated).

import (
	"fmt"
	"strings"
)

// levelMergeGatedShareTolMs mirrors the RSPA µs identity tolerance — one
// tolerance family for every interval-accounting identity in the rank lanes.
const levelMergeGatedShareTolMs = rspaAnchorIdentityTolMs

// levelMergeSummaryClaimSeatOnBoard / levelMergeSummaryClaimSeatUnpublished
// are the verbatim co-publication anchors of the split sentences —
// patchGatedShareSummariesForClaimVisibility rewrites the claim when the
// named inversion seat did not survive truncation (RNB-1 D1 修复轮 discipline:
// a board sentence must never point at a row nobody can see). Idempotent by
// verbatim-anchor construction.
const levelMergeSummaryClaimSeatOnBoard = "(the claiming inversion seat is published on this board)"
const levelMergeSummaryClaimSeatUnpublished = "(the claiming inversion seat is not on the published board; see the compaction disclosure)"

// levelMergeSegmentWindows resolves a rank row's REAL segment inventory for
// the interval-accounting split. Inventory ladder (precise typed segments
// only — the XLANE-1 修复轮 P1-1 lesson: hull/envelope timestamps are NOISE
// and must never feed a hard gate):
//  1. familyMemberIntervals — the fold pass's exact member segments;
//  2. OccurrenceWindows — the aggregate lane's typed per-occurrence windows;
//     complete ONLY while strictly below the engine trim cap (an exactly-full
//     inventory may have dropped members — the semanticSpanCapLowerBound
//     exact-cap precedent, so it degrades to the disclosure arm);
//  3. the row's own [StartTs, EndTs] ONLY on a true singleton seat.
//
// complete=false with a non-empty windows slice = real segments exist but the
// inventory may be partial (disclosure-arm grade: the segments may witness an
// overlap, never bound a value split). Empty windows = no real segment at all.
func levelMergeSegmentWindows(item RootCauseRankItem) (windows []TimeWindow, complete bool) {
	if len(item.familyMemberIntervals) > 0 {
		for _, iv := range item.familyMemberIntervals {
			if iv.end > iv.start {
				windows = append(windows, TimeWindow{StartTs: iv.start, EndTs: iv.end})
			}
		}
		return windows, len(windows) > 0
	}
	if len(item.OccurrenceWindows) > 0 {
		for _, occ := range item.OccurrenceWindows {
			if occ.Window.EndTs > occ.Window.StartTs {
				windows = append(windows, occ.Window)
			}
		}
		return windows, len(windows) > 0 && len(item.OccurrenceWindows) < wakeupCausalAggregateOccurrenceCap
	}
	if item.MemberCount <= 1 && item.EndTs > item.StartTs {
		// True singleton: the pair IS the one occurrence segment, never an
		// envelope.
		return []TimeWindow{{StartTs: item.StartTs, EndTs: item.EndTs}}, true
	}
	return nil, false
}

// levelMergeRowHoldsCompetingSeat — the 「实际持席」 half of the claim
// eligibility: the row is in the competing pool right now (not absorbed into
// a family, not on any ◇/side lane, not a zero-seat disclosure row).
func levelMergeRowHoldsCompetingSeat(item RootCauseRankItem) bool {
	if item.AbsorbedByRankFamily || item.ChainAnchorRemainderSeat ||
		item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat ||
		item.GatedShareConstituentSeat {
		return false
	}
	return !rootCauseRankItemIsZeroSeatDisclosure(item)
}

// levelMergeClaimant is one same-pid inversion seat eligible to claim (or, at
// the partial grade, to witness) runnable-share overlap.
type levelMergeClaimant struct {
	windows  []TimeWindow
	complete bool
	lineSpan string
}

// levelMergeClaimantsByPID collects the priority-inversion chain seats whose
// gated composite claims runnable seconds. Eligibility is typed end to end
// (see the file header); a seat whose only inventory is an envelope
// contributes NOTHING (not even to the disclosure measure).
func levelMergeClaimantsByPID(target ThreadRef, items []RootCauseRankItem) map[int][]levelMergeClaimant {
	var out map[int][]levelMergeClaimant
	for i := range items {
		item := &items[i]
		if item.Thread.PID <= 0 || item.Type != "priority_inversion_candidate" {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(item.Source), "wakeup_chain") {
			continue
		}
		if !rootCauseItemIsOnChain(*item) || rspaRowIsSelfExempt(*item, target) {
			continue
		}
		// The claim is the RUNNABLE component of the gated composite — a pure
		// running-deficit composite overlaps no runnable aggregate account.
		// EffectiveImpactMs is the published gated value (authoritative
		// including zero: a gated-to-zero inversion row holds no seat and
		// claims nothing).
		if item.GatedRunnableMs <= 0 || item.EffectiveImpactMs <= 0 {
			continue
		}
		if !levelMergeRowHoldsCompetingSeat(*item) {
			continue
		}
		windows, complete := levelMergeSegmentWindows(*item)
		if len(windows) == 0 {
			continue
		}
		if out == nil {
			out = map[int][]levelMergeClaimant{}
		}
		out[item.Thread.PID] = append(out[item.Thread.PID], levelMergeClaimant{
			windows:  windows,
			complete: complete,
			lineSpan: fmt.Sprintf("%d..%d", item.LineStart, item.LineEnd),
		})
	}
	return out
}

// levelMergeWindowUnionOverlapMs computes |∪a ∩ ∪b| for two MERGED window
// unions: iterate one union's disjoint intervals, sum their overlap with the
// other union. Both inputs must already be merged (mergeAnchorTimeWindows) so
// the sum is the exact union-intersection measure.
func levelMergeWindowUnionOverlapMs(a, b []TimeWindow) float64 {
	total := 0.0
	for _, w := range b {
		total += anchorWindowsOverlapMs(a, w.StartTs, w.EndTs)
	}
	return total
}

// levelMergeResidualSegments subtracts the merged claim union from the merged
// occurrence union and returns the residual segments longer than the µs
// tolerance — the B row's OWN credential inventory (链上纪律③: the residual
// seat re-earns its chain lane from these, never by inheritance).
func levelMergeResidualSegments(occ, claim []TimeWindow) []TimeWindow {
	var out []TimeWindow
	for _, w := range occ {
		cursor := w.StartTs
		for _, c := range claim {
			if c.EndTs <= cursor || c.StartTs >= w.EndTs {
				continue
			}
			if c.StartTs > cursor {
				out = append(out, TimeWindow{StartTs: cursor, EndTs: c.StartTs})
			}
			if c.EndTs > cursor {
				cursor = c.EndTs
			}
		}
		if cursor < w.EndTs {
			out = append(out, TimeWindow{StartTs: cursor, EndTs: w.EndTs})
		}
	}
	kept := out[:0]
	for _, seg := range out {
		if (seg.EndTs-seg.StartTs)*1000 > levelMergeGatedShareTolMs {
			kept = append(kept, seg)
		}
	}
	return kept
}

func levelMergeClaimSeatSpans(claimants []levelMergeClaimant) []string {
	spans := make([]string, 0, len(claimants))
	for _, claimant := range claimants {
		spans = append(spans, claimant.lineSpan)
	}
	return spans
}

// splitAggregateGatedRunnableShare is the Plan-P pass. It runs in BOTH rank
// pipelines right after reanchorOnChainStateSeats (build + enrich) and is
// idempotent: already-split rows carry GatedShareFullMs / the constituent
// marker and pass through untouched.
func splitAggregateGatedRunnableShare(chain ChainResult, items []RootCauseRankItem) []RootCauseRankItem {
	if len(items) == 0 {
		return items
	}
	claimantsByPID := levelMergeClaimantsByPID(chain.Target, items)
	if len(claimantsByPID) == 0 {
		return items
	}
	var appended []RootCauseRankItem
	for i := range items {
		item := &items[i]
		if item.GatedShareFullMs > 0 || item.GatedShareConstituentSeat || item.GatedShareOverlapDisclosureMs > 0 {
			continue // idempotency: this seat already took its split/disclosure
		}
		if item.Thread.PID <= 0 || item.Source != "wakeup_chain.aggregated_impacts" {
			continue
		}
		// The claimed-against seat is the NON-inversion runnable aggregate
		// (the inversion-typed aggregate IS a refined seat and keeps its full
		// gated composite — Plan-P face 1).
		if item.Type != "runnable_wait" || item.DominantState != string(StateRunnable) {
			continue
		}
		if strings.TrimSpace(item.MemberFoldCaliber) != "" {
			// 修补轮 belt (件1 顺带, 2026-07-18; argued unreachable): a
			// family-fold survivor's RunnableMs is a CALIBER-computed account
			// (sum-disjoint / interval-union / max-overlap / count-sum —
			// RootCauseMemberFoldCaliber*), not the additive per-occurrence
			// account this split re-partitions. Fold survivors never carry
			// Source=="wakeup_chain.aggregated_impacts" in the production
			// mint chain (the fold pass seeds off state/window seats), so
			// this arm is pure defense in depth: byte-identical skip, no
			// account to re-carve.
			continue
		}
		if !rootCauseItemIsOnChain(*item) || rspaRowIsSelfExempt(*item, chain.Target) {
			continue
		}
		if !levelMergeRowHoldsCompetingSeat(*item) {
			continue
		}
		claimants := claimantsByPID[item.Thread.PID]
		if len(claimants) == 0 {
			continue
		}
		full := item.RunnableMs
		if full <= 0 {
			continue
		}
		occWindows, occComplete := levelMergeSegmentWindows(*item)
		if len(occWindows) == 0 {
			// No real segment inventory at all on the aggregate side — nothing
			// provable in either direction. Byte-identical (absence never
			// guesses; 禁静默 means values must never silently MOVE, not that
			// an unprovable overlap must be asserted).
			continue
		}
		claimComplete := true
		var claimWindows []TimeWindow
		for _, claimant := range claimants {
			claimComplete = claimComplete && claimant.complete
			claimWindows = append(claimWindows, claimant.windows...)
		}
		claimUnion := mergeAnchorTimeWindows(claimWindows)
		occUnion := mergeAnchorTimeWindows(occWindows)
		overlap := levelMergeWindowUnionOverlapMs(claimUnion, occUnion)
		if overlap <= levelMergeGatedShareTolMs {
			// No physical branch-window overlap — the two seats account for
			// disjoint wall clock and both keep their full publications.
			continue
		}
		spans := levelMergeClaimSeatSpans(claimants)
		if item.PeriodicSource {
			// 修补轮 件1 (P1, 2026-07-18): a VS-1 periodic-source seat's
			// published attribution is the DISCOUNTED composite runnable (in
			// full) + lateness (EffectivePeriodicImpactMs, §7.8) — NOT the
			// pure runnable account this split re-partitions. Splitting it
			// would erase the lateness share from the published authority
			// (probe: pre 19 = 15 runnable + 4 lateness → A+B == 15 ≠ 19)
			// and both halves would carry PeriodicSource=true into every
			// periodic-Σ consumer. Honest form = the ruling-④ disclosure
			// (禁静默: a measured physical overlap is disclosed, never
			// silently dropped — byte-identical would hide it); every
			// published value untouched. Production mint chains stamp
			// PeriodicSource on sleep-dominant rows only; this typed guard
			// keeps the invariant independent of that structural argument.
			item.GatedShareOverlapDisclosureMs = overlap
			item.GatedShareClaimSeats = spans
			item.Summary += fmt.Sprintf("; %.3fms of this scheduling-demand account physically overlaps the same thread's priority-inversion seat branch window(s), whose gated composite already counts that runnable share %s — this seat's published attribution is the VS-1 periodic discounted composite (runnable + lateness), not the pure runnable account an interval split re-partitions, so no value split is performed and every published value is unchanged (the true overlap is at least this figure, measured over the available real segments)",
				overlap, levelMergeSummaryClaimSeatOnBoard)
			continue
		}
		if !occComplete || !claimComplete {
			// fail-open (Plan-P face 5 / ruling ④ form): a partial typed
			// inventory can WITNESS the overlap (real segments only — the
			// measure is a lower bound) but must never bound a value split.
			// Published value untouched.
			item.GatedShareOverlapDisclosureMs = overlap
			item.GatedShareClaimSeats = spans
			item.Summary += fmt.Sprintf("; %.3fms of this scheduling-demand account physically overlaps the same thread's priority-inversion seat branch window(s), whose gated composite already counts that runnable share %s — the typed interval inventory is incomplete on at least one side, so no value split is performed and every published value is unchanged (the true overlap is at least this figure, measured over the available real segments)",
				overlap, levelMergeSummaryClaimSeatOnBoard)
			continue
		}
		claimed := overlap
		if claimed > full {
			// The window-intersection measure bounds wall clock, not the
			// runnable time inside it — the claimed share can never exceed
			// the aggregate's own published account. Harmless belt (修补轮
			// 件7 备案, 对抗官 M10 等价突变实证): without this clamp the
			// residual clamps to 0 below and the full-claim rewrite passes
			// `full` explicitly, so both paths emit byte-identical output —
			// the clamp only keeps the intermediate arithmetic honest.
			claimed = full
		}
		residual := rspaClampNonNegative(full - claimed)
		residualSegments := levelMergeResidualSegments(occUnion, claimUnion)
		if residual <= levelMergeGatedShareTolMs {
			// Every credential segment is claimed: the WHOLE seat becomes the
			// demoted constituent row (A with B==0) — the inversion seat
			// carries the value, this row keeps the lossless account visible
			// on the adjacent lane. Identity: claimed==full, residual==0.
			levelMergeRewriteToConstituentRow(item, full, full, spans)
			continue
		}
		// Ordinary split: the surviving seat publishes the residual (B) and
		// the constituent twin (A) mints beside it on the adjacent lane.
		clone := *item
		levelMergeRewriteToConstituentRow(&clone, claimed, full, spans)
		appended = append(appended, clone)
		item.GatedShareClaimedMs = claimed
		item.GatedShareFullMs = full
		item.GatedShareClaimSeats = spans
		item.RunnableMs = residual
		item.EffectiveImpactMs = residual
		item.ImpactMs = residual
		item.ProjectedImpactMs = residual
		// 备案 (修补轮件7): the Score re-prices on the item-aware type weight
		// (runnable_wait 1.15), stepping off the chain-aggregate mint weight
		// (rootCauseScoreWeightChainAggregate 2.05) — the RSPA remainder/clip
		// rewrites take the same track; Score is a tie-breaker face, never a
		// published value.
		item.Score = residual * item.Confidence * rootCauseItemScoreWeight(*item)
		summary := fmt.Sprintf("%s runnable aggregate residual %.3fms after the interval-accounting split: %.3fms of the %.3fms scheduling-demand account lies inside the same thread's priority-inversion seat branch window(s) and is already counted by that seat's gated composite %s — claimed + residual = full account (one segment set, two disjoint shares, additive back)",
			threadLabel(item.Thread), residual, claimed, full, levelMergeSummaryClaimSeatOnBoard)
		if len(residualSegments) == 0 {
			// 链上纪律③ mechanical gate: B keeps the chain lane ONLY on its
			// own residual segments. REACHABLE (修补轮件3, 对抗官探针 + 冷读
			// 补充 — the earlier "arithmetically unreachable" note was wrong):
			// (i) when the published account exceeds the occurrence-window
			// measure, the claim union can cover EVERY window while the value
			// residual full−claimed survives (probe: occ measure 10ms <
			// account 15ms, claim full-covers → residual 5ms, zero residual
			// segments); (ii) the residual can fragment into per-segment
			// slivers each ≤ tol — all filtered — while their value sum stays
			// > tol. Either way the residual seat cannot show ONE credential
			// segment of its own, so it rides the ◇ lane whole through the R4
			// credential-demotion family (ChainCredentialLaneDemoted — the
			// typed lane the RSPA satellite arms use: the demotion itself
			// moves only the channel, demotedSide routing + the R4 legend
			// word follow from the flag; the credential is never inherited
			// from the pre-split aggregate). 「◇+披露」 spec 纪律③ literal.
			item.ChainCredentialLaneDemoted = true
			item.Causality = "adjacent_to_wakeup_chain"
			item.ChainRelevance = "adjacent"
			summary += "; no residual credential segment survives the claim subtraction, so this residual seat rides the adjacent lane (its chain credential is not inherited from the pre-split aggregate)"
		}
		item.Summary = summary
	}
	return append(items, appended...)
}

// levelMergeRewriteToConstituentRow turns a seat (or its clone) into the A
// row: the demoted constituent share already attributed through the inversion
// seat's gated composite. Adjacent lane, zero on-chain full-seat markers, all
// participating value channels = the claimed share, physical evidence
// identity (lines / occurrence windows) untouched.
func levelMergeRewriteToConstituentRow(item *RootCauseRankItem, claimed, full float64, spans []string) {
	item.GatedShareConstituentSeat = true
	item.GatedShareClaimedMs = claimed
	item.GatedShareFullMs = full
	item.GatedShareClaimSeats = spans
	item.Rank = 0
	item.BackgroundRank = 0
	item.RunnableMs = claimed
	item.EffectiveImpactMs = claimed
	item.ImpactMs = claimed
	item.ProjectedImpactMs = claimed
	item.CumulativeImpactMs = claimed
	item.Causality = "adjacent_to_wakeup_chain"
	item.ChainRelevance = "adjacent"
	// Same Score weight-track filing as the residual site (修补轮件7).
	item.Score = claimed * item.Confidence * rootCauseItemScoreWeight(*item)
	item.Summary = fmt.Sprintf("%s runnable share %.3fms of the %.3fms aggregate account already attributed through the same thread's priority-inversion seat gated composite (branch-window overlap measure) %s — constituent share only: it rides the adjacent lane and never competes, the inversion seat carries the value",
		threadLabel(item.Thread), claimed, full, levelMergeSummaryClaimSeatOnBoard)
}

// patchGatedShareSummariesForClaimVisibility runs AFTER each candidate/side
// truncation (build + enrich): every split/disclosure sentence claims its
// inversion seat is on the published board — when truncation killed that
// seat, the claim downgrades to the honest unpublished form (RNB-1 D1
// discipline; verbatim-anchor rewrite, idempotent).
func patchGatedShareSummariesForClaimVisibility(items []RootCauseRankItem) {
	var publishedClaimants map[int]bool
	for i := range items {
		if items[i].Type != "priority_inversion_candidate" ||
			!strings.HasPrefix(strings.TrimSpace(items[i].Source), "wakeup_chain") {
			continue
		}
		if items[i].EffectiveImpactMs <= 0 {
			continue
		}
		if publishedClaimants == nil {
			publishedClaimants = map[int]bool{}
		}
		publishedClaimants[items[i].Thread.PID] = true
	}
	for i := range items {
		item := &items[i]
		if item.GatedShareFullMs <= 0 && item.GatedShareOverlapDisclosureMs <= 0 {
			continue
		}
		if publishedClaimants[item.Thread.PID] {
			continue
		}
		item.Summary = strings.ReplaceAll(item.Summary,
			levelMergeSummaryClaimSeatOnBoard, levelMergeSummaryClaimSeatUnpublished)
	}
}
