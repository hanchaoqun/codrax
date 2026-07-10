package tracequery

// rank_cross_type_recon.go — B4 exact cross-type rank-seat reconciliation
// (external audit ruling accepted 2026-07-10).
//
// computeIOBurstEpisodes deliberately projects every DStateTop segment into
// an io_burst_episode resource view. buildRootCauseRankFrom then used to mint
// both that resource view and the source d_state_or_io_wait state view as two
// competing rank rows. On the production witness they had the same numeric
// thread, query window, physical interval and evidence lines (1.062ms), so
// one physical wait occupied two board seats.
//
// This pass is intentionally an adjudicated exact-match table, not a generic
// similarity fold. d_state_or_io_wait owns the seat because it is the formal
// scheduler-state cause lane; io_burst_episode remains a lossless supporting
// observation in RootCauseRankResult.AbsorbedItems. Any missing or unequal
// precise dimension fails open to the former two-row publication:
//
//   1. numeric TID identity (both PID fields positive and exactly equal);
//   2. exact adjudicated type+producer pair;
//   3. exact typed query-window endpoints and chain lane;
//   4. exact physical interval AND exact source line span.
//
// Values, labels, confidence and Summary prose never participate.

import (
	"fmt"
	"strings"
)

type crossTypeRankSeatReconSpec struct {
	absorberType   string
	absorbedType   string
	absorberSource string
	absorbedSource string
}

// Keyed by the absorbed type. Closed adjudication set: adding a pair changes
// rank semantics and requires a production witness plus an explicit ruling.
var crossTypeRankSeatReconPairs = map[string]crossTypeRankSeatReconSpec{
	"io_burst_episode": {
		absorberType:   "d_state_or_io_wait",
		absorbedType:   "io_burst_episode",
		absorberSource: "window_stats",
		absorbedSource: "window_stats.io_burst_episodes",
	},
}

// reconcileExactCrossTypeRankSeats reclaims duplicate rank/capacity seats
// while retaining every absorbed observation on the dedicated lossless
// carrier. Reset-first recomputation makes the pass idempotent.
func reconcileExactCrossTypeRankSeats(rank *RootCauseRankResult) {
	if rank == nil {
		return
	}
	pool := make([]RootCauseRankItem, 0, len(rank.Items)+len(rank.AbsorbedItems))
	pool = append(pool, rank.Items...)
	pool = append(pool, rank.AbsorbedItems...)
	rank.AbsorbedItems = nil
	if len(pool) < 2 {
		rank.Items = pool
		return
	}

	// Clear B4-owned markers only. G1 may already own RankFamilyKey on an
	// io_latency family; AbsorbedRankRows is the precise ownership signal that
	// prevents this pass from erasing another reconciliation lane.
	for i := range pool {
		if pool[i].AbsorbedRankRows > 0 {
			pool[i].RankFamilyKey = ""
		}
		pool[i].AbsorbedRankRows = 0
		if pool[i].AbsorbedByRankFamily {
			pool[i].AbsorbedByRankFamily = false
			pool[i].AbsorbedIntoFamily = ""
			if pool[i].Tier == RootCauseTierAbsorbed {
				pool[i].Tier = ""
			}
			pool[i].Rank = 0
			pool[i].BackgroundRank = 0
		}
	}

	absorbed := make(map[int]int) // absorbed index -> unique absorber index
	for i := range pool {
		spec, adjudicated := crossTypeRankSeatReconPairs[strings.TrimSpace(pool[i].Type)]
		if !adjudicated {
			continue
		}
		matched := -1
		for j := range pool {
			if i == j || !crossTypeRankSeatExactMatch(pool[j], pool[i], spec) {
				continue
			}
			if matched >= 0 {
				// More than one possible owner is not a precise identity. Keep the
				// original two-seat publication rather than electing arbitrarily.
				matched = -2
				break
			}
			matched = j
		}
		if matched >= 0 {
			absorbed[i] = matched
		}
	}
	if len(absorbed) == 0 {
		rank.Items = pool
		return
	}

	active := make([]RootCauseRankItem, 0, len(pool)-len(absorbed))
	lossless := make([]RootCauseRankItem, 0, len(absorbed))
	keys := make(map[int]string)
	counts := make(map[int]int)
	for absorbedIndex, absorberIndex := range absorbed {
		key := keys[absorberIndex]
		if key == "" {
			key = crossTypeRankSeatReconKey(pool[absorberIndex], pool[absorbedIndex])
			keys[absorberIndex] = key
		}
		item := pool[absorbedIndex]
		item.Rank = 0
		item.BackgroundRank = 0
		item.Tier = RootCauseTierAbsorbed
		item.AbsorbedByRankFamily = true
		item.AbsorbedIntoFamily = key
		lossless = append(lossless, item)
		counts[absorberIndex]++
	}
	for i := range pool {
		if _, drop := absorbed[i]; drop {
			continue
		}
		item := pool[i]
		if count := counts[i]; count > 0 {
			item.RankFamilyKey = keys[i]
			item.AbsorbedRankRows = count
		}
		active = append(active, item)
	}
	rank.Items = active
	rank.AbsorbedItems = lossless
}

func crossTypeRankSeatExactMatch(absorber, candidate RootCauseRankItem, spec crossTypeRankSeatReconSpec) bool {
	if absorber.Type != spec.absorberType || candidate.Type != spec.absorbedType {
		return false
	}
	if absorber.Source != spec.absorberSource || candidate.Source != spec.absorbedSource {
		return false
	}
	// Numeric TID only. Comm-only equality is deliberately insufficient.
	if absorber.Thread.PID <= 0 || candidate.Thread.PID <= 0 || absorber.Thread.PID != candidate.Thread.PID {
		return false
	}
	// A trace clock may legitimately start at t=0. End>start is the typed
	// presence/validity signal; zero/zero remains the absent representation.
	if absorber.StatsWindowEndTs <= absorber.StatsWindowStartTs ||
		candidate.StatsWindowStartTs != absorber.StatsWindowStartTs ||
		candidate.StatsWindowEndTs != absorber.StatsWindowEndTs {
		return false
	}
	if rootCauseFamilyFoldLaneKey(absorber) != rootCauseFamilyFoldLaneKey(candidate) {
		return false
	}
	if absorber.EndTs <= absorber.StartTs ||
		candidate.StartTs != absorber.StartTs || candidate.EndTs != absorber.EndTs {
		return false
	}
	if absorber.LineStart <= 0 || absorber.LineEnd < absorber.LineStart ||
		candidate.LineStart != absorber.LineStart || candidate.LineEnd != absorber.LineEnd {
		return false
	}
	return true
}

func crossTypeRankSeatReconKey(absorber, absorbed RootCauseRankItem) string {
	return fmt.Sprintf("rank_pair:%s>%s|pid:%d|%s|window:%.6f..%.6f|interval:%.6f..%.6f|lines:%d-%d",
		absorber.Type, absorbed.Type, absorber.Thread.PID,
		rootCauseFamilyFoldLaneKey(absorber),
		absorber.StatsWindowStartTs, absorber.StatsWindowEndTs,
		absorber.StartTs, absorber.EndTs,
		absorber.LineStart, absorber.LineEnd)
}
