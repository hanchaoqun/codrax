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
// are the claim-seat clauses of the split sentences: the held form (the
// claiming inversion seat holds the carved share on the published board) and
// the compaction-disclosure form (it did not survive truncation — RNB-1 D1
// discipline: a board sentence must never point at a row nobody can see).
// HMC377 round 11 (R-377-11 ①): each clause is the span of the split's typed
// gated-share claim, written by the one resolver (resolveRankShareClaims);
// the former verbatim-anchor patch (patchGatedShareSummariesForClaimVisibility,
// pid-keyed) and the span-keyed withdrawal rewrite are removed.
const levelMergeSummaryClaimSeatOnBoard = "(the claiming inversion seat is published on this board)"
const levelMergeSummaryClaimSeatUnpublished = "(the claiming inversion seat is not on the published board; see the compaction disclosure)"

// levelMergeSummaryClaimSeatContext (HMC377 round 10, R-377-10 ②④): the
// claiming seats were published by the trace_query publication gate as
// context only. The split's sentences then say so and no longer say that seat
// counts the share.
const levelMergeSummaryClaimSeatContext = "(the claiming inversion seat " + rspaHolderContextOnly + ")"

// levelMergeGatedShareClaim is the split's typed claim (HMC377 round 11,
// R-377-11 ①): the carved share's instants (the claiming inversion seats'
// branch windows ∩ the aggregate's occurrence windows), held by those seats
// in the gated-share role. The constituent row and the residual seat make it;
// the fail-open overlap disclosure makes it value-free.
func levelMergeGatedShareClaim(pid int, ms float64, carved []foldInterval, disclosure bool) RootCauseShareClaim {
	c := rspaClaim(RootCauseShareClaimGatedShare, RootCauseShareHolderGatedShareSeat, pid, false, ms, carved)
	c.disclosure = disclosure
	return c
}

// levelMergeResidualSentence / levelMergeConstituentSentence /
// levelMergeOverlapSentence compose the split's engine sentences, each with
// its gated-share claim span (one composer per sentence; the resolver writes
// the form the claim's resolution allows — held, compaction disclosure,
// withdrawn claim seat with its successor or with the share taken back).
func levelMergeResidualSentence(thread ThreadRef, residual, claimed, full float64, claim RootCauseShareClaim) rspaSentence {
	var s rspaSentence
	counted := s.spanForms(rspaSpanForms{
		held:      "and is already counted by that seat's gated composite " + levelMergeSummaryClaimSeatOnBoard,
		compact:   "and is already counted by that seat's gated composite " + levelMergeSummaryClaimSeatUnpublished,
		context:   "and is not counted by that seat " + levelMergeSummaryClaimSeatContext,
		successor: "and is counted by another row of this thread on the published board " + levelMergeSummaryClaimSeatContext,
		returned:  "and this seat counts it with its own value " + levelMergeSummaryClaimSeatContext,
	}, claim)
	s.text = fmt.Sprintf("%s runnable aggregate residual %.3fms after the interval-accounting split: %.3fms of the %.3fms scheduling-demand account lies inside the same thread's priority-inversion seat branch window(s) %s — claimed + residual = full account (one segment set, two disjoint shares, additive back)",
		threadLabel(thread), residual, claimed, full, counted)
	return s
}

func levelMergeConstituentSentence(thread ThreadRef, claimed, full float64, claim RootCauseShareClaim) rspaSentence {
	label := threadLabel(thread)
	attributed := func(anchor string) string {
		return fmt.Sprintf("%s runnable share %.3fms of the %.3fms aggregate account already attributed through the same thread's priority-inversion seat gated composite (branch-window overlap measure) %s — constituent share only: it rides the adjacent lane and never competes, the inversion seat carries the value",
			label, claimed, full, anchor)
	}
	overlapping := func(tail string) string {
		return fmt.Sprintf("%s runnable share %.3fms of the %.3fms aggregate account overlapping the same thread's priority-inversion seat branch window(s) (branch-window overlap measure) %s — %s",
			label, claimed, full, levelMergeSummaryClaimSeatContext, tail)
	}
	var s rspaSentence
	s.text = s.spanForms(rspaSpanForms{
		held:      attributed(levelMergeSummaryClaimSeatOnBoard),
		compact:   attributed(levelMergeSummaryClaimSeatUnpublished),
		context:   overlapping("constituent share only: it rides the adjacent lane and never competes"),
		successor: overlapping("constituent share only: it rides the adjacent lane and never competes; another row of this thread on the published board counts this share"),
		returned:  overlapping("constituent share: it rides the adjacent lane and counts this share with its own value"),
		kind:      rspaSpanKindConstituent, label: label, inside: claimed, full: full,
	}, claim)
	return s
}

// levelMergeConstituentMixedForm is a constituent row's sentence when its
// carved share ends split (HMC377 round 11, R-377-11 ①): part counted by a
// claiming inversion seat on the board (or by a withdrawn seat's successor),
// part taken back by this row because its claiming seat is context only,
// part held by a seat the cap cut.
func levelMergeConstituentMixedForm(label string, claimed, full float64, st rspaSpanState) string {
	var parts []string
	if st.resolvedMs > rspaAnchorIdentityTolMs {
		if st.successor {
			parts = append(parts, fmt.Sprintf("%.3fms of it is counted by another row of this thread on the published board", st.resolvedMs))
		} else {
			parts = append(parts, fmt.Sprintf("%.3fms of it is already attributed through a claiming inversion seat's gated composite on this board", st.resolvedMs))
		}
	}
	parts = append(parts, fmt.Sprintf("the claiming inversion seat of %.3fms of it %s", st.returnedMs, rspaHolderContextOnly))
	if st.restMs > rspaAnchorIdentityTolMs {
		parts = append(parts, fmt.Sprintf("the claiming inversion seat of the other %.3fms is not on the published board (see the compaction disclosure)", st.restMs))
	}
	return fmt.Sprintf("%s runnable share %.3fms of the %.3fms aggregate account overlapping the same thread's priority-inversion seat branch window(s) (branch-window overlap measure): %s — constituent share: it rides the adjacent lane and counts those %.3fms with its own value",
		label, claimed, full, strings.Join(parts, "; "), st.returnedMs)
}

func levelMergeOverlapSentence(overlap float64, periodic bool, claim RootCauseShareClaim) rspaSentence {
	var s rspaSentence
	counts := s.spanForms(rspaSpanForms{
		held:    "whose gated composite already counts that runnable share " + levelMergeSummaryClaimSeatOnBoard,
		compact: "whose gated composite already counts that runnable share " + levelMergeSummaryClaimSeatUnpublished,
		context: "whose gated composite is not published " + levelMergeSummaryClaimSeatContext,
	}, claim)
	if periodic {
		s.text = fmt.Sprintf("; %.3fms of this scheduling-demand account physically overlaps the same thread's priority-inversion seat branch window(s), %s — this seat's published attribution is the VS-1 periodic discounted composite (runnable + lateness), not the pure runnable account an interval split re-partitions, so no value split is performed and every published value is unchanged (the true overlap is at least this figure, measured over the available real segments)",
			overlap, counts)
		return s
	}
	s.text = fmt.Sprintf("; %.3fms of this scheduling-demand account physically overlaps the same thread's priority-inversion seat branch window(s), %s — the typed interval inventory is incomplete on at least one side, so no value split is performed and every published value is unchanged (the true overlap is at least this figure, measured over the available real segments)",
		overlap, counts)
	return s
}

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
	// idx: the claimant's row (HMC377 round 11: the split stamps the
	// gated-share hold on it).
	idx int
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
			idx:      i,
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

// levelMergeTimeFold converts typed windows to a merged instant union.
func levelMergeTimeFold(windows []TimeWindow) []foldInterval {
	out := make([]foldInterval, 0, len(windows))
	for _, w := range windows {
		if w.EndTs > w.StartTs {
			out = append(out, foldInterval{start: w.StartTs, end: w.EndTs})
		}
	}
	return rspaFoldCopy(out)
}

// levelMergeStampClaimSeatHolds records what each claiming inversion seat
// holds for the split (HMC377 round 11, R-377-11 ①): the runnable time inside
// its own branch windows, which its gated composite counts — the gated-share
// role the split's claims name.
func levelMergeStampClaimSeatHolds(items []RootCauseRankItem, claimants []levelMergeClaimant) {
	for _, claimant := range claimants {
		if claimant.idx < 0 || claimant.idx >= len(items) {
			continue
		}
		seat := &items[claimant.idx]
		rspaHold(seat, RootCauseShareHolderGatedShareSeat, false, levelMergeTimeFold(claimant.windows), seat.GatedRunnableMs)
	}
}

// levelMergeRestampResidualHolds keeps the residual seat's holds off the
// carved share (HMC377 round 11, R-377-11 ①): a hold the pre-split aggregate
// recorded (its segments inside the walked windows, at its pre-split
// account) holds only what the residual still counts — the carved instants
// are the constituent's, counted at the claiming seat.
func levelMergeRestampResidualHolds(item *RootCauseRankItem, carved []foldInterval, claimed float64) {
	if len(item.rspaHeld) == 0 {
		return
	}
	held := make([]rspaHeldShare, 0, len(item.rspaHeld))
	for _, h := range item.rspaHeld {
		if len(h.windows) > 0 {
			rest := foldIntervalSubtract(h.windows, carved)
			if rspaFoldLengthMs(rest) <= levelMergeGatedShareTolMs {
				continue
			}
			h.windows = rest
		}
		h.ms = rspaClampNonNegative(h.ms - claimed)
		if h.ms <= levelMergeGatedShareTolMs {
			continue
		}
		held = append(held, h)
	}
	item.rspaHeld = held
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
		// HMC377 round 11: the carved instants and the claiming seats' holds.
		carved := rspaFoldIntersect(levelMergeTimeFold(claimUnion), levelMergeTimeFold(occUnion))
		levelMergeStampClaimSeatHolds(items, claimants)
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
			// PeriodicSource on sleep-dominant rows and — GAP-B2, 2026-07-25
			// — on d_sleep∧timer-closed-set rows; this typed guard keeps the
			// invariant independent of that structural argument either way.
			item.GatedShareOverlapDisclosureMs = overlap
			item.GatedShareClaimSeats = spans
			rspaAppendSentence(item, levelMergeOverlapSentence(overlap, true, levelMergeGatedShareClaim(item.Thread.PID, overlap, carved, true)))
			continue
		}
		if !occComplete || !claimComplete {
			// fail-open (Plan-P face 5 / ruling ④ form): a partial typed
			// inventory can WITNESS the overlap (real segments only — the
			// measure is a lower bound) but must never bound a value split.
			// Published value untouched.
			item.GatedShareOverlapDisclosureMs = overlap
			item.GatedShareClaimSeats = spans
			rspaAppendSentence(item, levelMergeOverlapSentence(overlap, false, levelMergeGatedShareClaim(item.Thread.PID, overlap, carved, true)))
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
			levelMergeRewriteToConstituentRow(item, full, full, spans, carved)
			continue
		}
		// Ordinary split: the surviving seat publishes the residual (B) and
		// the constituent twin (A) mints beside it on the adjacent lane.
		clone := *item
		levelMergeRewriteToConstituentRow(&clone, claimed, full, spans, carved)
		appended = append(appended, clone)
		levelMergeRestampResidualHolds(item, carved, claimed)
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
		rspaSetSentence(item, levelMergeResidualSentence(item.Thread, residual, claimed, full,
			levelMergeGatedShareClaim(item.Thread.PID, claimed, carved, false)))
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
			item.Summary += "; no residual credential segment survives the claim subtraction, so this residual seat rides the adjacent lane (its chain credential is not inherited from the pre-split aggregate)"
		}
	}
	return append(items, appended...)
}

// levelMergeRewriteToConstituentRow turns a seat (or its clone) into the A
// row: the demoted constituent share already attributed through the inversion
// seat's gated composite. Adjacent lane, zero on-chain full-seat markers, all
// participating value channels = the claimed share, physical evidence
// identity (lines / occurrence windows) untouched.
func levelMergeRewriteToConstituentRow(item *RootCauseRankItem, claimed, full float64, spans []string, carved []foldInterval) {
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
	// HMC377 round 11: the constituent speaks and holds for itself only — its
	// one claim is the carved share (held by the claiming seats).
	rspaClearClaims(item)
	rspaSetSentence(item, levelMergeConstituentSentence(item.Thread, claimed, full,
		levelMergeGatedShareClaim(item.Thread.PID, claimed, carved, false)))
}
