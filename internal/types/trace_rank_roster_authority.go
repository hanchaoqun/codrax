package types

import (
	"sort"
	"strings"
)

type TraceRankRosterSeat struct {
	Rank                     int
	Tier                     string
	Type                     string
	Subject                  string
	EffectiveImpactMS        float64
	EffectiveImpactPublished bool
	ChainRelevance           string
	FixDirection             string
	EvidenceID               string
}

type TraceRankRosterAuthority struct {
	ArtifactLabel          string
	BoardTarget            string
	BoardParamsFingerprint string
	BoardChannel           string
	WindowStartTs          float64
	WindowEndTs            float64
	Seats                  []TraceRankRosterSeat
	Complete               bool
	Status                 string
}

// BuildTraceRankRosterAuthorities projects the complete pre-cap ranked-seat
// carrier into per-board answer-writing authority. It consumes only compiled
// typed projection fields and never inspects request or model prose.
func BuildTraceRankRosterAuthorities(set TraceCausalProjectionSet) []TraceRankRosterAuthority {
	var out []TraceRankRosterAuthority
	for _, projection := range set.Projections {
		nodes := projection.RankedSeats
		if len(nodes) == 0 {
			// Compatibility for callers/tests that hand-build projections.
			nodes = traceCausalProjectionCollectRankedSeats(
				projection.PrimaryRootCauses,
				projection.OnChainCauses,
				projection.AdjacentCauses,
				projection.BackgroundCauses,
			)
		}
		byKey := map[string]int{}
		for _, node := range nodes {
			if node.Rank <= 0 {
				continue
			}
			boardChannel := traceRankRosterBoardChannel(node)
			key := traceCausalProjectionRankBoardIdentityKey(node) + "\x00" + boardChannel
			idx, ok := byKey[key]
			if !ok {
				start, end := node.RankQueryWindowStartTs, node.RankQueryWindowEndTs
				if !traceCausalProjectionIntervalValid(start, end) {
					start, end = node.QueryWindowStartTs, node.QueryWindowEndTs
				}
				idx = len(out)
				byKey[key] = idx
				out = append(out, TraceRankRosterAuthority{
					ArtifactLabel:          strings.TrimSpace(projection.ArtifactLabel),
					BoardTarget:            strings.TrimSpace(node.RankBoardTarget),
					BoardParamsFingerprint: strings.TrimSpace(node.RankBoardParamsFingerprint),
					BoardChannel:           boardChannel,
					WindowStartTs:          start,
					WindowEndTs:            end,
				})
			}
			out[idx].Seats = append(out[idx].Seats, TraceRankRosterSeat{
				Rank:                     node.Rank,
				Tier:                     strings.TrimSpace(node.Tier),
				Type:                     firstNonEmptyTraceRankRosterString(node.TypeToken, node.Object, node.Predicate, "unknown"),
				Subject:                  strings.TrimSpace(node.Subject),
				EffectiveImpactMS:        node.EffectiveImpactMS,
				EffectiveImpactPublished: node.EffectiveImpactPublished,
				ChainRelevance:           strings.TrimSpace(node.ChainRelevance),
				FixDirection:             strings.TrimSpace(node.FixDirection),
				EvidenceID:               strings.TrimSpace(node.EvidenceID),
			})
		}
	}
	for i := range out {
		sort.SliceStable(out[i].Seats, func(a, b int) bool {
			if out[i].Seats[a].Rank != out[i].Seats[b].Rank {
				return out[i].Seats[a].Rank < out[i].Seats[b].Rank
			}
			if out[i].Seats[a].EffectiveImpactMS != out[i].Seats[b].EffectiveImpactMS {
				return out[i].Seats[a].EffectiveImpactMS > out[i].Seats[b].EffectiveImpactMS
			}
			return out[i].Seats[a].EvidenceID < out[i].Seats[b].EvidenceID
		})
		out[i].Complete, out[i].Status = traceRankRosterCompleteness(out[i].Seats)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ArtifactLabel != out[j].ArtifactLabel {
			return out[i].ArtifactLabel < out[j].ArtifactLabel
		}
		if out[i].WindowStartTs != out[j].WindowStartTs {
			return out[i].WindowStartTs < out[j].WindowStartTs
		}
		if out[i].BoardTarget != out[j].BoardTarget {
			return out[i].BoardTarget < out[j].BoardTarget
		}
		if out[i].BoardChannel != out[j].BoardChannel {
			return out[i].BoardChannel < out[j].BoardChannel
		}
		return out[i].BoardParamsFingerprint < out[j].BoardParamsFingerprint
	})
	return out
}

func traceRankRosterBoardChannel(node TraceCausalProjectionNode) string {
	if channel := strings.TrimSpace(node.ChainRelevance); channel != "" {
		return channel
	}
	return "unspecified"
}

func traceRankRosterCompleteness(seats []TraceRankRosterSeat) (bool, string) {
	if len(seats) == 0 {
		return false, "absent"
	}
	for i, seat := range seats {
		want := i + 1
		if seat.Rank < want {
			return false, "duplicate_rank"
		}
		if seat.Rank > want {
			return false, "rank_gap"
		}
		if !seat.EffectiveImpactPublished || seat.EffectiveImpactMS <= 0 {
			return false, "rank_value_unavailable"
		}
	}
	return true, "complete"
}

func firstNonEmptyTraceRankRosterString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
