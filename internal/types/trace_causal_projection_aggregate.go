package types

// Deterministic pre-render aggregation for the Trace Causal Projection
// (presentation v3 §6, docs/design/trace_projection_presentation_v3_20260702.md).
//
// Real customer traces surfaced three presentation-poisoning duplication shapes
// that no renderer can fix row-by-row:
//   - the SAME wall-clock fact emitted under two predicates (an io_latency
//     primary row and its critical_blocking twin with identical ms + identical
//     evidence line range) rendering as two rows;
//   - the same (subject, cause) repeated many times with tiny values (six
//     sub-ms io_latency rows flooding the first screen);
//   - background rows whose impact point is the unknown-thread sentinel
//     flooding half the report.
//
// All rules here are STRICT, pure comparisons (user-adjudicated tolerance):
// R1 merges only on subject + projected ms equal at 3 decimals + identical
// evidence line range; R2 groups only on exactly-equal (subject, object); R3
// keys only on the unknown-thread sentinel. No ±ε approximation, no prose.
// ONE adjudicated exception: V4's near-duplicate tier (PTV6 批② #4,
// 2026-07-06) admits a bounded ≤3% value band — but only INSIDE the full V4
// identity (equal subject + REAL non-sentinel object + TypeToken) AND a
// precise line/time overlap, and it folds to the member MAX with the
// publication count disclosed; it never feeds a sum. Every merged row's observation id is
// retained (MergedEvidenceIDs), so the aggregation is lossless for
// auditability.
//
// R2's ×N value carries the member SUM with one value-CALIBER exception
// (§11-N2, real_trace_campaign_20260705.md): members from DISTINCT query
// windows whose occurrence intervals overlap re-measured the same physical
// wall clock, so the merged row publishes the interval-union caliber (typed
// StartTs/EndTs algebra, shared authority in
// trace_causal_projection_interval.go) with the raw Σ retained in
// MergedSumMS. Grouping, membership and every disjoint/window-less shape are
// byte-unchanged.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	// traceCausalProjectionSameKindAggregateMin is the R2 threshold: only ≥3
	// repeats of the same (subject, object) collapse into one ×N row — merging
	// two rows saves nothing and hides the repetition count.
	traceCausalProjectionSameKindAggregateMin = 3
	// traceCausalProjectionUnknownBackgroundKeep / Min: R3 keeps the top-K
	// unknown-impact-point background rows and folds the rest, but only when at
	// least two rows would fold (N ≥ Keep+2) — folding a single row is noise.
	traceCausalProjectionUnknownBackgroundKeep = 2
	traceCausalProjectionUnknownBackgroundMin  = traceCausalProjectionUnknownBackgroundKeep + 2
	// traceCausalProjectionSecondaryObjectCap bounds the 影响点 note list an R1
	// survivor accumulates; further merged views keep their evidence ids but do
	// not grow the display note.
	traceCausalProjectionSecondaryObjectCap = 3
	// traceCausalProjectionMergedSubjectCap bounds MergedSubjects on a merged
	// row: up to 4 distinct member thread names are preserved for display;
	// anything beyond is expressed through MergedCount (the evidence ids stay
	// lossless in MergedEvidenceIDs).
	traceCausalProjectionMergedSubjectCap = 4
	// traceCausalProjectionDuplicatePublicationNearTolerance is V4's near-tier
	// band (PTV6 批② #4, 2026-07-06): duplicate publications of ONE wall-clock
	// measurement re-carve its boundary as adjacent samples land, so the
	// republished values drift by the tail-sampling delta instead of matching
	// bit-for-bit. Real specimen (single-thread io_latency, window 2.992ms):
	// 1.354/1.382/1.383ms over pairwise-overlapping line spans 2908-3094 /
	// 2911-3114 / 2913-3120 — max pairwise drift 2.10% — escaped the exact
	// lane by 0.03ms and R2-SUMMED into a 4.119ms phantom (138% of the window,
	// physically impossible for one thread). 3% covers that observed
	// boundary-refinement drift with margin and stays deliberately narrow:
	// genuinely additive same-(subject,object) segments inside one wide
	// enclosing evidence range differ by far more than 3% (heterogeneous
	// magnitudes), so they keep the R2 SUM path; the residual risk — two REAL
	// distinct waits landing within 3% of each other AND overlapping in the
	// artifact — is the same quantization risk RF2a already accepted for the
	// exact lane, narrowed further by the band's upper bound. The band is NOT
	// the whole guard: the adjudicated distinct-fact shape (two same-subject
	// waits on UNRESOLVED peers, 9µs = 0.008% apart, overlapping ranges) sits
	// inside ANY band, so the near lane additionally requires a real
	// non-sentinel object identity (traceCausalProjectionSameDuplicatePublication).
	traceCausalProjectionDuplicatePublicationNearTolerance = 0.03
)

func traceCausalProjectionAggregateForPresentation(out *TraceCausalProjection) {
	if out == nil {
		return
	}
	traceCausalProjectionMergeSameFacts(out)
	// B.2 arm A (v5 P1 件①, 2026-07-13) runs right after R1: the raw
	// root_evidence same-segment twins R1's value-keyed identity cannot reach
	// (valueless raw copies; keepers valued on the eff/actual lanes only)
	// converge HERE, so R4/V4/R2 never see the bare double seat.
	traceCausalProjectionConvergeRawSegmentTwins(out)
	// R4 peer-alias fold runs between R1 (same-fact) and R2 (×N): the two alias
	// rows carry slightly different ms, so R1's strict identity never catches
	// them, and letting them reach R2 would risk a double-counting ×2 sum.
	out.PrimaryRootCauses = traceCausalProjectionMergePeerAliases(out.PrimaryRootCauses)
	out.OnChainCauses = traceCausalProjectionMergePeerAliases(out.OnChainCauses)
	out.AdjacentCauses = traceCausalProjectionMergePeerAliases(out.AdjacentCauses)
	out.BackgroundCauses = traceCausalProjectionMergePeerAliases(out.BackgroundCauses)
	out.SupportingHops = traceCausalProjectionMergePeerAliases(out.SupportingHops)
	// V4 duplicate-publication dedup MUST run before R2: three same-value
	// overlapping publications would otherwise reach the ≥3 threshold and SUM
	// into a 3× phantom total (customer revisit 2026-07-03: three 35.350ms
	// irq_activity rows over overlapping spans published as 106.05ms; PTV6
	// 批② #4: three near-value 1.354/1.382/1.383ms io_latency republications
	// escaped the exact lane by 0.03ms and summed into a 4.119ms/138%-of-window
	// phantom — the near lane folds those too). After the fold the survivor
	// count is what R2 legitimately sees.
	out.PrimaryRootCauses = traceCausalProjectionDedupDuplicatePublications(out.PrimaryRootCauses)
	out.OnChainCauses = traceCausalProjectionDedupDuplicatePublications(out.OnChainCauses)
	out.AdjacentCauses = traceCausalProjectionDedupDuplicatePublications(out.AdjacentCauses)
	out.BackgroundCauses = traceCausalProjectionDedupDuplicatePublications(out.BackgroundCauses)
	out.SupportingHops = traceCausalProjectionDedupDuplicatePublications(out.SupportingHops)
	// WO-G2 (SMR-1 批 SMR-S12b, smr_audit_report §②, 2026-07-12): zero-value
	// instant markers fold into their enclosing valued row as valueless members
	// (§29.13 无时长值成员披露 same-lane execution) — runs between V4 and R2 so
	// the marker never seats and never perturbs the ×N sum.
	out.PrimaryRootCauses = traceCausalProjectionFoldZeroValueMarkerRows(out.PrimaryRootCauses)
	out.OnChainCauses = traceCausalProjectionFoldZeroValueMarkerRows(out.OnChainCauses)
	out.AdjacentCauses = traceCausalProjectionFoldZeroValueMarkerRows(out.AdjacentCauses)
	out.BackgroundCauses = traceCausalProjectionFoldZeroValueMarkerRows(out.BackgroundCauses)
	out.SupportingHops = traceCausalProjectionFoldZeroValueMarkerRows(out.SupportingHops)
	out.PrimaryRootCauses = traceCausalProjectionAggregateSameKind(out.PrimaryRootCauses)
	out.OnChainCauses = traceCausalProjectionAggregateSameKind(out.OnChainCauses)
	out.AdjacentCauses = traceCausalProjectionAggregateSameKind(out.AdjacentCauses)
	out.BackgroundCauses = traceCausalProjectionAggregateSameKind(out.BackgroundCauses)
	out.SupportingHops = traceCausalProjectionAggregateSameKind(out.SupportingHops)
	// B.2 arms B/C (v5 P1 件①, 2026-07-13) run AFTER R2, where every ×N
	// keeper shape (wire fold / R2 merge / marker fold) is final: raw member
	// re-issues of a merged seat fold into it (arm B, 25846 shape), and one
	// physical segment set R2-merged independently on two lanes converges to
	// one seat (arm C, 42729 E9/E15 shape). E# union only — no account moves.
	traceCausalProjectionConvergeRawMemberReissues(out)
	traceCausalProjectionConvergeMergedTwinSeats(out)
	out.BackgroundCauses = traceCausalProjectionFoldUnknownBackground(out.BackgroundCauses)
	// RANK-U Stage 2 (donghu W1 witness, 2026-07-13): carry the R1-merged
	// semantic seat onto the entity's SemanticSpans display copy — the ✦ 语义
	// lane renders from THAT bucket, and without the hand-off the engine's
	// seat evaporated between the merge survivor (classified bucket copy) and
	// the rendered row.
	traceCausalProjectionUnifySemanticSpanSeats(out)
	traceCausalProjectionResortAfterAggregation(out)
}

// traceCausalProjectionUnifySemanticSpanSeats (RANK-U Stage 2, 2026-07-13):
// bucket-copy seat unification for semantic span entities. The compile keeps
// one classified copy of a semantic record in the relevance buckets (the R1
// merge survivor — it may have absorbed the entity's RANK view: seat ordinal,
// ladder tier, published effective) AND one copy in SemanticSpans (the ✦
// display lane's source; possibly a sibling emission of the same family under
// its own record id). Identity is the EXACT EvidenceID / R1 MergedEvidenceIDs
// membership, else the SAME strict same-fact key R1 itself merges on (one
// identity function, never a new heuristic). Empty display-copy slots adopt
// the survivor's seat/tier/effective; non-empty slots always keep their own
// (same doctrine as the R1 absorb).
func traceCausalProjectionUnifySemanticSpanSeats(out *TraceCausalProjection) {
	if len(out.SemanticSpans) == 0 {
		return
	}
	donors := map[string]*TraceCausalProjectionNode{}
	factDonors := map[string]*TraceCausalProjectionNode{}
	buckets := []*[]TraceCausalProjectionNode{
		&out.PrimaryRootCauses,
		&out.OnChainCauses,
		&out.SupportingHops,
		&out.AdjacentCauses,
		&out.BackgroundCauses,
	}
	for _, bucket := range buckets {
		for i := range *bucket {
			node := &(*bucket)[i]
			if node.Role != TraceCausalRoleSemanticSpan &&
				strings.TrimSpace(node.Predicate) != "trace_semantic_span" {
				continue
			}
			if node.Rank <= 0 && node.BackgroundRank <= 0 && node.EffectiveImpactMS <= 0 {
				continue
			}
			ids := append([]string{node.EvidenceID}, node.MergedEvidenceIDs...)
			for _, id := range ids {
				key := traceCausalProjectionCanonicalNode(id)
				if key == "" {
					continue
				}
				if _, exists := donors[key]; !exists {
					donors[key] = node
				}
			}
			if key := traceCausalProjectionSameFactKey(*node); key != "" {
				if _, exists := factDonors[key]; !exists {
					factDonors[key] = node
				}
			}
		}
	}
	if len(donors) == 0 && len(factDonors) == 0 {
		return
	}
	for i := range out.SemanticSpans {
		sem := &out.SemanticSpans[i]
		donor := donors[traceCausalProjectionCanonicalNode(sem.EvidenceID)]
		if donor == nil {
			if key := traceCausalProjectionSameFactKey(*sem); key != "" {
				donor = factDonors[key]
			}
		}
		if donor == nil {
			continue
		}
		if sem.Rank == 0 && donor.Rank > 0 {
			sem.Rank = donor.Rank
			if strings.TrimSpace(sem.Tier) == "" {
				sem.Tier = donor.Tier
			}
			// XLANE-3 件1: the adopted seat's board identity travels with it
			// (same donor discipline as the R1 absorb arm).
			if sem.RankBoardTarget == "" {
				sem.RankBoardTarget = donor.RankBoardTarget
			}
			if sem.RankBoardParamsFingerprint == "" {
				sem.RankBoardParamsFingerprint = donor.RankBoardParamsFingerprint
			}
			// ELIM-V2 (2026-07-18): the engine-stamped fix direction travels
			// with the adopted seat (verbatim; empty-slot fill only).
			if strings.TrimSpace(sem.FixDirection) == "" {
				sem.FixDirection = donor.FixDirection
			}
		}
		if sem.BackgroundRank == 0 && donor.BackgroundRank > 0 {
			sem.BackgroundRank = donor.BackgroundRank
			if strings.TrimSpace(sem.Tier) == "" {
				sem.Tier = donor.Tier
			}
		}
		if sem.EffectiveImpactMS <= 0 && donor.EffectiveImpactMS > 0 {
			sem.EffectiveImpactMS = donor.EffectiveImpactMS
			sem.EffectiveImpactPublished = sem.EffectiveImpactPublished || donor.EffectiveImpactPublished
		}
	}
}

// --- R1: cross-predicate same-fact merge -----------------------------------

// traceCausalProjectionSameFactKey returns the strict identity of one observed
// wall-clock fact: subject + projected ms at 3 decimals + the exact evidence
// line range. Empty when the node lacks a line span or a positive projected
// value — such rows are never merged.
func traceCausalProjectionSameFactKey(node TraceCausalProjectionNode) string {
	if node.LineStart <= 0 || node.LineEnd < node.LineStart {
		return ""
	}
	impact := node.ImpactMS
	if impact <= 0 {
		impact = node.CumulativeImpactMS
	}
	// RSPA (§29.61.10, 2026-07-14): the ⛓ clipped half of a re-anchored seat
	// still DESCRIBES the same physical full-window fact its un-migrated
	// twins publish (critical_blocking / window-view rows carry the full
	// account over the same line range) — the value-keyed identity therefore
	// reads the typed full-account float, restoring the pre-RSPA one-fact-
	// one-row merge (survivor keeps its published anchored value; the twin's
	// E# folds in losslessly). The ◇ remainder half is its OWN account and
	// never joins a value-keyed identity (marker-forked key).
	remainderHalf := ""
	if node.ChainAnchorRemainderSeat {
		remainderHalf = "\x00remainder"
	} else if node.ChainAnchorFullMS > 0 {
		impact = node.ChainAnchorFullMS
	}
	// LEVELMERGE-1 件2 (方案 P, 2026-07-18): same discipline as the RSPA
	// bipartition — the gated-share CONSTITUENT half is its OWN account and
	// never joins a value-keyed identity; the residual seat still DESCRIBES
	// the same physical full account its un-split twins publish, so its
	// value-keyed identity reads the typed full-account float.
	if node.GatedShareConstituentSeat {
		remainderHalf += "\x00gated_constituent"
	} else if node.GatedShareFullMS > 0 {
		impact = node.GatedShareFullMS
	}
	if impact <= 0 {
		return ""
	}
	subject := traceCausalProjectionCanonicalNode(node.Subject)
	if subject == "" {
		return ""
	}
	return fmt.Sprintf("%s\x00%.3f\x00%d\x00%d%s", subject, impact, node.LineStart, node.LineEnd, remainderHalf)
}

// traceCausalProjectionMergeSameFacts merges R1 duplicates across the
// projection buckets in priority order (primary → on-chain → hops → adjacent →
// background; semantic spans are deliberately excluded — a span is a different
// kind of fact than a state/cause row even at identical coordinates). The
// first occurrence in scan order survives; later occurrences with a DIFFERENT
// EvidenceID fold into it (same-EvidenceID hits are the survivor's own
// cross-bucket copy, which bucket-overlap semantics require keeping).
func traceCausalProjectionMergeSameFacts(out *TraceCausalProjection) {
	type survivorRef struct {
		bucket int
		index  int
	}
	buckets := []*[]TraceCausalProjectionNode{
		&out.PrimaryRootCauses,
		&out.OnChainCauses,
		&out.SupportingHops,
		&out.AdjacentCauses,
		&out.BackgroundCauses,
	}
	survivors := map[string]survivorRef{}
	merged := map[string]map[string]bool{} // fact key -> evidence ids already absorbed
	// SFD 复核 F4: per-fact-key fold back-fill state — the absorb lane mirrors
	// the join lane's donor-conflict rule (ambiguity fails open, never
	// first-writer-wins), which needs memory across sequential losers.
	foldBackfills := map[string]*traceCausalProjectionSupplyFoldBackfill{}
	for b, bucket := range buckets {
		kept := (*bucket)[:0]
		for _, node := range *bucket {
			key := traceCausalProjectionSameFactKey(node)
			if key == "" {
				kept = append(kept, node)
				continue
			}
			ref, seen := survivors[key]
			if !seen {
				survivors[key] = survivorRef{bucket: b, index: len(kept)}
				merged[key] = map[string]bool{traceCausalProjectionCanonicalNode(node.EvidenceID): true}
				foldBackfills[key] = &traceCausalProjectionSupplyFoldBackfill{}
				kept = append(kept, node)
				continue
			}
			survivor := &(*buckets[ref.bucket])[ref.index]
			if traceCausalProjectionCanonicalNode(node.EvidenceID) != "" &&
				traceCausalProjectionCanonicalNode(node.EvidenceID) == traceCausalProjectionCanonicalNode(survivor.EvidenceID) {
				// The survivor's own copy in another bucket — keep it so bucket
				// overlap semantics (and their consumers) stay intact. A copy
				// scanned BEFORE a fold-carrying loser stays bare here; the
				// post-aggregation join pass re-unifies it from the back-filled
				// survivor (same key, same account → clean donor), so surfaces
				// converge.
				kept = append(kept, node)
				continue
			}
			// RSPA (§29.61.10, 2026-07-14): when a full-account twin met a
			// re-anchored ⛓ clipped half under the full-keyed identity, the ⛓
			// half OWNS the published seat (its value is the credential-
			// anchored account; letting the full-value twin survive would
			// republish the very full-window claim the migration retired).
			// Swap the node into the survivor slot and absorb the displaced
			// full-account view as the loser.
			// 件2 (修复轮, 2026-07-14): the id accounting swaps WITH the seat —
			// the seed slot held the old survivor's id, and absorbing the
			// displaced twin without re-seeding skipped its id entirely
			// (MergedEvidenceIDs came out empty; "lossless" was false). The
			// new survivor's id takes the seed; the displaced id becomes
			// absorbable and is recorded by the absorb below.
			if node.ChainAnchorFullMS > 0 && !node.ChainAnchorRemainderSeat && survivor.ChainAnchorFullMS == 0 {
				displaced := *survivor
				*survivor = node
				node = displaced
				delete(merged[key], traceCausalProjectionCanonicalNode(node.EvidenceID))
				if id := traceCausalProjectionCanonicalNode(survivor.EvidenceID); id != "" {
					merged[key][id] = true
				}
			}
			traceCausalProjectionAbsorbSameFact(survivor, node, merged[key], foldBackfills[key])
		}
		*bucket = kept
	}
}

// traceCausalProjectionSupplyFoldBackfill is the R1-lane fold back-fill state
// of ONE same-fact survivor (SFD 复核 F4): backfilled marks an account taken
// from an absorbed loser (as opposed to the survivor's OWN publication, which
// is never overwritten); conflicted poisons the key after two absorbed views
// disagreed — the group is cleared and never refilled (fail-open, 裸值保留 —
// the same ambiguity rule as the join lane's donor conflict).
type traceCausalProjectionSupplyFoldBackfill struct {
	backfilled bool
	conflicted bool
}

// traceCausalProjectionAbsorbSupplyFold is the SupplyFold arm of the same-fact
// absorb (SFD §15.A display half + SFD 复核 F1/F4): the loser view of this ONE
// fact may carry the engine-published supply-fold accounting the survivor's
// funnel never set (the rootCauseItem running twin publishes basis=nil by
// construction while its causal-impact twin carries the typed fold_basis
// notes). The group copies as a unit keyed on the presence flag — zeros are
// load-bearing (§7.10 affirmative branch) — under the SAME precision guards
// as the post-aggregation join lane (traceCausalProjectionJoinSupplyFoldTwins
// covers twins whose projected values differ; an absorbed donor is gone
// before that pass, so this arm handles the value-equal R1 shape).
func traceCausalProjectionAbsorbSupplyFold(survivor *TraceCausalProjectionNode, loser TraceCausalProjectionNode, backfill *traceCausalProjectionSupplyFoldBackfill) {
	if backfill == nil || !loser.SupplyFoldComputed {
		return
	}
	// A survivor that PUBLISHED its own basis keeps it unconditionally — it
	// won the priority scan (the same "conflicting non-empty values keep the
	// survivor's" doctrine as every typed slot above).
	if survivor.SupplyFoldComputed && !backfill.backfilled {
		return
	}
	if backfill.conflicted {
		return
	}
	// SFD 复核 F4(c) — running-state gate, mirroring the join lane: the fold's
	// deficit is defined over the segment's OWN running wall clock (§7.10).
	// The state back-fill above already adopted the loser's state when the
	// survivor's was empty, and every fold carrier is a running row, so this
	// arm is satisfied by construction in the reachable shapes; a survivor
	// with a DIFFERENT non-empty state at an identical subject + %.3f value +
	// line range would need bit-equal cross-state projections (near
	// unreachable) — if it ever occurs, the fold stays off it.
	if strings.TrimSpace(strings.ToLower(survivor.StateKind)) != "running" {
		return
	}
	// SFD 复核 F1 — cross-window veto, mirroring the join lane: both sides
	// declaring their own typed selected_window with any endpoint deviating
	// beyond the F-2 tolerance re-measured the segment in DIFFERENT windows
	// (§11-N2 — reachable via overlapping window_sweep re-measurements that
	// R1-merge on an identical %.3f value + line range); the fold describes
	// the loser's window's clamping and never back-fills across.
	if TraceCausalProjectionWindowPresent(survivor.QueryWindowStartTs, survivor.QueryWindowEndTs) &&
		TraceCausalProjectionWindowPresent(loser.QueryWindowStartTs, loser.QueryWindowEndTs) &&
		(math.Abs(survivor.QueryWindowStartTs-loser.QueryWindowStartTs) > traceCausalProjectionFullWindowSameWindowToleranceS ||
			math.Abs(survivor.QueryWindowEndTs-loser.QueryWindowEndTs) > traceCausalProjectionFullWindowSameWindowToleranceS) {
		return
	}
	if backfill.backfilled {
		// SFD 复核 F4 — the join lane's donor-conflict rule on the absorb
		// lane: a later same-fact view DISAGREEING with the already
		// back-filled accounting is ambiguity. Clear the group and poison the
		// key (fail-open to the bare value) — never first-writer-wins.
		if survivor.SupplyFoldDeficitMS != loser.SupplyFoldDeficitMS ||
			survivor.SupplyFoldIdealMS != loser.SupplyFoldIdealMS ||
			survivor.SupplyFoldKnownMS != loser.SupplyFoldKnownMS ||
			survivor.SupplyFoldUnknownMS != loser.SupplyFoldUnknownMS {
			survivor.SupplyFoldComputed = false
			survivor.SupplyFoldDeficitMS = 0
			survivor.SupplyFoldIdealMS = 0
			survivor.SupplyFoldKnownMS = 0
			survivor.SupplyFoldUnknownMS = 0
			survivor.SupplyFoldCapabilitySource = ""
			survivor.SupplyFoldReferenceClass = ""
			backfill.backfilled = false
			backfill.conflicted = true
		}
		return
	}
	survivor.SupplyFoldComputed = true
	survivor.SupplyFoldDeficitMS = loser.SupplyFoldDeficitMS
	survivor.SupplyFoldIdealMS = loser.SupplyFoldIdealMS
	survivor.SupplyFoldKnownMS = loser.SupplyFoldKnownMS
	survivor.SupplyFoldUnknownMS = loser.SupplyFoldUnknownMS
	// CAP (§26 C3 + 复核 F1): the caliber and basis-class tokens travel with
	// the accounting group they price (same unit-copy rule as the join lane's
	// donor).
	survivor.SupplyFoldCapabilitySource = loser.SupplyFoldCapabilitySource
	survivor.SupplyFoldReferenceClass = loser.SupplyFoldReferenceClass
	backfill.backfilled = true
}

// traceCausalProjectionIdleCadence reads a node's idle-cadence annotation
// (ENG-2, 2026-07-12): either the carried IdleCadenceMS/Kind pair of an
// already-annotated node, or the node's OWN typed idle lane (canonical
// pacing_idle / periodic_idle on the TypeToken→Object→Predicate precedence)
// valued at its published impact. Exact typed token match only.
func traceCausalProjectionIdleCadence(node TraceCausalProjectionNode) (float64, string) {
	if node.IdleCadenceMS > 0 && node.IdleCadenceKind != "" {
		return node.IdleCadenceMS, node.IdleCadenceKind
	}
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		switch traceCausalProjectionCanonicalNode(token) {
		case "pacing_idle", "periodic_idle":
			ms := node.ImpactMS
			if ms <= 0 {
				ms = node.CumulativeImpactMS
			}
			return ms, traceCausalProjectionCanonicalNode(token)
		}
	}
	return 0, ""
}

func traceCausalProjectionAbsorbSameFact(survivor *TraceCausalProjectionNode, loser TraceCausalProjectionNode, absorbed map[string]bool, foldBackfill *traceCausalProjectionSupplyFoldBackfill) {
	appendEvidence := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		key := traceCausalProjectionCanonicalNode(id)
		if absorbed[key] {
			return
		}
		absorbed[key] = true
		survivor.MergedEvidenceIDs = append(survivor.MergedEvidenceIDs, id)
	}
	appendEvidence(loser.EvidenceID)
	for _, id := range loser.MergedEvidenceIDs {
		appendEvidence(id)
	}
	if object := strings.TrimSpace(loser.Object); object != "" &&
		traceCausalProjectionCanonicalNode(object) != traceCausalProjectionCanonicalNode(survivor.Object) {
		// PTV5 Q4 (#68 用户裁定 2026-07-05, Object 空路径打通): a survivor with
		// an EMPTY Object takes the loser's Object as its own cause token — a
		// root_evidence-family loser carries the typed cause on this lane, and
		// shunting it to SecondaryObjects left the merged row causeless.
		// Conflicting non-empty Objects keep the survivor's and record the
		// loser's as an 影响点, exactly as before.
		if strings.TrimSpace(survivor.Object) == "" {
			survivor.Object = object
		} else {
			traceCausalProjectionAppendSecondaryObject(survivor, object)
		}
	}
	for _, object := range loser.SecondaryObjects {
		traceCausalProjectionAppendSecondaryObject(survivor, object)
	}
	// Field back-fill: the merged views describe ONE fact, so empty typed slots
	// on the survivor take the loser's value; conflicting non-empty values keep
	// the survivor's (it won the priority scan).
	if survivor.StateKind == "" {
		survivor.StateKind = loser.StateKind
	}
	if survivor.SubjectKind == "" {
		survivor.SubjectKind = loser.SubjectKind
	}
	// ELIM-V2 (2026-07-18): the registry fix-direction attribute follows the
	// fact — the rank-lane view carries the engine-stamped token while the
	// chain-lane survivor arrives bare, and dropping it stranded the merged
	// seat in the ◎ 方向未定 tail section. Verbatim engine value, empty-slot
	// fill only (a survivor's own published direction always wins).
	if strings.TrimSpace(survivor.FixDirection) == "" {
		survivor.FixDirection = loser.FixDirection
	}
	// §29.50.5 / DSTATE-REFINE proof family (v5 P1 批 件②, 2026-07-13): the
	// merged views describe ONE physical set of segments, so the typed D/IO
	// proof fields follow the fact — the 修复轮 P2-3 twin propagation
	// precedent lifted to the compile merge (the single-member remainder
	// seat's value equals its chain twin's, so R1 folds them and the
	// survivor must keep the 「(原因未证)」/等待对象 word inputs).
	// OR-monotone booleans; empty-slot caller fill; the residual pair moves
	// as a pair (count+symbols are one disclosure).
	survivor.DStateRefinedNonIO = survivor.DStateRefinedNonIO || loser.DStateRefinedNonIO
	survivor.DStateCauseUnprovenRemainder = survivor.DStateCauseUnprovenRemainder || loser.DStateCauseUnprovenRemainder
	// RSPA (§29.61.10): the bipartition trio travels as ONE disclosure —
	// values + remainder marker move together (a survivor with its own
	// decomposition keeps it; the two halves of one bipartition can never R1
	// same-fact merge because their published values differ by construction).
	if survivor.ChainAnchorFullMS == 0 && loser.ChainAnchorFullMS > 0 {
		survivor.ChainAnchoredMS = loser.ChainAnchoredMS
		survivor.ChainAnchorFullMS = loser.ChainAnchorFullMS
		survivor.ChainAnchorRemainderSeat = loser.ChainAnchorRemainderSeat
		// RNB-1 (§29.88 R2): the case-A' divergence disclosure is part of the
		// same bipartition disclosure — it travels with the trio (never
		// mixed with a survivor's own decomposition).
		survivor.ChainAnchorOwnershipDivergent = loser.ChainAnchorOwnershipDivergent
		survivor.ChainAnchorChainLaneMS = loser.ChainAnchorChainLaneMS
		survivor.ChainAnchorCensusMS = loser.ChainAnchorCensusMS
	}
	// LEVELMERGE-1 件2 (方案 P, 2026-07-18): the gated-share split family
	// travels as ONE disclosure the same way (values + marker + pointer
	// roster + fail-open overlap together; a survivor with its own split
	// keeps it — the two halves of one split never R1-merge because their
	// published values differ by construction).
	if survivor.GatedShareFullMS == 0 && loser.GatedShareFullMS > 0 {
		survivor.GatedShareClaimedMS = loser.GatedShareClaimedMS
		survivor.GatedShareFullMS = loser.GatedShareFullMS
		survivor.GatedShareConstituentSeat = loser.GatedShareConstituentSeat
		survivor.GatedShareClaimSeats = loser.GatedShareClaimSeats
	}
	if survivor.GatedShareOverlapDisclosureMS == 0 && loser.GatedShareOverlapDisclosureMS > 0 {
		survivor.GatedShareOverlapDisclosureMS = loser.GatedShareOverlapDisclosureMS
		if len(survivor.GatedShareClaimSeats) == 0 {
			survivor.GatedShareClaimSeats = loser.GatedShareClaimSeats
		}
	}
	// PARTSPLIT-1 (§29.150④): the R4-mirror refusal record travels as ONE
	// disclosure the same way (all four fields together — a survivor with
	// its own record keeps it; the 行2 sub-line re-validates X+Y against the
	// merged row's own runnable account before rendering, so an inherited
	// record on a diverged-value survivor silently never renders instead of
	// lying, 宁漏勿假指).
	if survivor.GatedCompositeEdgeAnchorTS == 0 && loser.GatedCompositeEdgeAnchorTS > 0 {
		survivor.GatedCompositeEdgePreShareMS = loser.GatedCompositeEdgePreShareMS
		survivor.GatedCompositeEdgePostShareMS = loser.GatedCompositeEdgePostShareMS
		survivor.GatedCompositeEdgeAnchorTS = loser.GatedCompositeEdgeAnchorTS
		survivor.GatedCompositeEdgeAnchorVia = loser.GatedCompositeEdgeAnchorVia
	}
	// RNB-1 R4 / XLANE-1 件1 markers — XLANE-2 件3 narrowing (E11 rider,
	// §29.109 记录①; §29.104.2 定谳⑤族, 2026-07-17): both whole-seat demotion
	// markers speak a CHANNEL story — "this seat rides the ◇ adjacent lane"
	// (R4: cannot show credential / represented: credential held elsewhere).
	// That story is per-VIEW epistemics, not fact ontology: a survivor whose
	// own face IS the chain lane carries a positive on-chain admission proof
	// (engine discipline pairs every demotion with ChainRelevance="adjacent"),
	// and the former unconditional OR minted the three-face contradiction on
	// ONE row — ❶ badge + ├─链上─ lane + 根因排序#N seat + the
	// 「无链上凭证(整席降道)」 sentence (the fused E11 witness). 诚实面胜出:
	// absence-of-proof on a merged view never overrides an EXPLICIT surviving
	// chain admission (ChainRelevance=="on_chain" — the precise positive
	// counterpart of the demotion's paired "adjacent"); every other survivor
	// (◇/▒/legacy bare "") keeps the OR-monotone inheritance so the demotion
	// story is never dropped wordlessly (XLANE-1 P2-① pin). No arm ever
	// fabricates a credential — the survivor's own markers and every value
	// channel stay untouched either way. The chain face keeps the QUIET
	// account-identity memory instead (AbsorbedWholeSeatDemotedView): the
	// ×N same-kind fold key must keep forking exactly as it did when the
	// marker itself crossed, or same-(subject,object) OVERLAPPING accounts
	// re-Σ into a false family total (the fused donghu 低频运行 32.877
	// witness) — word faces never read the memory.
	if strings.TrimSpace(survivor.ChainRelevance) != "on_chain" {
		survivor.ChainCredentialLaneDemoted = survivor.ChainCredentialLaneDemoted || loser.ChainCredentialLaneDemoted
		survivor.ChainAnchorRepresentedByChainSeat = survivor.ChainAnchorRepresentedByChainSeat || loser.ChainAnchorRepresentedByChainSeat
	} else if loser.ChainCredentialLaneDemoted || loser.ChainAnchorRepresentedByChainSeat {
		survivor.AbsorbedWholeSeatDemotedView = true
	}
	survivor.AbsorbedWholeSeatDemotedView = survivor.AbsorbedWholeSeatDemotedView || loser.AbsorbedWholeSeatDemotedView
	survivor.ResourceCompletionClosure = survivor.ResourceCompletionClosure || loser.ResourceCompletionClosure
	if survivor.BlockedReasonCaller == "" {
		survivor.BlockedReasonCaller = loser.BlockedReasonCaller
	}
	if survivor.BlockedReasonWindowCount == 0 && loser.BlockedReasonWindowCount > 0 {
		survivor.BlockedReasonWindowCount = loser.BlockedReasonWindowCount
		survivor.BlockedReasonWindowCaller = loser.BlockedReasonWindowCaller
	}
	// VS-1 F6(b) (adversarial review 2026-07-04): a periodic-source survivor's
	// EffectiveImpactMS is the AUTHORITATIVE discounted attribution even at
	// exactly 0 (pure in-period cadence) — the merged twin is the raw-lane
	// view of the same fact, and backfilling its positive value would
	// resurrect the very sleep the discount removed. The survivor's periodic
	// triple (PeriodicSource/DetectedPeriodMS/PeriodicLatenessMS) likewise
	// stays exactly as the survivor published it (it won the priority scan);
	// a loser's periodic fields are never copied over. Precise boolean gate.
	if !survivor.PeriodicSource && survivor.EffectiveImpactMS <= 0 {
		survivor.EffectiveImpactMS = loser.EffectiveImpactMS
		// EPUB (§29.31): the published marker follows the fact, OR-monotone —
		// a positive adopted value is always a published one (decode invariant:
		// value>0 ⇒ note present), and when both views sit at 0 either view's
		// authoritative published-0 keeps the merged row published (never
		// down-graded to "unpublished" by merging with a silent twin).
		survivor.EffectiveImpactPublished = survivor.EffectiveImpactPublished || loser.EffectiveImpactPublished
	}
	if survivor.ActualImpactMS <= 0 {
		survivor.ActualImpactMS = loser.ActualImpactMS
		// CR-2 组③ P7: the actual channel's physical interval travels WITH the
		// value it bounds (one fact, one carrier pair) — a survivor adopting
		// the loser's actual must adopt its interval, or the ⚠ containment
		// gate would judge the value against a foreign/absent window.
		if survivor.ActualWindowStartTs <= 0 && survivor.ActualWindowEndTs <= 0 {
			survivor.ActualWindowStartTs = loser.ActualWindowStartTs
			survivor.ActualWindowEndTs = loser.ActualWindowEndTs
		}
	}
	if survivor.UndrillableReason == "" {
		survivor.UndrillableReason = loser.UndrillableReason
	}
	// XLANE-2 件2: the self-gap semantic-overlap disclosure travels with the
	// fact (empty-slot fill; a survivor with its own roster keeps it —
	// priority-scan doctrine, and the engine stamps at most one seat).
	if len(survivor.SelfGapSemanticOverlaps) == 0 {
		survivor.SelfGapSemanticOverlaps = loser.SelfGapSemanticOverlaps
	}
	// RANK-U Stage 2 (donghu W1 witness, 2026-07-13): the BOARD SEAT identity
	// follows the fact. The self-basis semantic family publishes TWO views of
	// one fact — the span-family record (no rank notes) and the rank record
	// (根因排序#N) — with identical subject/value/line envelope since SELF-SEM;
	// the span view scans first (wakeup_chain view order) and used to absorb
	// the rank view WITHOUT its seat: the engine seated the row #2 while every
	// display face spoke the mention floor 「未入根因排序前N」 (same-page
	// contradiction, §29.30.1 family). An unseated survivor adopts the seated
	// loser's ordinal with its ladder tier as ONE pair; a seated survivor
	// keeps its own (priority-scan doctrine). The non-chain composite-board
	// seat (BackgroundRank) travels the same way. Typed fields only.
	//
	// XLANE-2 件3 (E11 rider, 2026-07-17): a WHOLE-SEAT DEMOTED view's ordinal
	// is a ◇ demotion-domain artifact — its entire channel story is "this
	// account rides ◇" (no credential / represented elsewhere), so an
	// explicit chain-face survivor must not adopt it as a 根因排序#N seat
	// (the E11 three-face family with the sentence arm swapped for an
	// ordinal arm; same 诚实面胜出 arbitration as the marker guard above).
	// Ordinary (undemoted) seated views keep the legacy adoption in full —
	// the RANK-U seat-follows-the-fact doctrine and the XLANE-3 fused
	// cross-step forms depend on it (refusing there would silently drop a
	// board seat from display: the ELIM-GAP disappearance family).
	loserWholeSeatDemoted := loser.ChainCredentialLaneDemoted || loser.ChainAnchorRepresentedByChainSeat
	if survivor.Rank == 0 && loser.Rank > 0 &&
		!(loserWholeSeatDemoted && strings.TrimSpace(survivor.ChainRelevance) == "on_chain") {
		survivor.Rank = loser.Rank
		if strings.TrimSpace(survivor.Tier) == "" {
			survivor.Tier = loser.Tier
		}
		// XLANE-3 件1: the ordinal's BOARD identity travels with the adopted
		// seat (same donor discipline as the RankQueryWindow pair — an ordinal
		// without its board is exactly the bare-#N collision being fixed). A
		// seated survivor keeps its own board fields untouched.
		if survivor.RankBoardTarget == "" {
			survivor.RankBoardTarget = loser.RankBoardTarget
		}
		if survivor.RankBoardParamsFingerprint == "" {
			survivor.RankBoardParamsFingerprint = loser.RankBoardParamsFingerprint
		}
	}
	if survivor.BackgroundRank == 0 && loser.BackgroundRank > 0 {
		survivor.BackgroundRank = loser.BackgroundRank
		if strings.TrimSpace(survivor.Tier) == "" {
			survivor.Tier = loser.Tier
		}
	}
	if survivor.ChainDepth <= 0 {
		survivor.ChainDepth = loser.ChainDepth
	}
	// The dual-view case (a root_cause_primary row carrying the chain-cumulative
	// value merged with its per-hop twin): cumulative is the larger scope, keep
	// the max — both describe the same fact, max never invents a number.
	//
	// RSPA-HYG 件① (§29.77 立案①, 2026-07-14): EXCEPT onto a re-anchored
	// survivor. A ⛓ clipped (or ◇ remainder) seat publishes the anchored /
	// remainder account on its value channels BY CONSTRUCTION, and an
	// undecomposed same-fact mirror's cumulative IS the retired full-window
	// claim (== ChainAnchorFullMS, which already travels typed on the
	// survivor) — lifting it would republish on the cumulative channel the
	// very claim the migration retired. Caught by the synthetic same-line-
	// range exchange fixture (TestRSPAHygD2SameLineRangeExchangeSurvivorIs-
	// ClippedSeat); the production donghu/tieba twins publish over different
	// line ranges and never reached this arm.
	if survivor.ChainAnchorFullMS == 0 && loser.CumulativeImpactMS > survivor.CumulativeImpactMS {
		survivor.CumulativeImpactMS = loser.CumulativeImpactMS
	}
	// COV §24.9 D-1: TargetImpactMS follows the same one-fact MAX discipline —
	// both views explain the SAME stretch of the target's blocked clock, and a
	// survivor-only inheritance would be view-order dependent (D-3 家族).
	if loser.TargetImpactMS > survivor.TargetImpactMS {
		survivor.TargetImpactMS = loser.TargetImpactMS
	}
	if survivor.Confidence <= 0 {
		survivor.Confidence = loser.Confidence
	}
	if survivor.TypeToken == "" {
		survivor.TypeToken = loser.TypeToken
	}
	// PTV5 Q4 (#68 用户裁定 2026-07-05): inversion candidacy is a property of
	// the ONE fact — either view observing it marks the merged row.
	if loser.PriorityInversionCandidate {
		survivor.PriorityInversionCandidate = true
	}
	// ENG-2 (复核冷读 CP1-③, 2026-07-12): an idle-cadence loser (the P9
	// arm-c pacing_idle / periodic_idle typed lanes) annotates the surviving
	// seat with its value + kind instead of vanishing into SecondaryObjects.
	// One-fact MAX (the rank view and the root_evidence witness of the SAME
	// idle segment both fold here — never summed); kind first-wins.
	if idleMS, idleKind := traceCausalProjectionIdleCadence(loser); idleMS > 0 {
		if survivor.IdleCadenceKind == "" {
			survivor.IdleCadenceKind = idleKind
		}
		if idleMS > survivor.IdleCadenceMS {
			survivor.IdleCadenceMS = idleMS
		}
	}
	// SFD (§15.A display half, user q6 issue 1): the SupplyFold arm — guards
	// and conflict memory live in the helper (SFD 复核 F1/F4).
	traceCausalProjectionAbsorbSupplyFold(survivor, loser, foldBackfill)
}

// traceCausalProjectionAppendMergedSubject records one merged member's thread
// subject on the aggregate row (display roster for the fold/×N line): distinct
// by canonical key, real thread names only (the unknown sentinel and empty
// subjects carry no display value), capped at traceCausalProjectionMergedSubjectCap.
func traceCausalProjectionAppendMergedSubject(aggregate *TraceCausalProjectionNode, subject string) {
	subject = strings.TrimSpace(subject)
	if subject == "" || !traceCausalProjectionKnownSubject(subject) {
		return
	}
	if len(aggregate.MergedSubjects) >= traceCausalProjectionMergedSubjectCap {
		return
	}
	key := traceCausalProjectionCanonicalNode(subject)
	for _, existing := range aggregate.MergedSubjects {
		if traceCausalProjectionCanonicalNode(existing) == key {
			return
		}
	}
	aggregate.MergedSubjects = append(aggregate.MergedSubjects, subject)
}

func traceCausalProjectionAppendSecondaryObject(survivor *TraceCausalProjectionNode, object string) {
	object = strings.TrimSpace(object)
	if object == "" || len(survivor.SecondaryObjects) >= traceCausalProjectionSecondaryObjectCap {
		return
	}
	key := traceCausalProjectionCanonicalNode(object)
	if key == traceCausalProjectionCanonicalNode(survivor.Object) {
		return
	}
	for _, existing := range survivor.SecondaryObjects {
		if traceCausalProjectionCanonicalNode(existing) == key {
			return
		}
	}
	survivor.SecondaryObjects = append(survivor.SecondaryObjects, object)
}

// --- R4: peer-alias merge (customer audit 2026-07-03, H18) --------------------

// traceCausalProjectionMergePeerAliases folds the readfile_de E1/E2 shape: the
// SAME contention observed twice with the lock owner written two ways — once as
// a resolved thread label ("NetworkKit_AssetsUtil_Operate_0-42067") and once as
// the raw "pid=42067" handle. Two rows in one bucket merge when ALL of:
//   - canonical subject equal (same blocked thread),
//   - canonical TypeToken equal (same producer kind token),
//   - one row's BlockingPeer is the literal pid=N form (character-class check)
//     and the other's peer name carries the SAME integer N as its -pid tail
//     (integer equality, never substring),
//   - the two rows' own time spans overlap (boolean intersection; both spans
//     must be valid).
//
// The NAMED variant survives; the projected impact keeps the LARGER of the two
// measurements (both describe one wait — max never invents a number); evidence
// ids union losslessly.
func traceCausalProjectionMergePeerAliases(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	if len(nodes) < 2 {
		return nodes
	}
	dropped := map[int]bool{}
	for i := 0; i < len(nodes); i++ {
		if dropped[i] {
			continue
		}
		for j := i + 1; j < len(nodes); j++ {
			if dropped[j] {
				continue
			}
			named, pidVariant, ok := traceCausalProjectionPeerAliasPair(&nodes[i], &nodes[j])
			if !ok {
				continue
			}
			traceCausalProjectionAbsorbPeerAlias(named, *pidVariant)
			if pidVariant == &nodes[i] {
				dropped[i] = true
			} else {
				dropped[j] = true
			}
			if dropped[i] {
				break
			}
		}
	}
	if len(dropped) == 0 {
		return nodes
	}
	out := make([]TraceCausalProjectionNode, 0, len(nodes)-len(dropped))
	for i, node := range nodes {
		if dropped[i] {
			continue
		}
		out = append(out, node)
	}
	return out
}

func traceCausalProjectionPeerAliasPair(a, b *TraceCausalProjectionNode) (named, pidVariant *TraceCausalProjectionNode, ok bool) {
	if traceCausalProjectionCanonicalNode(a.Subject) != traceCausalProjectionCanonicalNode(b.Subject) {
		return nil, nil, false
	}
	if traceCausalProjectionCanonicalNode(a.TypeToken) != traceCausalProjectionCanonicalNode(b.TypeToken) {
		return nil, nil, false
	}
	if !traceCausalProjectionSpansOverlap(*a, *b) {
		return nil, nil, false
	}
	if pid, isPid := traceCausalProjectionPidPeerForm(b.BlockingPeer); isPid {
		if n, hasTail := traceCausalProjectionNamePidTail(a.BlockingPeer); hasTail && n == pid {
			return a, b, true
		}
	}
	if pid, isPid := traceCausalProjectionPidPeerForm(a.BlockingPeer); isPid {
		if n, hasTail := traceCausalProjectionNamePidTail(b.BlockingPeer); hasTail && n == pid {
			return b, a, true
		}
	}
	return nil, nil, false
}

// traceCausalProjectionSpansOverlap is the boolean time-span intersection;
// both nodes must expose a valid span of their own. Delegates to the shared
// interval authority (trace_causal_projection_interval.go, §11-N2) — same
// guard, same strict inequalities, one home.
func traceCausalProjectionSpansOverlap(a, b TraceCausalProjectionNode) bool {
	return TraceCausalProjectionIntervalsOverlap(a.StartTs, a.EndTs, b.StartTs, b.EndTs)
}

// traceCausalProjectionPidPeerForm matches the literal "pid=N" peer handle
// (character-class check: the fixed prefix plus pure digits).
func traceCausalProjectionPidPeerForm(peer string) (int, bool) {
	peer = strings.TrimSpace(peer)
	if !strings.HasPrefix(peer, "pid=") {
		return 0, false
	}
	return traceCausalProjectionPureInt(strings.TrimPrefix(peer, "pid="))
}

// traceCausalProjectionNamePidTail extracts the integer -pid tail of a named
// thread label (non-empty name part, pure-digit tail after the last '-').
func traceCausalProjectionNamePidTail(peer string) (int, bool) {
	peer = strings.TrimSpace(peer)
	idx := strings.LastIndex(peer, "-")
	if idx <= 0 || idx == len(peer)-1 {
		return 0, false
	}
	return traceCausalProjectionPureInt(peer[idx+1:])
}

func traceCausalProjectionPureInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func traceCausalProjectionAbsorbPeerAlias(named *TraceCausalProjectionNode, pidVariant TraceCausalProjectionNode) {
	if pidVariant.ImpactMS > named.ImpactMS {
		named.ImpactMS = pidVariant.ImpactMS
	}
	if pidVariant.CumulativeImpactMS > named.CumulativeImpactMS {
		named.CumulativeImpactMS = pidVariant.CumulativeImpactMS
	}
	// COV §24.9 D-1: same one-fact MAX discipline (see the R1 absorb).
	if pidVariant.TargetImpactMS > named.TargetImpactMS {
		named.TargetImpactMS = pidVariant.TargetImpactMS
	}
	absorbed := map[string]bool{traceCausalProjectionCanonicalNode(named.EvidenceID): true}
	for _, id := range named.MergedEvidenceIDs {
		absorbed[traceCausalProjectionCanonicalNode(id)] = true
	}
	appendID := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
			return
		}
		absorbed[traceCausalProjectionCanonicalNode(raw)] = true
		named.MergedEvidenceIDs = append(named.MergedEvidenceIDs, raw)
	}
	appendID(pidVariant.EvidenceID)
	for _, id := range pidVariant.MergedEvidenceIDs {
		appendID(id)
	}
}

// --- V4: duplicate-publication dedup (pre-R2) ---------------------------------

// traceCausalProjectionDedupDuplicatePublications folds duplicate publications
// of ONE measurement inside a bucket (V4, customer revisit 2026-07-03): rows
// with the same canonical subject + object + TypeToken, matching positive
// projected ms AND a precise line-range or time-span overlap describe one
// wall-clock fact republished N times. Two value lanes:
//   - exact lane (original V4): pure float equality; the first occurrence
//     survives with the value UNCHANGED — no field beyond the publication
//     count and the evidence union is touched;
//   - near lane (PTV6 批② #4, 2026-07-06): values inside the ≤3% band
//     (traceCausalProjectionDuplicatePublicationNearTolerance), and ONLY when
//     the shared Object is a real identity (never the unknown-thread sentinel
//     or empty), are the SAME measurement republished with a refined boundary;
//     the survivor lifts ImpactMS/CumulativeImpactMS to the member MAX — the
//     widest boundary estimate of the one fact; max never invents a number,
//     while letting the drifted copies reach R2 summed a single-thread 1.383ms
//     wait into a 4.119ms/138%-of-window phantom.
//
// DuplicatePublications counts the publications and evidence ids union
// losslessly. MergedCount is never touched — its ×N carries SUM semantics for
// genuinely distinct instances (near-value NON-overlapping bursts stay
// separate and may legitimately R2-SUM). Value proximity alone never folds;
// upstream ×N sum aggregates and same-EvidenceID copies are never folded.
func traceCausalProjectionDedupDuplicatePublications(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	if len(nodes) < 2 {
		return nodes
	}
	dropped := map[int]bool{}
	folded := false
	for i := 0; i < len(nodes); i++ {
		if dropped[i] || nodes[i].MergedCount > 1 {
			continue
		}
		for j := i + 1; j < len(nodes); j++ {
			if dropped[j] || nodes[j].MergedCount > 1 {
				continue
			}
			if !traceCausalProjectionSameDuplicatePublication(nodes[i], nodes[j]) {
				continue
			}
			traceCausalProjectionAbsorbDuplicatePublication(&nodes[i], nodes[j])
			dropped[j] = true
			folded = true
		}
	}
	if !folded {
		return nodes
	}
	out := make([]TraceCausalProjectionNode, 0, len(nodes)-len(dropped))
	for i, node := range nodes {
		if dropped[i] {
			continue
		}
		out = append(out, node)
	}
	return out
}

// traceCausalProjectionSameDuplicatePublication is the strict identity of one
// republished measurement — the types-layer home of the identity the renderer's
// H6 display fold pioneered. The tool-layer safety-net isomorph
// (runtimeTraceProjSameAdjacentMeasurement) mirrors BOTH value lanes since
// PTV6-B: it consumes the exported band/gate authorities below
// (TraceCausalProjectionNearDuplicateValues / TraceCausalProjectionKnownSubject)
// — the former "near lane lives here only" fork is gone, and the band constant
// still has exactly one home.
func traceCausalProjectionSameDuplicatePublication(a, b TraceCausalProjectionNode) bool {
	if traceCausalProjectionCanonicalNode(a.EvidenceID) != "" &&
		traceCausalProjectionCanonicalNode(a.EvidenceID) == traceCausalProjectionCanonicalNode(b.EvidenceID) {
		// The same observation's own copy — renderers dedupe by node key; a fold
		// here would fabricate a publication count.
		return false
	}
	if traceCausalProjectionCanonicalNode(a.Subject) != traceCausalProjectionCanonicalNode(b.Subject) ||
		traceCausalProjectionCanonicalNode(a.Object) != traceCausalProjectionCanonicalNode(b.Object) {
		return false
	}
	sameValue := a.ImpactMS > 0 && a.ImpactMS == b.ImpactMS
	if tokenA, tokenB := traceCausalProjectionCanonicalNode(a.TypeToken),
		traceCausalProjectionCanonicalNode(b.TypeToken); tokenA != tokenB {
		// WO-D3 根修臂 (SMR-1 批 S3-TPF, smr_audit_report §②, 2026-07-12): the
		// typed-token completeness fork — one lane publishes the observation
		// WITH its typed token, the other lane re-publishes the same
		// measurement token-less (42729 E9/E15、62930 E9/E19: 13.418 ×3 twice,
		// word faces 「对端线程未解析·iowait」 vs 「D-state/iowait(对端未解析)」
		// forked purely by token completeness). EXACT lane only, and only the
		// single-side-absence shape: exact float value equality + same subject
		// + the same SENTINEL object + one side token-absent = one measurement
		// (去重先于合并 — folding here, pre-R2, is the root fix; a typed FAMILY
		// key merge is FORBIDDEN — vnote 实证: ×N is a SUM, a family-key merge
		// would double-book the twin publications into ×6). Two DIFFERENT
		// non-empty tokens stay two accounts; the 9µs near-lane strict ruling
		// (sentinel exclusion) is untouched — this arm never enters the near
		// lane. v5 P1 件① 盘点 (2026-07-13): this PRE-R2 relaxation is NOT
		// dead code — it converges the single-row pairs whose lanes carve the
		// SAME span; pairs whose lanes carve DIFFERENT envelopes escape it,
		// R2-merge per lane, and converge post-R2 at engine arm C
		// (traceCausalProjectionConvergeMergedTwinSeats) — two positions of
		// ONE convergence doctrine, never two taxonomies.
		oneSideAbsent := (tokenA == "") != (tokenB == "")
		sentinelObject := !traceCausalProjectionKnownSubject(a.Object)
		if !(oneSideAbsent && sentinelObject && sameValue) {
			return false
		}
	}
	// Near lane (PTV6 批② #4): the ≤3% band additionally requires the shared
	// Object to be a REAL identity — the unknown-thread/unknown sentinel and
	// empty objects are excluded through the same precise helper R3 keys on.
	// An approximate merge asserts "one republished measurement", and that
	// assertion leans on the object identity; a sentinel object carries none
	// (user-adjudicated strict pin: two same-subject critical_blocking waits on
	// UNRESOLVED peers, 112.223 vs 112.214ms — 9µs apart, overlapping enclosing
	// ranges — are DISTINCT facts and must never merge). When the identity is
	// indeterminate the fold fails open to separate rows, exactly like the
	// RF2a location rule.
	// [Med 修正轮 2026-07-06] the sentinel gate covers BOTH identity legs: the
	// "one republished measurement" assertion leans on the whole
	// (subject, object) identity — an unknown-thread SUBJECT carries none
	// either (canonical subjects are already equal here, so one side's check
	// covers the pair).
	nearValue := !sameValue && traceCausalProjectionKnownSubject(a.Subject) &&
		traceCausalProjectionKnownSubject(a.Object) &&
		traceCausalProjectionNearDuplicateValues(a.ImpactMS, b.ImpactMS)
	return (sameValue || nearValue) &&
		(traceCausalProjectionLineSpansOverlap(a, b) || traceCausalProjectionSpansOverlap(a, b))
}

// traceCausalProjectionNearDuplicateValues reports whether two positive
// projected values sit inside the near-duplicate band (PTV6 批② #4): relative
// difference against the LARGER value ≤ 3%. Only ever consulted behind the
// full V4 identity (with a real, non-sentinel object) + overlap gate;
// proximity alone never folds.
//
// DRIFTGUARD (RULE3-1 件12③, §29.185① maintain ruling, 2026-07-21; audit
// G4): the fold key is the PRECISE V4 identity (typed subject/object/window
// + real overlap — 双真身份门); the 3% band only tolerates boundary-resample
// jitter of ONE fact's value, never folds two different facts. Softening or
// widening this band re-opens the PTV6 批②#4 adjudicated 138% phantom
// (double-published copies summing past their own account) — 禁重诉区. The survivor's value may have been lifted by an
// earlier near fold, so a later candidate is compared against the lifted value
// — drift stays bounded per step by the band and every step still requires
// overlap with the survivor.
func traceCausalProjectionNearDuplicateValues(a, b float64) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	hi, lo := a, b
	if hi < lo {
		hi, lo = lo, hi
	}
	return (hi-lo)/hi <= traceCausalProjectionDuplicatePublicationNearTolerance
}

// TraceCausalProjectionNearDuplicateValues is the exported single authority of
// the V4 near-duplicate value band (PTV6 批② #4;
// TraceCausalProjectionSameWindowToleranceS 先例): the display-layer safety-net
// fold (runtimeTraceProjSameAdjacentMeasurement, internal/tool) consumes THIS
// function — the ≤3% band lives here once and is never copied.
func TraceCausalProjectionNearDuplicateValues(a, b float64) bool {
	return traceCausalProjectionNearDuplicateValues(a, b)
}

// TraceCausalProjectionKnownSubject exports the R3 sentinel gate for the same
// display-layer mirror: a near fold asserts "one republished measurement", and
// that assertion leans on a REAL (non-sentinel, non-empty) object identity —
// the identical gate the types-layer near lane reads.
func TraceCausalProjectionKnownSubject(subject string) bool {
	return traceCausalProjectionKnownSubject(subject)
}

// traceCausalProjectionLineSpansOverlap is the boolean line-range intersection;
// both nodes must expose a valid range of their own (same guard style as the
// time-span twin traceCausalProjectionSpansOverlap).
func traceCausalProjectionLineSpansOverlap(a, b TraceCausalProjectionNode) bool {
	if a.LineStart <= 0 || a.LineEnd < a.LineStart || b.LineStart <= 0 || b.LineEnd < b.LineStart {
		return false
	}
	return a.LineStart <= b.LineEnd && b.LineStart <= a.LineEnd
}

func traceCausalProjectionAbsorbDuplicatePublication(survivor *TraceCausalProjectionNode, dup TraceCausalProjectionNode) {
	// Near lane only (PTV6 批② #4): when the two publications' values differ
	// (inside the ≤3% band, or the identity would not have matched), the fold
	// keeps the LARGEST boundary estimate of the one fact — ImpactMS and
	// CumulativeImpactMS lift to the pairwise max. The exact lane
	// (bit-equal ImpactMS) takes neither branch below and stays byte-identical
	// to pre-PTV6 behavior: publication count + evidence union only.
	if dup.ImpactMS != survivor.ImpactMS {
		if dup.ImpactMS > survivor.ImpactMS {
			survivor.ImpactMS = dup.ImpactMS
		}
		if dup.CumulativeImpactMS > survivor.CumulativeImpactMS {
			survivor.CumulativeImpactMS = dup.CumulativeImpactMS
		}
		// COV §24.9 D-1: same one-fact MAX discipline (see the R1 absorb).
		if dup.TargetImpactMS > survivor.TargetImpactMS {
			survivor.TargetImpactMS = dup.TargetImpactMS
		}
	}
	if survivor.DuplicatePublications < 1 {
		survivor.DuplicatePublications = 1
	}
	add := dup.DuplicatePublications
	if add < 1 {
		add = 1
	}
	survivor.DuplicatePublications += add
	absorbed := map[string]bool{traceCausalProjectionCanonicalNode(survivor.EvidenceID): true}
	for _, id := range survivor.MergedEvidenceIDs {
		absorbed[traceCausalProjectionCanonicalNode(id)] = true
	}
	appendID := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
			return
		}
		absorbed[traceCausalProjectionCanonicalNode(raw)] = true
		survivor.MergedEvidenceIDs = append(survivor.MergedEvidenceIDs, raw)
	}
	appendID(dup.EvidenceID)
	for _, id := range dup.MergedEvidenceIDs {
		appendID(id)
	}
	// WO-D3 根修臂 (SMR-1 批 S3-TPF, 2026-07-12): when the exact-lane
	// single-side token-absence relaxation folded the pair, the survivor keeps
	// the RICHER identity (词位取最高车道 — the typed token and its state word
	// must not be lost to scan order; the token-less copy carried none).
	if strings.TrimSpace(survivor.TypeToken) == "" && strings.TrimSpace(dup.TypeToken) != "" {
		survivor.TypeToken = dup.TypeToken
		if strings.TrimSpace(survivor.StateKind) == "" {
			survivor.StateKind = dup.StateKind
		}
	}
}

// --- WO-G2: zero-value instant-marker fold ----------------------------------

// traceCausalProjectionZeroValueMarkerInstantToleranceS is the µs-scale
// containment tolerance of the WO-G2 marker fold (seconds): the wake-instant
// marker's timestamp must sit inside (or on the boundary of) the valued row's
// own occurrence segment. 2µs — same magnitude class as the µs-precision
// timestamps themselves, far below any segment length that mints a row.
const traceCausalProjectionZeroValueMarkerInstantToleranceS = 0.000002

// traceCausalProjectionZeroValueMarkerRow reports whether node is a zero-value
// INSTANT marker (WO-G2, SMR-S12b — 42728 E3/E4、E9/E10 witnesses): a
// wake-instant record (StartTs == EndTs, e.g. sched_blocked_reason markers)
// carrying no measurable account of its own. trace_gap diagnostics are NOT
// markers (they carry their own ◌ seat semantics — WO-G1's lane); merged /
// fold / semantic rows never qualify.
func traceCausalProjectionZeroValueMarkerRow(node TraceCausalProjectionNode) bool {
	if node.ImpactMS > 0 || node.CumulativeImpactMS > 0 ||
		node.EffectiveImpactMS > 0 || node.ActualImpactMS > 0 {
		return false
	}
	if node.MergedCount > 1 || node.DuplicatePublications > 1 ||
		node.OnChainOverflowFold || node.FamilyMemberCount > 0 ||
		strings.TrimSpace(node.SemanticClass) != "" {
		return false
	}
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		if traceCausalProjectionCanonicalNode(token) == "trace_gap" {
			return false
		}
	}
	// §29.183 G8 (start arm only): a marker AT ts=0 exactly is a legal instant
	// in a rebased trace; the zero-length instant shape (StartTs==EndTs) is
	// this predicate's whole point, so the shared window predicate does not
	// apply — only the start-negativity arm changes. The (0,0) absence pair is
	// carved out explicitly (no timestamp = no containment proof).
	if node.StartTs < 0 || node.EndTs < node.StartTs || node.EndTs <= 0 {
		return false
	}
	return node.EndTs-node.StartTs <= traceCausalProjectionZeroValueMarkerInstantToleranceS
}

// traceCausalProjectionFoldZeroValueMarkerRows (WO-G2, SMR-1 批 SMR-S12b,
// smr_audit_report §②, 2026-07-12; 42728 witness: E3(+1) 0.091ms iowait valued
// row beside E4 0.000ms same-subject same-object marker seat — the zero-value
// seat implies yet another wait account): a zero-value instant marker whose
// timestamp sits INSIDE exactly one same-(canonical subject, canonical object)
// valued row's own occurrence segment is the wake-instant marker of THAT
// segment, not a second account. It folds into the valued row as a VALUELESS
// member — the §29.13 无时长值成员披露 lane (target form = the same page's
// E8(+2) 「×3(有值2项…,1项无时长值)」precedent): MergedValuelessCount counts
// it, its E# joins the bracket, and NO ms value moves (吸收=佩记号,数值不重计).
// 禁裸删席 (zero-silent-disappearance): the fold is the only removal path —
// evidence stays registered through MergedEvidenceIDs. Ambiguity (0 or ≥2
// enclosing valued rows), missing timestamps, or an incompatible query window
// fail open to the two-seat render. Mechanism kinship: CR-3 P10
// blocked_reason 消费义务 + §29.13 — an existing lane reaching the marker
// shape, never a second mechanism.
func traceCausalProjectionFoldZeroValueMarkerRows(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	if len(nodes) < 2 {
		return nodes
	}
	dropped := map[int]bool{}
	for i := range nodes {
		if !traceCausalProjectionZeroValueMarkerRow(nodes[i]) {
			continue
		}
		marker := nodes[i]
		subject := traceCausalProjectionCanonicalNode(marker.Subject)
		object := traceCausalProjectionCanonicalNode(marker.Object)
		if subject == "" {
			continue
		}
		host := -1
		ambiguous := false
		for j := range nodes {
			if j == i || dropped[j] {
				continue
			}
			candidate := nodes[j]
			if candidate.ImpactMS <= 0 && candidate.CumulativeImpactMS <= 0 {
				continue // valueless hosts never absorb (no account to disclose on)
			}
			if traceCausalProjectionCanonicalNode(candidate.Subject) != subject ||
				traceCausalProjectionCanonicalNode(candidate.Object) != object {
				continue
			}
			if !TraceCausalProjectionWindowPresent(candidate.StartTs, candidate.EndTs) {
				continue // no occurrence segment = no containment proof (fail-open)
			}
			tol := traceCausalProjectionZeroValueMarkerInstantToleranceS
			if marker.StartTs < candidate.StartTs-tol || marker.StartTs > candidate.EndTs+tol {
				continue
			}
			// Cross-window re-measurements never fold (SFD F1 family veto).
			if traceCausalProjectionIntervalValid(marker.QueryWindowStartTs, marker.QueryWindowEndTs) &&
				traceCausalProjectionIntervalValid(candidate.QueryWindowStartTs, candidate.QueryWindowEndTs) &&
				(math.Abs(marker.QueryWindowStartTs-candidate.QueryWindowStartTs) > TraceCausalProjectionSameWindowToleranceS ||
					math.Abs(marker.QueryWindowEndTs-candidate.QueryWindowEndTs) > TraceCausalProjectionSameWindowToleranceS) {
				continue
			}
			if host >= 0 {
				ambiguous = true
				break
			}
			host = j
		}
		if host < 0 || ambiguous {
			continue // fail-open: the honest two-seat render beats a guessed fold
		}
		h := &nodes[host]
		display := h.ImpactMS
		if display <= 0 {
			display = h.CumulativeImpactMS
		}
		if h.MergedCount <= 1 {
			h.MergedCount = 2
			// Min/max range over POSITIVE displays only (G12-ENG §29.1) — the
			// single valued member is both extrema.
			h.MergedMinMS = display
			h.MergedMaxMS = display
		} else {
			h.MergedCount++
		}
		h.MergedValuelessCount++
		absorbed := map[string]bool{traceCausalProjectionCanonicalNode(h.EvidenceID): true}
		for _, id := range h.MergedEvidenceIDs {
			absorbed[traceCausalProjectionCanonicalNode(id)] = true
		}
		appendID := func(raw string) {
			raw = strings.TrimSpace(raw)
			if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
				return
			}
			absorbed[traceCausalProjectionCanonicalNode(raw)] = true
			h.MergedEvidenceIDs = append(h.MergedEvidenceIDs, raw)
		}
		appendID(marker.EvidenceID)
		for _, id := range marker.MergedEvidenceIDs {
			appendID(id)
		}
		if marker.LineStart > 0 && (h.LineStart <= 0 || marker.LineStart < h.LineStart) {
			h.LineStart = marker.LineStart
		}
		if marker.LineEnd > h.LineEnd {
			h.LineEnd = marker.LineEnd
		}
		dropped[i] = true
	}
	if len(dropped) == 0 {
		return nodes
	}
	out := make([]TraceCausalProjectionNode, 0, len(nodes)-len(dropped))
	for i, node := range nodes {
		if dropped[i] {
			continue
		}
		out = append(out, node)
	}
	return out
}

// --- R2: same-kind ×N aggregation -------------------------------------------

// traceCausalProjectionAggregateSameKind collapses ≥3 rows with exactly the
// same (subject, object) inside ONE bucket into a single ×N row carrying the
// SUM, the per-instance min–max range, and every instance's evidence id.
// Cross-bucket copies stay consistent because each bucket aggregates the same
// member set to the same lead EvidenceID (renderers dedupe by node key).
//
// §11-N2 value-caliber exception (real_trace_campaign_20260705.md, q2
// specimen E10): when the merged members come from DISTINCT query windows
// (typed QueryWindow identity) AND members of different windows have
// overlapping occurrence intervals, the same physical wall-clock segment was
// carved once per window and a plain SUM double-counts it (183.940ms
// published where ~15.2ms was the same runnable segment counted by both
// windows). Such a row publishes the interval-union caliber instead — see
// traceCausalProjectionCrossWindowUnion. GROUPING is untouched: the merge key
// stays exactly (subject, object), only the merged row's published value
// changes caliber, so the q1-B6 adjudication (dual-window rows that R2 does
// NOT merge stay independent) is not reversed. Fully disjoint members keep
// the SUM byte-identically (existing pins), and bare time overlap WITHOUT
// distinct window identity also keeps the SUM — same-window overlapping
// same-(subject,object) rows are DISTINCT facts (the E9/E10 9µs strict pin;
// the PTV6 review explicitly rejected envelope-overlap-only folding as a
// noisy signal).
//
// §21 CWD cross-window MAX caliber (cmp_01 revisit 2026-07-07): when members
// from distinct query windows have OVERLAPPING QUERY WINDOWS but the union
// deduction is structurally unavailable (no occurrence Span ts on a windowed
// member, or an F-2 containment violation — the density>1 cpu·ms shape), the
// merged row publishes the member MAX instead of the SUM: overlapping-window
// magnitudes are known to double-count even when the exact overlap cannot be
// deducted per segment (the specimen SUMMED 4 overlapping-window
// supply_pressure observations to 34008.569ms and the flagship comparison's
// direction inverted). See traceCausalProjectionUnionOutcome.crossWindowMax.
// traceCausalProjectionAnchorFormKey is the display-side twin of the engine
// rank-fold anchorForm key (rootCauseFamilyFoldAnchorFormKey, RNB-1 §29.88
// R2/R4): the typed re-anchoring account identity of one row. Precise typed
// signals only; "" on every plain row.
func traceCausalProjectionAnchorFormKey(node TraceCausalProjectionNode) string {
	switch {
	case node.ChainAnchorRemainderSeat:
		return "anchor_remainder"
	case node.ChainAnchorFullMS > 0:
		return "anchor_clipped"
	case node.ChainCredentialLaneDemoted:
		return "lane_demoted"
	case node.ChainAnchorRepresentedByChainSeat:
		// XLANE-1 件1: mirrors the engine fold key — the represented-demoted
		// satellite is its own account form.
		return "anchor_represented"
	case node.GatedShareConstituentSeat:
		// LEVELMERGE-1 件2: the demoted A constituent row is its own account
		// form — never re-Σ with plain rows or with its residual twin.
		return "gated_share_constituent"
	case node.GatedShareFullMS > 0:
		// LEVELMERGE-1 件2: the residual seat (B) publishes a carved account
		// — a plain-row re-Σ would mix residual and full calibers.
		return "gated_share_residual"
	case node.AbsorbedWholeSeatDemotedView:
		// XLANE-2 件3: a chain-face survivor that absorbed a whole-seat
		// demoted view — the account-identity fork the inherited marker used
		// to provide, without the marker's lying word face (its absorbed
		// account can overlap same-(subject,object) siblings, so a plain-row
		// re-Σ is forbidden exactly as before).
		return "absorbed_demoted_view"
	default:
		return ""
	}
}

// TraceCausalProjectionBlockingBasisWaitSegments mirrors
// tracequery.BlockingValueBasisWaitSegments — the wire value of the typed
// blocking_value_basis note (this package cannot import tracequery; the
// equality is pinned tool-side where both packages are visible).
const TraceCausalProjectionBlockingBasisWaitSegments = "wait_segments"

func traceCausalProjectionAggregateSameKind(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	if len(nodes) < traceCausalProjectionSameKindAggregateMin {
		return nodes
	}
	type group struct {
		first   int
		members []int
	}
	groups := map[string]*group{}
	order := make([]string, 0, len(nodes))
	for i, node := range nodes {
		subject := traceCausalProjectionCanonicalNode(node.Subject)
		object := traceCausalProjectionCanonicalNode(node.Object)
		if subject == "" || object == "" {
			continue
		}
		// XERR1-FIX 修补 件B (冷读 P2, 2026-07-16): a converged blocking-wait
		// row (typed basis wait_segments) is ITSELF the re-measured
		// Σ(sleep+D+iowait) of the waiter's wait lane over span∩window — any
		// same-(subject,object) wait-family row (binder ⋈ / sleep twin)
		// measures time PHYSICALLY CONTAINED in that Σ, so an R2 ×N SUM
		// double-counts it (witness: ⊖ 10.721 = 6.637+2.445+1.639 where the
		// two binder rows' 4.084 are exactly the Σ's sleep component). The
		// row is EXEMPT from R2 grouping and stays an independent seat;
		// identity-keyed folds (R1 same-fact absorb, dedup publications) are
		// untouched. Precise typed gate — never the word face; basis-less
		// rows keep the legacy fold byte-identically.
		if node.BlockingValueBasis == TraceCausalProjectionBlockingBasisWaitSegments {
			continue
		}
		key := subject + "\x00" + object
		// 复核 P1-1 捎带 (2026-07-09, pre-existing blind spot): the R2 group
		// key carried no predicate dimension, so a wakeup_causal_aggregate row
		// — a DERIVED VIEW whose per-hop member rows are retained beside it —
		// bucketed WITH its own members and a ≥3 group summed the identical
		// wall clock twice. Aggregate views bucket apart (typed predicate
		// discriminator): view-with-view merging stays possible, view-with-
		// member never.
		if strings.TrimSpace(node.Predicate) == "wakeup_causal_aggregate" {
			key += "\x00wakeup_causal_aggregate"
		}
		// CAL-1 件⑤ PACE-ROW (§29.47.4②, 2026-07-12): a cadence-idle segment
		// (typed pacing_idle / periodic_idle lane, or the R1 survivor that
		// adopted the idle view's TypeToken) buckets APART from the plain
		// same-(subject,state) sleep family — the engine already minted the
		// independent idle row and folding it into the ×N 睡眠 family dilutes
		// both semantics (等依赖 sleep vs 正常节拍空闲). The idle row stands
		// alone (or forms its own pure ×N idle family); the ENG-2 「其中 …」
		// annotation machinery stays as the fold fallback for shapes that
		// still merge (see traceCausalProjectionAbsorbSameFact).
		if ms, kind := traceCausalProjectionIdleCadence(node); ms > 0 && kind != "" {
			key += "\x00idle:" + kind
		}
		// RNB-2 件2 (§29.88 W3 病①, 2026-07-15): the re-anchoring account-
		// identity fork — same judgment as the engine rank-fold anchorForm key
		// (rootCauseFamilyFoldAnchorFormKey, RNB-1): a ◇ remainder seat, a ⛓
		// clipped seat and an R4 lane-demoted seat are DIFFERENT accounts from
		// a plain row of the same (subject, object) and never re-Σ with one.
		// Witness (customer runnable.txt E32): the 9.272-full remainder seat
		// merged with two plain ◇ D-state rows into a 10.643 ×3 row whose 行2
		// still spoke the seed's 「全窗9.272…本行其余9.272」 — 「本行」 was
		// false on the merged row. "" on every plain row keeps all pre-RNB-2
		// merges byte-identical.
		if form := traceCausalProjectionAnchorFormKey(node); form != "" {
			key += "\x00" + form
		}
		g, ok := groups[key]
		if !ok {
			g = &group{first: i}
			groups[key] = g
			order = append(order, key)
		}
		g.members = append(g.members, i)
	}
	replaced := map[int]TraceCausalProjectionNode{}
	dropped := map[int]bool{}
	for _, key := range order {
		g := groups[key]
		if len(g.members) < traceCausalProjectionSameKindAggregateMin {
			continue
		}
		replaced[g.first] = traceCausalProjectionMergeSameKindMembers(nodes, g.first, g.members)
		for _, idx := range g.members {
			if idx != g.first {
				dropped[idx] = true
			}
		}
	}
	if len(replaced) == 0 && len(dropped) == 0 {
		return nodes
	}
	out := make([]TraceCausalProjectionNode, 0, len(nodes))
	for i, node := range nodes {
		if dropped[i] {
			continue
		}
		if aggregate, ok := replaced[i]; ok {
			out = append(out, aggregate)
			continue
		}
		out = append(out, node)
	}
	return out
}

// traceCausalProjectionMergeSameKindMembers is THE R2 ×N member-merge body
// (extracted 2026-07-09, GAP-B G5 — behavior-preserving refactor): given the
// member indexes of one merged group it builds the surviving ×N aggregate
// seeded on nodes[first]. It is the SINGLE merge authority — the ≥3-threshold
// R2 pass above and the display trunk's ×2 same-(thread,state) occurrence fold
// (TraceCausalProjectionMergeOccurrenceRows) both call it, so the field
// semantics (SUM/union/cross-window-max calibers, chimera clears, periodic
// re-derivation) can never drift between the two thresholds.
func traceCausalProjectionMergeSameKindMembers(nodes []TraceCausalProjectionNode, first int, members []int) TraceCausalProjectionNode {
	aggregate := nodes[first]
	// 修复轮二 件B (2026-07-13): the ×N family's refined-D proof is the AND
	// over its members — DISTINCT facts, so one unproven member keeps the
	// honest merged 「D-state/iowait」 word (the R1 same-fact absorb keeps OR:
	// one fact, one proof); the 等待对象 word rides only when every member
	// carries the seed's exact symbol (unanimity, absence never guesses).
	refinedAll := aggregate.DStateRefinedNonIO
	callerUnanimous := strings.TrimSpace(aggregate.BlockedReasonCaller)
	callerConflict := false
	var sum, minMS, maxMS float64
	valuelessRows := 0
	// DISP-3 (§29.8 P2-⑧ E22 窗标回归): the rank ordinal's own window identity
	// travels with whichever member supplies the winning (smallest) Rank — the
	// seed first, then every smaller-rank member below. Verbatim member
	// endpoints; a rank-supplying member without a window identity leaves the
	// pair zero (absence never guesses a window).
	if aggregate.Rank > 0 && traceCausalProjectionIntervalValid(nodes[first].QueryWindowStartTs, nodes[first].QueryWindowEndTs) {
		aggregate.RankQueryWindowStartTs = nodes[first].QueryWindowStartTs
		aggregate.RankQueryWindowEndTs = nodes[first].QueryWindowEndTs
	}
	// DISP-3 复核 P2-1: the actual channel travels verbatim from the seed —
	// its pre-merge chain total rides along so the ⚠ predicate can apply the
	// member-level dual-scope carve-out after the SUM overwrite below destroys
	// the row-level pair. A pre-merged seed keeps its own donor value (already
	// copied with the seed); a seed without an actual leaves the field zero
	// and the display suppresses ⚠ on the merged row (宁漏勿假).
	if aggregate.ActualImpactMS > 0 && aggregate.MergedActualDonorCumulativeMS <= 0 {
		aggregate.MergedActualDonorCumulativeMS = nodes[first].CumulativeImpactMS
	}
	// OMGCLEAN-1 件2 (§29.175 定谳② class-C carriage bug / design G2+G4,
	// 2026-07-20): the ×N merge adopts Rank + window + board identity from the
	// rank-supplying member (DISP-3 / XLANE-3 件1) but never carried
	// FixDirection — a direction-stamped rank seat merged under a
	// direction-bare census/chain-view group-first survivor was stranded in
	// the ◎ 「其他方向」 tail (runnable_2 witness: the tieba self running ×N
	// fold, engine-stamped 频率与热治理; differential proof = the unmerged
	// twin E15 wears the 修向 word). Empty-slot adoption ONLY (a survivor's
	// own published direction always wins — the two sibling carriages at the
	// R1 absorb and the semantic-donor fill share the doctrine), taken ONLY
	// from the rank-supplying member — the same member whose board/window
	// identity the row already wears — and ONLY on typed unanimity of the
	// rank members' published directions: two rank members disagreeing → the
	// slot stays empty and the row keeps the honest tail (宁漏勿假指).
	// Value/ordinal channels untouched (direction is an attribute axis).
	rankDirectionConflict := false
	rankDirectionSeen := ""
	noteRankDirection := func(node TraceCausalProjectionNode) string {
		if node.Rank <= 0 {
			return ""
		}
		direction := strings.TrimSpace(node.FixDirection)
		if direction == "" {
			return ""
		}
		if rankDirectionSeen == "" {
			rankDirectionSeen = direction
		} else if rankDirectionSeen != direction {
			rankDirectionConflict = true
		}
		return direction
	}
	rankSupplierDirection := noteRankDirection(nodes[first])
	// §29.183 G8: seed-side twin of the member presence disjunction below —
	// the seed's own envelope start counts as adopted under exactly the same
	// test, so a legal seed [0,end] start is never overwritten by a later
	// positive member (0 stopped being the unset sentinel).
	envelopeStartSet := aggregate.StartTs > 0 ||
		TraceCausalProjectionWindowPresent(aggregate.StartTs, aggregate.EndTs)
	absorbed := map[string]bool{traceCausalProjectionCanonicalNode(aggregate.EvidenceID): true}
	for _, idx := range members {
		member := nodes[idx]
		traceCausalProjectionAppendMergedSubject(&aggregate, member.Subject)
		display := member.ImpactMS
		if display <= 0 {
			display = member.CumulativeImpactMS
		}
		// G12-ENG (§29.1): non-positive display members never enter the
		// min–max range below — count them so the ×N range claim stays honest
		// (same accounting as the on-chain overflow fold constructor).
		if display <= 0 {
			valuelessRows++
		}
		sum += display
		if minMS == 0 || (display > 0 && display < minMS) {
			minMS = display
		}
		if display > maxMS {
			maxMS = display
		}
		if idx != first {
			refinedAll = refinedAll && member.DStateRefinedNonIO
			if strings.TrimSpace(member.BlockedReasonCaller) != callerUnanimous {
				callerConflict = true
			}
			// OMGCLEAN-1 件2: every rank-carrying member joins the direction
			// unanimity census (the winner assignment sits in the rank-adoption
			// branch below — 供席与普查分离, so a non-winning rank member's
			// disagreement still vetoes the adoption).
			noteRankDirection(member)
			id := strings.TrimSpace(member.EvidenceID)
			if id != "" && !absorbed[traceCausalProjectionCanonicalNode(id)] {
				absorbed[traceCausalProjectionCanonicalNode(id)] = true
				aggregate.MergedEvidenceIDs = append(aggregate.MergedEvidenceIDs, id)
			}
			for _, id := range member.MergedEvidenceIDs {
				if id = strings.TrimSpace(id); id != "" && !absorbed[traceCausalProjectionCanonicalNode(id)] {
					absorbed[traceCausalProjectionCanonicalNode(id)] = true
					aggregate.MergedEvidenceIDs = append(aggregate.MergedEvidenceIDs, id)
				}
			}
			for _, object := range member.SecondaryObjects {
				traceCausalProjectionAppendSecondaryObject(&aggregate, object)
			}
			// ENG-2: distinct member facts' idle-cadence annotations SUM on
			// the ×N aggregate (the one-fact MAX lives in the R1 absorb).
			if ms, kind := traceCausalProjectionIdleCadence(member); ms > 0 {
				if aggregate.IdleCadenceKind == "" {
					aggregate.IdleCadenceKind = kind
				}
				aggregate.IdleCadenceMS += ms
			}
			if member.LineStart > 0 && (aggregate.LineStart <= 0 || member.LineStart < aggregate.LineStart) {
				aggregate.LineStart = member.LineStart
			}
			if member.LineEnd > aggregate.LineEnd {
				aggregate.LineEnd = member.LineEnd
			}
			if member.Rank > 0 && (aggregate.Rank <= 0 || member.Rank < aggregate.Rank) {
				aggregate.Rank = member.Rank
				// DISP-3 (§29.8 P2-⑧): the ordinal's window identity follows the
				// rank-supplying member verbatim. A pre-merged member seed keeps
				// its own RankQueryWindow pair when its row-level window was
				// already zeroed by an earlier merge.
				aggregate.RankQueryWindowStartTs, aggregate.RankQueryWindowEndTs = 0, 0
				if traceCausalProjectionIntervalValid(member.QueryWindowStartTs, member.QueryWindowEndTs) {
					aggregate.RankQueryWindowStartTs = member.QueryWindowStartTs
					aggregate.RankQueryWindowEndTs = member.QueryWindowEndTs
				} else if traceCausalProjectionIntervalValid(member.RankQueryWindowStartTs, member.RankQueryWindowEndTs) {
					aggregate.RankQueryWindowStartTs = member.RankQueryWindowStartTs
					aggregate.RankQueryWindowEndTs = member.RankQueryWindowEndTs
				}
				// XLANE-3 件1: the ordinal's BOARD identity follows the same
				// rank-supplying member verbatim (target + params halves).
				aggregate.RankBoardTarget = member.RankBoardTarget
				aggregate.RankBoardParamsFingerprint = member.RankBoardParamsFingerprint
				// OMGCLEAN-1 件2: the adoption candidate follows the SAME
				// rank-supplying member (board/window/direction one identity).
				rankSupplierDirection = strings.TrimSpace(member.FixDirection)
			}
			if member.Confidence > 0 && (aggregate.Confidence <= 0 || member.Confidence < aggregate.Confidence) {
				aggregate.Confidence = member.Confidence
			}
			// §29.183 G8: the envelope-start min-fold — ts==0 is a REAL start
			// in a rebased [0,end] trace, so 0 can no longer double as the
			// accumulator's unset sentinel; envelopeStartSet (initialized from
			// the seed above) tracks adoption instead. Member presence keeps
			// the pre-G8 positive arm verbatim (instant markers StartTs==EndTs>0
			// keep folding) and adds only the real [0,end] window shape; the
			// (0,0) absence pair still never folds.
			if member.StartTs > 0 || TraceCausalProjectionWindowPresent(member.StartTs, member.EndTs) {
				if !envelopeStartSet || member.StartTs < aggregate.StartTs {
					aggregate.StartTs = member.StartTs
					envelopeStartSet = true
				}
			}
			if member.EndTs > aggregate.EndTs {
				aggregate.EndTs = member.EndTs
			}
		}
	}
	// OMGCLEAN-1 件2: empty-slot FixDirection adoption fires only when the
	// survivor arrived bare, the rank-supplying member publishes a direction,
	// and every rank member agrees (typed unanimity — conflict keeps the
	// honest empty slot / tail placement).
	if strings.TrimSpace(aggregate.FixDirection) == "" && !rankDirectionConflict &&
		rankSupplierDirection != "" {
		aggregate.FixDirection = rankSupplierDirection
	}
	aggregate.MergedCount = len(members)
	aggregate.MergedMinMS = minMS
	aggregate.MergedMaxMS = maxMS
	aggregate.MergedValuelessCount = valuelessRows
	aggregate.ImpactMS = sum
	aggregate.CumulativeImpactMS = sum
	// COV §24.9 D-1: TargetImpactMS re-derives as the member MAX — the
	// members explain overlapping stretches of the ONE target's blocked
	// wall clock, so a Σ would double-count it, and a group-first
	// inheritance is order-dependent (D-3 家族). MAX never invents.
	targetImpact := 0.0
	for _, idx := range members {
		if v := nodes[idx].TargetImpactMS; v > targetImpact {
			targetImpact = v
		}
	}
	aggregate.TargetImpactMS = targetImpact
	// §11-N2: cross-query-window caliber. The roster of distinct member
	// query windows is disclosed on every ×N row whose members carried a
	// window identity (窗身份, 联动 q1-B6); the union caliber replaces the
	// SUM only when distinct-window members overlap in time (see the
	// function docs — disjoint and window-less shapes keep the SUM
	// unchanged). Row-level window identity survives only when EVERY
	// member carried the SAME window — a mixed or partially-unknown roster
	// must not let the merged row claim a single window as its own.
	union := traceCausalProjectionCrossWindowUnion(nodes, members)
	aggregate.MergedQueryWindows = union.roster
	if !union.singleWindow {
		aggregate.QueryWindowStartTs, aggregate.QueryWindowEndTs = 0, 0
	}
	if union.applied {
		aggregate.ImpactMS = union.unionMS
		aggregate.CumulativeImpactMS = union.unionMS
		aggregate.MergedIntervalUnion = true
		aggregate.MergedSumMS = sum
	} else if union.crossWindowMax {
		// §21 CWD (cmp_01 revisit 2026-07-07, D-新P0 排队深度方向反转
		// engine half): overlapping-query-window magnitudes must not SUM
		// (墙钟跨窗不可加和) and the per-segment union deduction was
		// structurally unavailable — publish the member MAX (a lower
		// bound that never invents; R3 cross-thread fold precedent) with
		// the lossless raw Σ kept for the audit trail and the max
		// member's own query window kept as the display density base.
		aggregate.ImpactMS = maxMS
		aggregate.CumulativeImpactMS = maxMS
		aggregate.MergedCrossWindowMax = true
		aggregate.MergedSumMS = sum
		aggregate.MergedMaxWindowStartTs = union.maxMemberWindowStart
		aggregate.MergedMaxWindowEndTs = union.maxMemberWindowEnd
	}
	// F2 (adversarial review 2026-07-03): the ×N row carries a SUM, so the
	// DuplicatePublications contract ("dup>0 ⇒ the value is ONE republished
	// measurement") can never hold on it — a dup count inherited from the
	// group-first survivor (or silently lost from a non-first member) once
	// rendered the mutually-exclusive ×2同值合并 and ×3合并 labels on one row.
	// Cleared unconditionally: member provenance stays lossless through
	// MergedEvidenceIDs; no second counter is introduced.
	aggregate.DuplicatePublications = 0
	// RCM-2 复核 F-1 (2026-07-08, ledger §24.7.1/§24.10 批): the group-first
	// seed can be an ENGINE family contender (multi-window same-(thread,type)
	// families make ≥3 same-kind rows production-reachable), and inheriting
	// its FamilyMember*/FamilyFoldCaliber/roster/BackgroundRank/Inode/Dev
	// wholesale minted a CHIMERA row carrying BOTH ×N lanes — 行1
	// 「×2 合计6.598」 beside the subordinate 「×3(1.598–3.000ms)」 R2 tag
	// (one row, two contradictory counts) — and let ONE member's inode/board
	// seat impersonate the whole merge. Cleared unconditionally, same family
	// as the DuplicatePublications/SupplyFold clears beside it: the merged
	// row is an R2 ×N SUM and speaks ONLY that grammar; member provenance
	// stays lossless through MergedEvidenceIDs. (The R3 background fold is
	// structurally clean — it builds a FRESH node, never a group-first copy;
	// R1/V4/alias absorbs keep the SURVIVOR's own identity and set no
	// MergedCount, so no second ×N lane can co-render there.)
	aggregate.FamilyMemberCount = 0
	aggregate.FamilyMemberMaxMS = 0
	aggregate.FamilyMemberMinMS = 0
	aggregate.FamilyMemberSumMS = 0
	aggregate.FamilyFoldCaliber = ""
	aggregate.FamilyMemberRoster = nil
	// SPANTOP-1 件1 hygiene (§29.131): the per-member carriers die with the
	// family grammar they decompose (every consumer already gates on
	// FamilyMemberCount>1, so this is belt-and-braces, not behavior).
	aggregate.FamilyMemberLineRanges = nil
	aggregate.FamilyMemberWallMS = nil
	aggregate.BackgroundRank = 0
	aggregate.Inode = ""
	aggregate.Dev = ""
	// G1 收尾 P2-b (对抗复核, 2026-07-09): RankFamilyKey is NOT family
	// grammar — it is the 链上并入 disclosure's JOIN IDENTITY (§27.2-G1), and
	// unlike the nine fields above it must SURVIVE the merge: a family
	// contender absorbed as a NON-first ×N member otherwise lost the key, the
	// display attach found no carrier, and the absorbed-rows disclosure
	// silently died (values stayed lossless via the SUM; the disclosure
	// identity did not). Key ONLY — the F-1 chimera lesson holds: the merged
	// row stays a plain R2 ×N row (family grammar fields cleared above, so
	// runtimeTraceProjFamilyRow stays false and no family wording renders);
	// the key's sole consumer is the AbsorbedChainPeers attach + the 链上并入
	// note, which the display renders independent of family grammar. First
	// non-empty member key wins (deterministic member order); a second
	// distinct key in one merge group (two absorbing families sharing
	// (subject,object) across windows/lanes) keeps only the first — that
	// family's disclosure rides its E# index entries (honest residual,
	// P3 留观 same class as the cap-seat non-return).
	aggregate.RankFamilyKey = ""
	for _, idx := range members {
		if key := strings.TrimSpace(nodes[idx].RankFamilyKey); key != "" {
			aggregate.RankFamilyKey = key
			break
		}
	}
	// RNB-2 件2 (§29.88 W3 病①, 2026-07-15; CASE3-D4 同款处置精神 — 禁单成员
	// 值冒充合并行账): the seed's ChainAnchor bipartition triple (full /
	// anchored / the divergence quartet) is a PER-SEAT ledger account — on a
	// ×N Σ row the 行2 「全窗X=…本行其余Y」 grammar has no true referent
	// (「本行」 is the merged row, whose value is a member Σ; the seed triple
	// spoke for one member only). The anchorForm grouping fork above keeps
	// groups homogeneous, but even a homogeneous Σ of bipartition seats may
	// not Σ the triples (same-(pid,family) cross-window decisions can re-
	// measure ONE census account — the very double count the union/CWD
	// calibers exist for), so the merged row clears the account fields and
	// carries the typed seed-member qualifier instead; member splits stay
	// lossless on the members' own detail/evidence-index faces.
	// 如实注 (E7 继承源, inv_3 任务2): the customer runnable.txt E7 shape
	// (有效归因 8.606 beside 行值 8.226) lives in this domain — the concrete
	// case replay awaits the customer's original ftrace (§29.88 立案注).
	for _, idx := range members {
		if nodes[idx].ChainAnchorFullMS > 0 || nodes[idx].ChainAnchorRemainderSeat {
			aggregate.ChainAnchoredMS = 0
			aggregate.ChainAnchorFullMS = 0
			aggregate.ChainAnchorOwnershipDivergent = false
			aggregate.ChainAnchorChainLaneMS = 0
			aggregate.ChainAnchorCensusMS = 0
			aggregate.MergedChainAnchorMemberAccounts = true
			break
		}
	}
	// LEVELMERGE-1 件2 (方案 P, 2026-07-18): same per-seat-ledger discipline
	// for the gated-share split — a ×N Σ row must not wear one member's
	// claimed/full decomposition or its claim-seat pointers (「本行」 grammar
	// has no true referent on a member Σ). The anchorForm fork keeps groups
	// homogeneous; a homogeneous Σ still may not Σ the accounts.
	for _, idx := range members {
		if nodes[idx].GatedShareFullMS > 0 || nodes[idx].GatedShareOverlapDisclosureMS > 0 {
			aggregate.GatedShareClaimedMS = 0
			aggregate.GatedShareFullMS = 0
			aggregate.GatedShareClaimSeats = nil
			aggregate.GatedShareOverlapDisclosureMS = 0
			break
		}
	}
	// PARTSPLIT-1 (§29.150④): same grammar — a ×N Σ row must not wear one
	// member's R4-refusal bisection record (X+Y partitions ONE member's own
	// runnable account, never a member Σ). The 行2 sub-line's own identity
	// re-validation would already refuse to render it; clearing here keeps
	// the wire face honest too.
	for _, idx := range members {
		if nodes[idx].GatedCompositeEdgeAnchorTS > 0 {
			aggregate.GatedCompositeEdgePreShareMS = 0
			aggregate.GatedCompositeEdgePostShareMS = 0
			aggregate.GatedCompositeEdgeAnchorTS = 0
			aggregate.GatedCompositeEdgeAnchorVia = ""
			break
		}
	}
	// SFD 复核 F3 (2026-07-07, same family as the DuplicatePublications
	// clear above and the VS-1 F6(a) periodic re-derivation below): the ×N
	// row carries a member SUM, so a SINGLE member's supply-fold accounting
	// (deficit/ideal over that one segment's own running clock, §7.10) can
	// never describe it — the group-first seed's inherited group rendered a
	// single-member deficit clause beside a three-member sum (伪对比: 缺口
	// 5.000ms 贴在 42.000ms 总和旁). Cleared unconditionally whenever ≥2
	// members merge; re-deriving a fold from members stays open (P0-E)
	// until the engine publishes a per-member basis to sum from — the
	// display layer never mints accounting the engine did not publish.
	aggregate.SupplyFoldComputed = false
	aggregate.SupplyFoldDeficitMS = 0
	aggregate.SupplyFoldIdealMS = 0
	aggregate.SupplyFoldKnownMS = 0
	aggregate.SupplyFoldUnknownMS = 0
	// VS-1 F6(a) (adversarial review 2026-07-04): the ×N SUM row re-derives
	// its periodic accounting from the MEMBERS instead of inheriting the
	// group-first copy. All members periodic → the fold keeps the flag with
	// the summed discount and the group head's DetectedPeriodMS (already on
	// the aggregate). The original F6(a) basis claimed the Σ is legal
	// UNCONDITIONALLY ("per-member discounts are disjoint per-occurrence
	// amounts, never overlapping wall clock") — that absolute held only for
	// DISTINCT occurrences and was retired by PERIODIC-DEDUP (§29.104 ①,
	// 2026-07-15): a cross-window re-measured occurrence carries the SAME
	// discount twice, so the Σ inside the arm below now dedups proven
	// same-segment re-measurements (see the arm's EVOLUTION RECORD and
	// traceCausalProjectionPeriodicDiscountCounted). ANY non-periodic member
	// → the SUM row is back to raw semantics: flag, cadence fields and the
	// inherited (periodic-only) discount are cleared — a part-cadence sum
	// labelled periodic would discount real waits it never measured, and a
	// stale group-first effective would understate the ×N total.
	allPeriodic := true
	for _, idx := range members {
		if !nodes[idx].PeriodicSource {
			allPeriodic = false
			break
		}
	}
	if allPeriodic {
		// PERIODIC-DEDUP (§29.104 ①, 2026-07-15). EVOLUTION RECORD: this Σ
		// used to add EVERY member's discount unconditionally on the
		// structural basis 逐次折减不重叠 — true for DISTINCT occurrences,
		// violated when one occurrence is re-measured across query windows
		// (§29.98 件2 诱错: the value channel's union caliber proved E10/E11
		// one occurrence at 66.000 while this lane paid the shared discount
		// twice, 0.090 vs 0.060). The Σ now consumes the SAME same-segment
		// proof (window slots + interval overlap): a member whose occurrence
		// is already counted from ANOTHER window contributes nothing, and the
		// seat-owning window's copy is the counted one (种子/席位窗优先 —
		// see traceCausalProjectionPeriodicDiscountCounted). Groups the proof
		// never touches (single-window, disjoint multi-window, windowless or
		// interval-less members) keep all-true counted sets and Σ in member
		// order byte-identically (F6(a) legal Σ untouched).
		seatStartTs, seatEndTs := 0.0, 0.0
		if aggregate.Rank > 0 && traceCausalProjectionIntervalValid(aggregate.RankQueryWindowStartTs, aggregate.RankQueryWindowEndTs) {
			seatStartTs, seatEndTs = aggregate.RankQueryWindowStartTs, aggregate.RankQueryWindowEndTs
		} else if traceCausalProjectionIntervalValid(nodes[first].QueryWindowStartTs, nodes[first].QueryWindowEndTs) {
			seatStartTs, seatEndTs = nodes[first].QueryWindowStartTs, nodes[first].QueryWindowEndTs
		}
		countedMembers := traceCausalProjectionPeriodicDiscountCounted(nodes, members, union, seatStartTs, seatEndTs)
		effective, lateness := 0.0, 0.0
		published := false
		for k, idx := range members {
			if !countedMembers[k] {
				continue
			}
			effective += nodes[idx].EffectiveImpactMS
			lateness += nodes[idx].PeriodicLatenessMS
			// EPUB (§29.31): Σ over published member discounts is itself a
			// published discount — any COUNTED published member keeps the fold
			// row published (OR-monotone, same direction as the R1 merge arm).
			// A skipped re-measurement's marker speaks for a copy that is not
			// in the Σ and never publishes it.
			published = published || nodes[idx].EffectiveImpactPublished
		}
		aggregate.EffectiveImpactMS = effective
		aggregate.PeriodicLatenessMS = lateness
		aggregate.EffectiveImpactPublished = published
	} else if aggregate.PeriodicSource {
		aggregate.PeriodicSource = false
		aggregate.DetectedPeriodMS = 0
		aggregate.PeriodicLatenessMS = 0
		aggregate.EffectiveImpactMS = 0
		// EPUB (§29.31): this branch is a deliberate UN-publication — the
		// part-cadence ×N row is "back to raw semantics" and the engine never
		// published a fold effective for the mixed sum, so the inherited
		// group-first marker must clear with the inherited discount (a
		// dangling published-0 here would wrongly refuse the ×N crown).
		aggregate.EffectiveImpactPublished = false
	} else {
		// CASE3-D4 B 根修 (§29.84 件④ 裁定, LT-HYG CASE-3 ❹ witness,
		// real_trace_campaign_20260705.md, 2026-07-14). EVOLUTION RECORD: the
		// plain (non-periodic) ×N fold's effective slot used to be a
		// group-first INHERITED copy ("the inherited VALUE stays untouched") —
		// a 3-member merge rendered 「3次(2.000~4.000ms) · 有效归因2.500ms」
		// with the SEED's single-member effective wearing zero qualifying
		// words, the ◎ board seated the row at that single-member value, and
		// the tree Σ vs ◎ value cross-face gap had no explanation. The plain
		// arm now re-mints from the members like the periodic arm above:
		//   - EVERY member carries a positive effective → Σ member eff (the
		//     §29.50.4 合计参赛 direction; member effectives are per-member
		//     attributions of DISTINCT facts, so the Σ is legal exactly where
		//     the display SUM is — and a pre-merged member's eff is already
		//     its own member Σ, so the semantics are idempotent);
		//   - ANY member without one (0 / never minted) → the whole row's
		//     effective clears to 0 (宁缺勿假 — the honest total is unknowable;
		//     the member's cumulative is a DIFFERENT caliber and must never
		//     substitute, per the ruling's explicit ban);
		//   - §11-N2 union / §21 CWD cross-window-MAX calibers → 0 as well:
		//     those calibers exist because the members' magnitudes re-measure
		//     overlapping wall clock across query windows (墙钟跨窗不可加和),
		//     and member effectives on these plain rows are the same
		//     wall-clock-derived magnitudes — a Σ here would re-mint the very
		//     double count the value channel retired (and exceed the row's
		//     published union/MAX value, forging a false 承自归因 face). No
		//     per-member deduction exists for the effective channel, so the
		//     row honestly publishes none.
		// EPUB 复核 L1 marker semantics evolve with the value: the Σ arm is a
		// derived published discount (OR-monotone over member markers, same
		// direction as the periodic arm); both clear arms un-publish — the
		// cleared 0 is an ABSENT effective, never an authoritative engine
		// zero, and a dangling published-0 would wrongly refuse the ×N crown
		// downstream (fail-open direction per the field contract).
		effective := 0.0
		published := false
		allMinted := true
		for _, idx := range members {
			if nodes[idx].EffectiveImpactMS <= 0 {
				allMinted = false
				break
			}
			effective += nodes[idx].EffectiveImpactMS
			published = published || nodes[idx].EffectiveImpactPublished
		}
		if allMinted && !union.applied && !union.crossWindowMax {
			aggregate.EffectiveImpactMS = effective
			aggregate.EffectiveImpactPublished = published
		} else {
			aggregate.EffectiveImpactMS = 0
			aggregate.EffectiveImpactPublished = false
		}
	}
	aggregate.DStateRefinedNonIO = refinedAll
	if callerConflict {
		aggregate.BlockedReasonCaller = ""
	}
	return aggregate
}

// TraceCausalProjectionMergeOccurrenceRows (GAP-B G5, §27.3
// real_trace_campaign_20260705.md, 2026-07-09) merges ≥2 occurrence rows of
// ONE subject into a single R2-grammar ×N row through the SAME merge authority
// as the aggregation pass (traceCausalProjectionMergeSameKindMembers — sum +
// per-instance a–b range, union/cross-window-max calibers, chimera clears,
// periodic re-derivation, lossless MergedEvidenceIDs). rows[0] is the seed
// identity carrier. The MEMBERSHIP decision (which rows may merge) is the
// caller's policy — the display trunk admits same-(thread, dominant-state)
// plain occurrence pairs at threshold 2, because rendering a thread's second
// same-state occurrence as its own 成因 child claims the thread CAUSED ITSELF
// (semantic error, §27.3 G5 witness: OS_mmi_EventHdr sleep 0.904 hung under
// sleep 4.431 as "├─成因─"), while the R2 pass keeps its ≥3 threshold (its
// fold is a row-count economy, not an error repair).
func TraceCausalProjectionMergeOccurrenceRows(rows []TraceCausalProjectionNode) TraceCausalProjectionNode {
	if len(rows) == 0 {
		return TraceCausalProjectionNode{}
	}
	if len(rows) == 1 {
		return rows[0]
	}
	members := make([]int, len(rows))
	for i := range rows {
		members[i] = i
	}
	return traceCausalProjectionMergeSameKindMembers(rows, 0, members)
}

// --- N2: cross-query-window ×N union caliber ---------------------------------

// traceCausalProjectionUnionOutcome is what the R2 merge consumes from the
// §11-N2 cross-window scan of one merged group.
type traceCausalProjectionUnionOutcome struct {
	// roster: the distinct member query windows (F-2 ±1ms endpoint dedupe,
	// ascending start order). Empty when no member carried an identity.
	roster []TraceCausalProjectionQueryWindow
	// singleWindow: EVERY member carried an identity and all matched ONE
	// window — only then may the merged row keep a row-level QueryWindow.
	singleWindow bool
	// applied + unionMS: the union caliber engaged (distinct-window members
	// overlap in time) and this is the deduplicated value.
	applied bool
	unionMS float64
	// crossWindowMax (§21 CWD, cmp_01 revisit 2026-07-07): members from ≥2
	// distinct query windows whose QUERY WINDOWS overlap in time, while the
	// union deduction is structurally unavailable — a windowed member without
	// a valid occurrence interval (rank-lane rows carry no Span ts) or an F-2
	// containment violation (value > own interval, the density>1 cpu·ms
	// shape). Overlapping-window magnitudes must not SUM (墙钟跨窗不可加和);
	// the merged row publishes the member MAX instead. Never set when the
	// union caliber applied (the per-segment deduction is more precise).
	crossWindowMax bool
	// maxMemberWindowStart/End: the typed query window of the member whose
	// display value is the group MAX (same strict-> / first-wins order as the
	// R2 merge loop's maxMS). Zero when that member carries no identity.
	maxMemberWindowStart float64
	maxMemberWindowEnd   float64
	// slots / slotOf (PERIODIC-DEDUP §29.104 ①, 2026-07-15): the window-slot
	// assignment the union proof is built on — slots lists the distinct member
	// query windows in first-occurrence order (the roster above is a SORTED
	// copy) and slotOf aligns index-for-index with the members slice (-1 = no
	// window identity). Recorded unconditionally before any early-out so the
	// periodic Σ-effective dedup consumes the SAME slot identity the value
	// channel proves same-segment re-measurement with (one authority, never a
	// second slot implementation).
	slots  []TraceCausalProjectionQueryWindow
	slotOf []int
}

// traceCausalProjectionCrossWindowUnion computes the §11-N2 window roster and
// (when distinct-window members overlap in time) the interval-union caliber
// value for one R2 merged group. Precise signals only:
//   - window identity: the members' typed QueryWindowStartTs/EndTs, grouped
//     into slots with the F-2 ±1ms endpoint tolerance (the SAME constant the
//     projection-level QueryWindows list dedupes with — one tolerance);
//   - engagement: ∃ member pair in DIFFERENT slots whose occurrence intervals
//     (typed StartTs/EndTs) overlap — bare overlap without distinct windows
//     never engages (same-window overlapping rows are distinct facts, E9/E10
//     strict pin), and distinct windows without time overlap never engage
//     (disjoint cross-window instances legitimately SUM);
//   - value: members are visited in display-value-descending order (ties by
//     bucket order — deterministic); each member's contribution is its value
//     minus min(value, wall clock already counted by OTHER windows inside the
//     member's own interval). The deduction is pure interval algebra on the
//     shared authority (trace_causal_projection_interval.go): bounded by the
//     physical overlap, so a contained re-measurement (the q2 E10 15.206ms
//     occurrence inside the 104.127ms one) contributes 0 while a partial
//     overlap loses at most the overlapping seconds — the union value is a
//     lower bound that never invents and never drops below the largest single
//     member. Members without a window identity or without a valid interval
//     contribute their full value and never deduct from anyone (fail-open to
//     the legacy SUM semantics for exactly those members).
//
// The periodic Σ-effective lane (VS-1 F6(a)) consumes this function's slot
// assignment (slots/slotOf on the outcome) for its own cross-window
// same-occurrence dedup since PERIODIC-DEDUP (§29.104 ①, 2026-07-15 — the
// former "out of N2 scope" residual is CLOSED): the per-occurrence discount
// arithmetic lives in traceCausalProjectionPeriodicDiscountCounted; the value
// deduction below is unchanged.
func traceCausalProjectionCrossWindowUnion(nodes []TraceCausalProjectionNode, members []int) traceCausalProjectionUnionOutcome {
	out := traceCausalProjectionUnionOutcome{}
	if len(members) == 0 {
		return out
	}
	// Window-slot assignment (F-2 tolerance, first occurrence defines a slot).
	slots := make([]TraceCausalProjectionQueryWindow, 0, 2)
	slotOf := make([]int, len(members))
	withIdentity := 0
	for k, idx := range members {
		node := nodes[idx]
		slotOf[k] = -1
		if !traceCausalProjectionIntervalValid(node.QueryWindowStartTs, node.QueryWindowEndTs) {
			continue
		}
		withIdentity++
		found := -1
		for si, w := range slots {
			if math.Abs(w.StartTs-node.QueryWindowStartTs) <= traceCausalProjectionFullWindowSameWindowToleranceS &&
				math.Abs(w.EndTs-node.QueryWindowEndTs) <= traceCausalProjectionFullWindowSameWindowToleranceS {
				found = si
				break
			}
		}
		if found < 0 {
			slots = append(slots, TraceCausalProjectionQueryWindow{
				StartTs: node.QueryWindowStartTs,
				EndTs:   node.QueryWindowEndTs,
			})
			found = len(slots) - 1
		}
		slotOf[k] = found
	}
	if len(slots) > 0 {
		roster := make([]TraceCausalProjectionQueryWindow, len(slots))
		copy(roster, slots)
		out.roster = traceCausalProjectionSortQueryWindows(roster)
	}
	out.slots = slots
	out.slotOf = slotOf
	out.singleWindow = len(slots) == 1 && withIdentity == len(members)
	if len(slots) < 2 {
		return out
	}
	// §21 CWD window-level overlap (cmp_01 revisit 2026-07-07): ∃ slot pair
	// whose QUERY WINDOWS overlap in time — pure typed arithmetic on the
	// window endpoints, never occurrence spans. Disjoint windows measured
	// disjoint wall clock and legitimately SUM; overlapping windows re-measure
	// the same clock (or the same cpu·ms capacity) and must not.
	windowsOverlap := false
	for i := 0; i < len(slots) && !windowsOverlap; i++ {
		for j := i + 1; j < len(slots); j++ {
			if TraceCausalProjectionIntervalsOverlap(slots[i].StartTs, slots[i].EndTs, slots[j].StartTs, slots[j].EndTs) {
				windowsOverlap = true
				break
			}
		}
	}
	// markCrossWindowMax records the §21-CWD MAX-caliber outcome together with
	// the max member's own typed query window (exact member endpoints, same
	// strict-> first-wins scan order as the merge loop's maxMS accounting so
	// the recorded window always belongs to the published MAX value).
	markCrossWindowMax := func() {
		out.crossWindowMax = true
		best, bestValue := -1, 0.0
		for k, idx := range members {
			if v := traceCausalProjectionDisplayValue(nodes[idx]); v > bestValue {
				best, bestValue = k, v
			}
		}
		if best >= 0 {
			node := nodes[members[best]]
			if traceCausalProjectionIntervalValid(node.QueryWindowStartTs, node.QueryWindowEndTs) {
				out.maxMemberWindowStart = node.QueryWindowStartTs
				out.maxMemberWindowEnd = node.QueryWindowEndTs
			}
		}
	}
	// F-2 structural-premise gate (复核 2026-07-06): the union deduction
	// assumes every member's value is wall clock CONTAINED in its own
	// occurrence interval (value ≤ interval length). Per-occurrence
	// wakeup_causal rows satisfy that by construction, but a density>1 row
	// (e.g. a multi-CPU cpu·ms cumulative whose value exceeds its interval's
	// wall clock) would break the premise and the interval deduction would
	// under-subtract. Precise in-code gate instead of a comment-level
	// assumption (future root_cause families gaining Span ts must not
	// silently activate an unsound deduction): ANY windowed member whose
	// value exceeds its own interval length ×(1+1e-9) (float headroom only,
	// not a tolerance band) fails the WHOLE group out of the union deduction.
	// Windowless / interval-less members never deduct anyway and are exempt
	// from the containment check, but a windowed member WITHOUT a valid
	// occurrence interval leaves cross-window occurrence disjointness
	// unprovable (§21 CWD) — tracked for the fail-open fork below.
	//
	// Fail-open target fork (§21 CWD, evolving the original fail-open-to-SUM):
	// when the member QUERY WINDOWS overlap, the SUM is known to double-count
	// regardless of whether the per-segment deduction is computable, so the
	// group fails open to the member MAX, not the SUM. Disjoint-window groups
	// keep the legacy SUM fail-open byte-identically.
	windowedWithoutInterval := false
	for k, idx := range members {
		node := nodes[idx]
		if slotOf[k] < 0 {
			continue
		}
		if !traceCausalProjectionIntervalValid(node.StartTs, node.EndTs) {
			windowedWithoutInterval = true
			continue
		}
		intervalMS := (node.EndTs - node.StartTs) * 1000
		if traceCausalProjectionDisplayValue(node) > intervalMS*(1+1e-9) {
			if windowsOverlap {
				markCrossWindowMax()
			}
			return out
		}
	}
	// Engagement: any cross-slot pair with overlapping occurrence intervals.
	engaged := false
	for k := 0; k < len(members) && !engaged; k++ {
		if slotOf[k] < 0 {
			continue
		}
		for l := k + 1; l < len(members); l++ {
			if slotOf[l] < 0 || slotOf[l] == slotOf[k] {
				continue
			}
			if traceCausalProjectionSpansOverlap(nodes[members[k]], nodes[members[l]]) {
				engaged = true
				break
			}
		}
	}
	if !engaged {
		// §21 CWD: with overlapping query windows, occurrence disjointness is
		// only provable when every windowed member exposed a valid occurrence
		// interval — the engagement scan above then really compared them all
		// and found no overlap (distinct facts, SUM stands). A windowed member
		// without an interval (the rank-lane no-Span-ts shape) leaves the
		// double count unprovable either way → member MAX, never the SUM.
		if windowsOverlap && windowedWithoutInterval {
			markCrossWindowMax()
		}
		return out
	}
	// Union value: display-desc greedy with cross-window interval deduction.
	order := make([]int, len(members))
	for k := range order {
		order[k] = k
	}
	sort.SliceStable(order, func(a, b int) bool {
		return traceCausalProjectionDisplayValue(nodes[members[order[a]]]) >
			traceCausalProjectionDisplayValue(nodes[members[order[b]]])
	})
	perSlot := make([]TraceCausalProjectionIntervalSet, len(slots))
	total := 0.0
	for _, k := range order {
		node := nodes[members[k]]
		display := traceCausalProjectionDisplayValue(node)
		contribution := display
		if slotOf[k] >= 0 && traceCausalProjectionIntervalValid(node.StartTs, node.EndTs) {
			// Wall clock inside THIS member's interval already counted by other
			// windows: union the other slots' coverage clipped to the member's
			// interval first (two other windows overlapping each other must not
			// deduct twice), then bound the deduction by the member's own value.
			var counted TraceCausalProjectionIntervalSet
			for si := range perSlot {
				if si == slotOf[k] {
					continue
				}
				for _, span := range perSlot[si].Spans() {
					lo, hi := span.StartTs, span.EndTs
					if lo < node.StartTs {
						lo = node.StartTs
					}
					if hi > node.EndTs {
						hi = node.EndTs
					}
					if hi > lo {
						counted.Add(lo, hi)
					}
				}
			}
			if dedupMS := counted.TotalSeconds() * 1000; dedupMS > 0 {
				if dedupMS > display {
					dedupMS = display
				}
				contribution = display - dedupMS
			}
			perSlot[slotOf[k]].Add(node.StartTs, node.EndTs)
		}
		total += contribution
	}
	out.applied = true
	out.unionMS = total
	return out
}

// traceCausalProjectionPeriodicDiscountCounted (PERIODIC-DEDUP, §29.104 ①
// 终判 2026-07-15; closes §29.85 残留① via the §29.98 件2 诱错 witness)
// decides, for one ALL-periodic ×N merge group, which members' per-occurrence
// periodic accounting (EffectiveImpactMS discount + PeriodicLatenessMS)
// enters the fold Σ. The same-segment identity proof is EXACTLY the value
// channel's union-caliber basis (traceCausalProjectionCrossWindowUnion, whose
// slots/slotOf this consumes): typed window slots (F-2 ±1ms tolerance) plus
// the shared strict interval-overlap predicate
// (traceCausalProjectionSpansOverlap) — two members from DIFFERENT query
// windows whose occurrence intervals overlap re-measure ONE physical
// occurrence, and one physical occurrence's discount counts ONCE (§29.98 件2:
// the union value channel proved E10/E11 one occurrence at 66.000 while the
// Σ-effective lane paid the shared 0.030 discount twice, 0.090 vs the unique
// 0.060). The lateness Σ rides the same occurrence identity — one physical
// late tick carries one lateness amount, so the counted copy's lateness is
// the one that enters (same lane accounting, not a second dedup).
//
// Pick rule (终判① verbatim: 重测折减值不同取席位归属窗份): members whose
// typed window slot matches the SEAT-owning window are visited FIRST, so when
// re-measured copies disagree the seat window's copy is the counted one. The
// seat window is typed, never a heuristic: a ranked row's ordinal window
// (aggregate RankQueryWindow, the DISP-3 席位窗 identity) when valid, else
// the SEED member's own query window (种子窗). Remaining members follow in
// member order (seed-deterministic), so a rankless/windowless seat degrades
// to the seed-side copy.
//
// Fail-open lanes (Σ direction — 终判① 禁一刀切清零; mirrors the union
// channel's fail-open): a member without a window identity or without a valid
// occurrence interval always counts and is never skipped (the proof needs
// both sides' typed intervals — an unprovable double count keeps the honest
// Σ, e.g. the §21 CWD windowed-no-interval shape); same-slot overlapping
// members are DISTINCT facts (E9/E10 strict pin) and both count; a SKIPPED
// re-measurement's interval is never recorded, so it cannot chain-knock a
// third member it merely touches (dedup only against positively counted
// occurrences' footprints). Single-window and disjoint multi-window groups
// return all-true, and the caller's Σ loop then runs over the same members in
// the same order — byte-identical to the pre-dedup Σ (F6(a) legal Σ pins).
func traceCausalProjectionPeriodicDiscountCounted(nodes []TraceCausalProjectionNode, members []int, union traceCausalProjectionUnionOutcome, seatStartTs, seatEndTs float64) []bool {
	counted := make([]bool, len(members))
	for k := range counted {
		counted[k] = true
	}
	if len(union.slots) < 2 || len(union.slotOf) != len(members) {
		return counted
	}
	seatSlot := -1
	if traceCausalProjectionIntervalValid(seatStartTs, seatEndTs) {
		for si, w := range union.slots {
			if math.Abs(w.StartTs-seatStartTs) <= traceCausalProjectionFullWindowSameWindowToleranceS &&
				math.Abs(w.EndTs-seatEndTs) <= traceCausalProjectionFullWindowSameWindowToleranceS {
				seatSlot = si
				break
			}
		}
	}
	order := make([]int, 0, len(members))
	if seatSlot >= 0 {
		for k := range members {
			if union.slotOf[k] == seatSlot {
				order = append(order, k)
			}
		}
	}
	for k := range members {
		if seatSlot >= 0 && union.slotOf[k] == seatSlot {
			continue
		}
		order = append(order, k)
	}
	countedPositions := make([]int, 0, len(members))
	for _, k := range order {
		node := nodes[members[k]]
		if union.slotOf[k] < 0 || !traceCausalProjectionIntervalValid(node.StartTs, node.EndTs) {
			continue // fail-open: stays counted; proves nothing, nothing proves against it
		}
		skip := false
		for _, c := range countedPositions {
			if union.slotOf[c] != union.slotOf[k] && traceCausalProjectionSpansOverlap(nodes[members[c]], node) {
				skip = true
				break
			}
		}
		if skip {
			counted[k] = false
			continue
		}
		countedPositions = append(countedPositions, k)
	}
	return counted
}

// traceCausalProjectionDisplayValue is the merged-member display value the R2
// sum/min/max accounting keys on (projected ms, cumulative fallback).
func traceCausalProjectionDisplayValue(node TraceCausalProjectionNode) float64 {
	if node.ImpactMS > 0 {
		return node.ImpactMS
	}
	return node.CumulativeImpactMS
}

// traceCausalProjectionSameValueFoldMembers is the DIAG A1 tie collector for
// the cross-thread take-MAX folds (§28.11-3(a), G12): members whose display
// value ties the fold's published MAX to the µs (strict
// |v−max| < TraceCausalProjectionSameValueTieMS) are returned as (subject,
// line-range) witnesses, capped at traceCausalProjectionSameValueMemberCap.
// nil unless AT LEAST TWO members with a NON-EMPTY subject tie — a single max
// member is the normal take-MAX shape, not a double-attribution suspicion,
// and a subjectless member (复核 P3-1 fold-of-fold shape: the hop cap
// re-folding a subjectless bucket-fold row whose inner max ties the outer
// max) cannot be a nameable witness — it is SKIPPED, keeping this collector
// symmetric with the wire parser's empty-subject discard arm
// (traceCausalProjectionParseSameValueMembers) and the audit face free of
// degenerate "same_value_members=,xxx" entries. Pure read: callers attach
// the roster as disclosure and MUST NOT let it touch any fold value.
func traceCausalProjectionSameValueFoldMembers(members []TraceCausalProjectionNode, maxMS float64) []TraceCausalProjectionSameValueMember {
	if maxMS <= 0 {
		return nil
	}
	var out []TraceCausalProjectionSameValueMember
	for _, member := range members {
		subject := strings.TrimSpace(member.Subject)
		if subject == "" {
			continue
		}
		v := traceCausalProjectionDisplayValue(member)
		if v <= 0 || math.Abs(v-maxMS) >= TraceCausalProjectionSameValueTieMS {
			continue
		}
		if len(out) < traceCausalProjectionSameValueMemberCap {
			out = append(out, TraceCausalProjectionSameValueMember{
				Subject:   subject,
				LineStart: member.LineStart,
				LineEnd:   member.LineEnd,
			})
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// --- R3: unknown-impact-point background folding -----------------------------

// traceCausalProjectionFoldUnknownBackground keeps the top-K background rows
// whose impact point is the unknown-thread sentinel and folds the rest into a
// single subjectless aggregate row (rendered as “其余 N 项合并”). Background
// rows with a REAL object (a cause word or resolved peer) are never folded.
//
// V3 (customer revisit 2026-07-03): the fold members are DIFFERENT threads, so
// their wall-clock projections must never be summed — six whole-window 101ms
// background threads once published as a 606ms/600% fold row. The fold's
// ImpactMS/CumulativeImpactMS carry the member MAX; MergedMinMS/MergedMaxMS
// keep the lossless per-member range and MergedCount the member count.
func traceCausalProjectionFoldUnknownBackground(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	var unknown []int
	for i, node := range nodes {
		if node.MergedCount > 1 {
			continue
		}
		if !traceCausalProjectionKnownSubject(node.Object) && strings.TrimSpace(node.Object) != "" {
			unknown = append(unknown, i)
		}
	}
	if len(unknown) < traceCausalProjectionUnknownBackgroundMin {
		return nodes
	}
	// Bucket order is already impact-major (classifiedLess); keep the first K.
	fold := unknown[traceCausalProjectionUnknownBackgroundKeep:]
	foldSet := make(map[int]bool, len(fold))
	for _, idx := range fold {
		foldSet[idx] = true
	}
	aggregate := TraceCausalProjectionNode{
		Role:           nodes[fold[0]].Role,
		Predicate:      nodes[fold[0]].Predicate,
		Object:         nodes[fold[0]].Object,
		ChainRelevance: "background",
		Causality:      nodes[fold[0]].Causality,
	}
	// F3 support: the fold row keeps the members' typed dominant state ONLY when
	// every member carries the same canonical StateKind (strict unanimity — any
	// divergence or an empty member leaves the fold stateless). The renderer's
	// whole-window idle annotation is gated on the wait-family StateKind, so a
	// fold of uniform whole-window sleepers legitimately keeps the tag while a
	// mixed or stateless fold never fabricates one.
	foldState := nodes[fold[0]].StateKind
	for _, idx := range fold {
		if traceCausalProjectionCanonicalNode(nodes[idx].StateKind) !=
			traceCausalProjectionCanonicalNode(foldState) {
			foldState = ""
			break
		}
	}
	aggregate.StateKind = strings.TrimSpace(foldState)
	var minMS, maxMS float64
	absorbed := map[string]bool{}
	for _, idx := range fold {
		member := nodes[idx]
		// Keep the folded rows' thread names visible on the subjectless fold
		// row — the renderer's "其余 N 项合并" line names them from here.
		traceCausalProjectionAppendMergedSubject(&aggregate, member.Subject)
		display := member.ImpactMS
		if display <= 0 {
			display = member.CumulativeImpactMS
		}
		if minMS == 0 || (display > 0 && display < minMS) {
			minMS = display
		}
		if display > maxMS {
			maxMS = display
			// RUN2FIX-A 件2: the ▒/◇ stanza fold names its MAX member too —
			// same all-or-nothing carriers as the on-chain constructor
			// (traceCausalProjectionOverflowFoldRow), 宁漏勿假 on unknown
			// subjects.
			if traceCausalProjectionKnownSubject(member.Subject) {
				aggregate.MergedMaxSubject = strings.TrimSpace(member.Subject)
				aggregate.MergedMaxStateKind = strings.TrimSpace(member.StateKind)
			} else {
				aggregate.MergedMaxSubject = ""
				aggregate.MergedMaxStateKind = ""
			}
		}
		appendID := func(raw string) {
			raw = strings.TrimSpace(raw)
			if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
				return
			}
			absorbed[traceCausalProjectionCanonicalNode(raw)] = true
			if aggregate.EvidenceID == "" {
				aggregate.EvidenceID = raw
				return
			}
			aggregate.MergedEvidenceIDs = append(aggregate.MergedEvidenceIDs, raw)
		}
		appendID(member.EvidenceID)
		for _, id := range member.MergedEvidenceIDs {
			appendID(id)
		}
		if member.LineStart > 0 && (aggregate.LineStart <= 0 || member.LineStart < aggregate.LineStart) {
			aggregate.LineStart = member.LineStart
		}
		if member.LineEnd > aggregate.LineEnd {
			aggregate.LineEnd = member.LineEnd
		}
		if member.Confidence > 0 && (aggregate.Confidence <= 0 || member.Confidence < aggregate.Confidence) {
			aggregate.Confidence = member.Confidence
		}
	}
	aggregate.MergedCount = len(fold)
	aggregate.MergedMinMS = minMS
	aggregate.MergedMaxMS = maxMS
	// V3: member MAX, never a cross-thread wall-clock sum (see the fold doc).
	aggregate.ImpactMS = maxMS
	aggregate.CumulativeImpactMS = maxMS
	// COV §24.9 D-1: TargetImpactMS keeps the same member-MAX discipline (the
	// fold row starts empty, so without this the typed caliber would silently
	// vanish on fold — MAX is the honest lower bound, never a cross-thread Σ).
	targetImpact := 0.0
	for _, idx := range fold {
		if v := nodes[idx].TargetImpactMS; v > targetImpact {
			targetImpact = v
		}
	}
	aggregate.TargetImpactMS = targetImpact
	// DIAG A1 (§28.11-3(a)): µs-tie disclosure at the take-MAX merge point —
	// zero weight, values above are already final.
	members := make([]TraceCausalProjectionNode, 0, len(fold))
	for _, idx := range fold {
		members = append(members, nodes[idx])
	}
	aggregate.SameValueMembers = traceCausalProjectionSameValueFoldMembers(members, maxMS)
	out := make([]TraceCausalProjectionNode, 0, len(nodes)-len(fold)+1)
	for i, node := range nodes {
		if foldSet[i] {
			continue
		}
		out = append(out, node)
	}
	return append(out, aggregate)
}

// --- post-aggregation ordering ----------------------------------------------

// traceCausalProjectionResortAfterAggregation restores impact-major order
// inside each bucket after R2 sums may have changed magnitudes. It reuses the
// build-time comparators; the R3 fold row is subjectless and deliberately sorts
// by its published magnitude (the member max since V3) like any other row.
func traceCausalProjectionResortAfterAggregation(out *TraceCausalProjection) {
	pathIndex := traceCausalProjectionPathIndex(out.WakeupPath)
	sort.SliceStable(out.PrimaryRootCauses, func(i, j int) bool {
		return traceCausalProjectionPrimaryLess(out.PrimaryRootCauses[i], out.PrimaryRootCauses[j], pathIndex)
	})
	sort.SliceStable(out.SupportingHops, func(i, j int) bool {
		return traceCausalProjectionHopLess(out.SupportingHops[i], out.SupportingHops[j], pathIndex)
	})
	nodes := out.OnChainCauses
	sort.SliceStable(nodes, func(i, j int) bool {
		return traceCausalProjectionClassifiedLess(nodes[i], nodes[j], pathIndex)
	})
	// RNB-1 D1 修复轮 (§29.88 复核, 2026-07-14): the ◇/▒ context buckets keep
	// the two-class order THROUGH the post-aggregation resort — re-sorting
	// them with the classified comparator here silently restored the in-path
	// -first preemption right before the fold cap ran, so the fold membership
	// was again decided by proximity instead of value (donghu 2955: the
	// 16.013/10.571 remainder/demoted seats folded while 1.462/1.252 kept
	// individual rows).
	out.AdjacentCauses = traceCausalProjectionSortContextBucket(out.AdjacentCauses, pathIndex)
	out.BackgroundCauses = traceCausalProjectionSortContextBucket(out.BackgroundCauses, pathIndex)
}
