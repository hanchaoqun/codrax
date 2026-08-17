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

func TestBuildTraceRankRosterAuthoritiesSeparatesOnChainAndAdjacentBoards(t *testing.T) {
	onOne := traceRankRosterTestNode("on-1", "target", "params", 1, 2, 1, "running", "target", 8)
	onTwo := traceRankRosterTestNode("on-2", "target", "params", 1, 2, 2, "io_wait", "worker", 4)
	adjOne := traceRankRosterTestNode("adj-1", "target", "params", 1, 2, 1, "runnable_wait", "neighbor", 6)
	adjOne.ChainRelevance = "adjacent"
	adjTwo := traceRankRosterTestNode("adj-2", "target", "params", 1, 2, 2, "io_wait", "other", 2)
	adjTwo.ChainRelevance = "adjacent"

	got := BuildTraceRankRosterAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{{
		ArtifactLabel: "trace.systrace",
		RankedSeats:   []TraceCausalProjectionNode{adjTwo, onTwo, adjOne, onOne},
	}}})
	if len(got) != 2 {
		t.Fatalf("expected independent on-chain and adjacent boards, got %+v", got)
	}
	byChannel := map[string]TraceRankRosterAuthority{}
	for _, board := range got {
		byChannel[board.BoardChannel] = board
	}
	for _, channel := range []string{"on_chain", "adjacent"} {
		board, ok := byChannel[channel]
		if !ok || !board.Complete || board.Status != "complete" || len(board.Seats) != 2 ||
			board.Seats[0].Rank != 1 || board.Seats[1].Rank != 2 {
			t.Fatalf("channel %s board drifted: %+v", channel, board)
		}
	}
}

func TestBuildTraceRankRosterAuthoritiesExcludesKnownExpandedWindowFromPrincipalRoster(t *testing.T) {
	exact := traceRankRosterTestNode("exact", "target", "params", 1, 1.010, 1, "runnable", "worker", 8.3)
	expanded := traceRankRosterTestNode("expanded", "target", "params", 1, 1.011, 2, "runnable_wait", "target", 0.020)
	legacy := traceRankRosterTestNode("legacy", "target", "legacy", 0, 0, 1, "io_wait", "legacy-worker", 2)
	projection := TraceCausalProjection{
		ArtifactLabel: "trace.systrace",
		WindowStartTs: 1,
		WindowEndTs:   1.010,
		RankedSeats:   []TraceCausalProjectionNode{expanded, exact, legacy},
		TargetStateAccount: &TraceCausalProjectionTargetStateAccount{
			Subject: "target", WindowStartTs: 1, WindowEndTs: 1.010, TotalMS: 10,
		},
	}

	got := BuildTraceRankRosterAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{projection}})
	if len(got) != 2 {
		t.Fatalf("exact and legacy boards should remain while expanded board is contextual only: %+v", got)
	}
	for _, board := range got {
		for _, seat := range board.Seats {
			if seat.EvidenceID == "expanded" || seat.Subject == "target" {
				t.Fatalf("known expanded-window seat leaked into principal rank authority: %+v", got)
			}
		}
	}
	if TraceCausalProjectionNodeMatchesPrincipalWindow(expanded, 1, 1.010) {
		t.Fatal("one-millisecond expansion must not pass the principal-value window ruler")
	}
	if !TraceCausalProjectionNodeMatchesPrincipalWindow(legacy, 1, 1.010) {
		t.Fatal("missing legacy window identity must not manufacture a rejection")
	}
}
