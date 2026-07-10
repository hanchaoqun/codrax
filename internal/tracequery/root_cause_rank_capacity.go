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

func rootCauseRankItemIsZeroSeatDisclosure(item RootCauseRankItem) bool {
	if item.Type == "trace_gap" {
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
	for _, item := range items {
		if rootCauseRankItemIsZeroSeatDisclosure(item) {
			sideTotal++
			if item.Type == "trace_gap" {
				gapSide = append(gapSide, item)
			} else {
				targetSide = append(targetSide, item)
			}
			continue
		}
		candidates = append(candidates, item)
	}
	candidateTotal = len(candidates)
	if limit > 0 && len(candidates) > limit {
		candidates = truncateRootCauseRankItemsWithSemanticSeats(candidates, limit)
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
	sideEmitted = len(side)
	out = make([]RootCauseRankItem, 0, candidateEmitted+sideEmitted)
	out = append(out, candidates...)
	out = append(out, side...)
	return out, candidateTotal, candidateEmitted, sideTotal, sideEmitted
}
