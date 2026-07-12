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
	for _, item := range items {
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
	if limit > 0 && len(candidates) > limit {
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
	sideEmitted = len(side)
	out = make([]RootCauseRankItem, 0, candidateEmitted+sideEmitted)
	out = append(out, candidates...)
	out = append(out, side...)
	return out, candidateTotal, candidateEmitted, sideTotal, sideEmitted
}
