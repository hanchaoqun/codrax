package types

import "testing"

func traceRankRosterTestNode(id, target, params string, start, end float64, rank int, typ, subject string, impact float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		EvidenceID:                 id,
		Rank:                       rank,
		Tier:                       "primary",
		TypeToken:                  typ,
		Subject:                    subject,
		EffectiveImpactMS:          impact,
		EffectiveImpactPublished:   true,
		ChainRelevance:             "on_chain",
		FixDirection:               "lock_priority",
		RankBoardTarget:            target,
		RankBoardParamsFingerprint: params,
		RankQueryWindowStartTs:     start,
		RankQueryWindowEndTs:       end,
	}
}

func TestBuildTraceRankRosterAuthoritiesSeparatesBoardsAndKeepsOrder(t *testing.T) {
	a1 := traceRankRosterTestNode("a1", "target-a", "params-a", 1, 2, 1, "running", "thread-a", 8)
	a2 := traceRankRosterTestNode("a2", "target-a", "params-a", 1, 2, 2, "io_wait", "thread-b", 4)
	b1 := traceRankRosterTestNode("b1", "target-b", "params-b", 1, 2, 1, "runnable_wait", "thread-c", 6)
	context := traceRankRosterTestNode("ctx", "target-a", "params-a", 1, 2, 0, "binder_wait", "thread-d", 1.409)
	set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{
		ArtifactLabel: "trace.systrace",
		RankedSeats:   []TraceCausalProjectionNode{a2, context, b1, a1},
	}}}

	got := BuildTraceRankRosterAuthorities(set)
	if len(got) != 2 {
		t.Fatalf("expected two independent rank boards, got %+v", got)
	}
	if got[0].BoardTarget != "target-a" || !got[0].Complete || got[0].Status != "complete" ||
		len(got[0].Seats) != 2 || got[0].Seats[0].Rank != 1 || got[0].Seats[1].Rank != 2 {
		t.Fatalf("board A roster drifted: %+v", got[0])
	}
	if got[1].BoardTarget != "target-b" || !got[1].Complete || len(got[1].Seats) != 1 ||
		got[1].Seats[0].Subject != "thread-c" {
		t.Fatalf("board B roster drifted: %+v", got[1])
	}
	for _, board := range got {
		for _, seat := range board.Seats {
			if seat.Type == "binder_wait" || seat.Rank <= 0 {
				t.Fatalf("unranked context row leaked into ordinal roster: %+v", seat)
			}
		}
	}
}

func TestBuildTraceRankRosterAuthoritiesReportsRankGap(t *testing.T) {
	one := traceRankRosterTestNode("one", "target", "params", 1, 2, 1, "running", "a", 8)
	three := traceRankRosterTestNode("three", "target", "params", 1, 2, 3, "io_wait", "b", 4)
	got := BuildTraceRankRosterAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{{
		RankedSeats: []TraceCausalProjectionNode{three, one},
	}}})
	if len(got) != 1 || got[0].Complete || got[0].Status != "rank_gap" {
		t.Fatalf("rank gap must fail closed as incomplete authority: %+v", got)
	}
}
