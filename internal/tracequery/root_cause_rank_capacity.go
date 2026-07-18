package tracequery

import (
	"fmt"
	"strings"
)

// Rank-0 rows are lossless side-lane disclosures, not election candidates.
// They must remain visible without consuming the root_cause_rank candidate
// budget. A separate small cap bounds their wire cost. When both typed lanes
// are populated, split the seats evenly so a flood of target symptoms cannot
// hide every data-gap disclosure (or vice versa).
const rootCauseRankZeroSeatDisclosureCap = 4
const rootCauseRankZeroSeatPerLaneReservedCap = rootCauseRankZeroSeatDisclosureCap / 2

// rootCauseRankRemainderSideCap bounds the RSPA ◇ remainder sub-lane
// (§29.61.10): a remainder seat is the complement half of an on-board ⛓
// anchored account — the same-source bipartition (identity/relation sentence)
// needs both halves visible, so remainder seats ride the side lane instead of
// competing for candidate seats (they'd otherwise sort behind every on-chain
// row and silently truncate — 零静默消失). Bounded by the migrated-thread
// count in practice; the cap is a defensive wire bound (overflow stays
// disclosed through the side-row caveat counts).
const rootCauseRankRemainderSideCap = 8

// rootCauseRankDemotedSideCap bounds the RNB-1 R4 lane-demoted sub-lane
// (§29.88 R4 / D1 修复轮, 2026-07-14): a credential-lane-demoted seat
// (affinity satellite / inversion-retyped seat / unprovable low_frequency
// satellite) is ◇-family like the remainder seats — as a plain candidate it
// sorts behind every on-chain row and structurally dies at the candidate cap,
// which made the「值零动,通道位归位」promise false on the WIRE (donghu 2955
// witness: all three demoted rows 47.678/22.408/16.013 vanished). Mirrored
// cap, same overflow disclosure through the side-row caveat counts.
const rootCauseRankDemotedSideCap = rootCauseRankRemainderSideCap

// rootCauseRankSelfSideCap bounds the ELIM-SELF-FIX 件2 selfSide sub-lane
// (§29.93.1 Form-2 + §29.93.3 全族收编, 2026-07-15): in a degenerate window
// (no cross-thread chain) the sort is channel-blind and the candidate cap
// can kill EVERY one of the target's own on-chain seats while background
// rows fill the board (tieba head-window witness: the self running /
// runnable / io_wait seats died at pre-truncation positions 18/20/24 behind
// 12 background rows) — the RNB-1 D1 death shape, so the same bounded side
// lane protects them. ALL self families ride it (running / runnable / D-IO /
// IO facets / semantic — the predicate is subject+channel+eff, never a
// token list).
// EVOLUTION RECORD (终判① §29.96.2, 2026-07-15): cap 4 → 6. The §29.93.1
// cap 4 (照抄 D1 模式) was in tension with the §29.93.3 全族收编 zero-
// silent-disappearance promise: the self lane serves ≥5 families (four
// state families + semantic), so a full-family overflow at cap 4 silently
// dropped the 5th/6th seat. 6 = four state families + semantic + 1 headroom
// (a family may hold two seats, e.g. the D-state/io_wait pair).
const rootCauseRankSelfSideCap = 6

func rootEvidenceRankSeatKey(root RootEvidence) string {
	return fmt.Sprintf("%s|%s|%d|%d|%.9f", strings.TrimSpace(root.Type), threadKey(root.Thread), root.LineStart, root.LineEnd, root.DurationMs)
}

// rootEvidenceDStateTwinFamilyKey is the mutation-invariant seat key for the
// d_state/io_wait RootEvidence family (ENG audit #65, §29.25 处置委托
// 2026-07-10). expandChain mutates the D-state twin IN PLACE after
// construction when a sched_blocked_reason resolves (Type→"io_wait" iff
// reason.IOWait>0, LineEnd→reason.Line whenever the reason row is found), so
// the exact rootEvidenceRankSeatKey diverges from the seed minted off the
// unmutated constructor output and the same physical occurrence seated twice.
// Thread, LineStart and DurationMs are never touched by that mutation and
// identify the occurrence; the family fold applies only to the two D-state
// family type tokens, so no other RootEvidence lane can be over-suppressed.
// The "dstate_twin" prefix cannot collide with rootEvidenceRankSeatKey (no
// type token spells it and the field counts differ).
func rootEvidenceDStateTwinFamilyKey(root RootEvidence) (string, bool) {
	switch strings.TrimSpace(root.Type) {
	case "d_state_or_io_wait", "io_wait":
		return fmt.Sprintf("dstate_twin|%s|%d|%.9f", threadKey(root.Thread), root.LineStart, root.DurationMs), true
	}
	return "", false
}

func rootCauseRankItemIsZeroSeatDisclosure(item RootCauseRankItem) bool {
	if item.Type == "trace_gap" {
		return true
	}
	if rootCauseEffectiveImpactMs(item) <= 0 {
		return true
	}
	// V2-P0 行级尺守卫 (design §6.1, 2026-07-12): a caliber-side row is
	// structurally Rank=0 (the ordinal guard in assignRootCauseRanksAndTiers
	// keys on the same shared registry arm), so it rides the bounded
	// side-disclosure lane instead of consuming a candidate board seat.
	if rootCauseOrdinalChannel(item) != rootCauseOrdinalChannelBackground &&
		CausalTokenCaliberSideClass(item.Type) != CausalCaliberSideNone {
		return true
	}
	return item.SubjectIsAnalysisTarget && rootCauseItemIsTargetWaitSymptomType(item)
}

// truncateRootCauseRankCandidatesAndSideRows applies `limit` only to rows that
// will receive a Rank>0 election seat. Rank-0 rows use an independent bounded
// disclosure lane and are appended after the sorted board; projection
// rendering places them in their typed self/data-gap sections, so this does
// not change candidate order or ordinals.
func truncateRootCauseRankCandidatesAndSideRows(items []RootCauseRankItem, limit int) (out []RootCauseRankItem, candidateTotal, candidateEmitted, sideTotal, sideEmitted int) {
	if len(items) == 0 {
		return nil, 0, 0, 0, 0
	}
	candidates := make([]RootCauseRankItem, 0, len(items))
	targetSide := make([]RootCauseRankItem, 0, rootCauseRankZeroSeatDisclosureCap)
	gapSide := make([]RootCauseRankItem, 0, rootCauseRankZeroSeatDisclosureCap)
	// V2-P0 (design §6.1, 2026-07-12): ⌗ caliber-side rows ride their OWN
	// bounded sub-lane — routing them into targetSide would crowd the
	// self-symptom/context disclosures out of the shared cap (donghu witness:
	// the pacing_idle context row evicted by two IO caliber rows).
	caliberSide := make([]RootCauseRankItem, 0, rootCauseRankZeroSeatDisclosureCap)
	remainderSide := make([]RootCauseRankItem, 0, rootCauseRankRemainderSideCap)
	demotedSide := make([]RootCauseRankItem, 0, rootCauseRankDemotedSideCap)
	for _, item := range items {
		if item.ChainAnchorRemainderSeat {
			// RSPA ◇ remainder seats: complement halves of on-board anchored
			// accounts — side lane, never a candidate seat (see cap comment).
			sideTotal++
			remainderSide = append(remainderSide, item)
			continue
		}
		if item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat ||
			item.GatedShareConstituentSeat {
			// RNB-1 R4 lane-demoted seats (D1 修复轮): ◇-family disclosure
			// rows — mirrored side lane so「值零动,通道位归位」holds on the
			// published wire, never a candidate seat. XLANE-1 件1: the
			// represented-demoted satellite rides the same side lane (same
			// value-untouched ◇ disclosure family, different credential story).
			// LEVELMERGE-1 件2: the gated-share CONSTITUENT row (the A share
			// of the interval-accounting split) is ◇-family too — its value
			// is already carried by the inversion seat's gated composite, so
			// it discloses on the side lane and never consumes a candidate
			// seat (the residual B row keeps the competing aggregate seat).
			sideTotal++
			demotedSide = append(demotedSide, item)
			continue
		}
		if rootCauseRankItemIsZeroSeatDisclosure(item) {
			sideTotal++
			if item.Type == "trace_gap" {
				gapSide = append(gapSide, item)
			} else if rootCauseOrdinalChannel(item) != rootCauseOrdinalChannelBackground &&
				CausalTokenCaliberSideClass(item.Type) != CausalCaliberSideNone {
				caliberSide = append(caliberSide, item)
			} else {
				targetSide = append(targetSide, item)
			}
			continue
		}
		candidates = append(candidates, item)
	}
	candidateTotal = len(candidates)
	// ELIM-SELF-FIX 件2 selfSide (§29.93.1 修向② + §29.93.3 全族收编): the
	// analysis target's own on-chain seats that die at the candidate cap are
	// preserved on a bounded side lane instead of vanishing (零静默消失的自身
	// 特化). Election candidates that WIN a board seat are untouched — the
	// lane holds only the strict-truncation overflow, in sorted order.
	var selfSide []RootCauseRankItem
	if limit > 0 && len(candidates) > limit {
		for _, item := range candidates[limit:] {
			if item.SubjectIsAnalysisTarget && rootCauseItemIsOnChain(item) &&
				rootCauseEffectiveImpactMs(item) > 0 {
				sideTotal++
				if len(selfSide) < rootCauseRankSelfSideCap {
					selfSide = append(selfSide, item)
				}
			}
		}
		candidates = truncateRootCauseRankItemsStrict(candidates, limit)
	}
	candidateEmitted = len(candidates)
	side := make([]RootCauseRankItem, 0, rootCauseRankZeroSeatDisclosureCap)
	if len(targetSide) > 0 && len(gapSide) > 0 {
		targetLimit := min(rootCauseRankZeroSeatPerLaneReservedCap, len(targetSide))
		gapLimit := min(rootCauseRankZeroSeatPerLaneReservedCap, len(gapSide))
		side = append(side, targetSide[:targetLimit]...)
		side = append(side, gapSide[:gapLimit]...)
	} else if len(targetSide) > 0 {
		targetLimit := min(rootCauseRankZeroSeatDisclosureCap, len(targetSide))
		side = append(side, targetSide[:targetLimit]...)
	} else {
		gapLimit := min(rootCauseRankZeroSeatDisclosureCap, len(gapSide))
		side = append(side, gapSide[:gapLimit]...)
	}
	if len(caliberSide) > 0 {
		caliberLimit := min(rootCauseRankZeroSeatDisclosureCap, len(caliberSide))
		side = append(side, caliberSide[:caliberLimit]...)
	}
	if len(remainderSide) > 0 {
		remainderLimit := min(rootCauseRankRemainderSideCap, len(remainderSide))
		side = append(side, remainderSide[:remainderLimit]...)
	}
	if len(demotedSide) > 0 {
		demotedLimit := min(rootCauseRankDemotedSideCap, len(demotedSide))
		side = append(side, demotedSide[:demotedLimit]...)
	}
	// selfSide is already bounded at collection time (sorted-order prefix).
	side = append(side, selfSide...)
	sideEmitted = len(side)
	out = make([]RootCauseRankItem, 0, candidateEmitted+sideEmitted)
	out = append(out, candidates...)
	out = append(out, side...)
	return out, candidateTotal, candidateEmitted, sideTotal, sideEmitted
}
