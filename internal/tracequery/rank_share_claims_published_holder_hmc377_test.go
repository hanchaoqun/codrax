package tracequery

// HMC377 fix round 9 (R9-1, register R8H-1) — the engine resolves every share
// claim against its own final board, but a claim is valid only against the
// record actually published: the tool's publication gate can still withdraw a
// holder row (context only). WithdrawRankShareHolders re-validates the claims
// against that gate's decision for every seat — the one typed signal, never a
// value, name or text match. Pinned here on engine-minted sentences: the
// claimant's spans take their compaction-disclosure form, a represented
// satellite stops wearing the marker, a withdrawn seat keeps no holder key,
// claims naming only published holders keep their bytes, a board without a
// withdrawn holder is unchanged, and the input rows are never mutated.

import (
	"reflect"
	"strings"
	"testing"
)

func hmc377R9PairBoard() []RootCauseRankItem {
	walked := []foldInterval{{start: 1.000, end: 1.007}}
	rest := []foldInterval{{start: 1.010, end: 1.030}}
	th := ThreadRef{Comm: "w", PID: 200}
	remainder := RootCauseRankItem{Type: "runnable_wait", Thread: th, ChainRelevance: "adjacent", DominantState: string(StateRunnable),
		RunnableMs: 20.0, ChainAnchoredMs: 7.0, ChainAnchorFullMs: 27.0, ChainAnchorRemainderSeat: true}
	rspaSetSentence(&remainder, rspaRemainderSentence(th, "runnable (scheduling-pressure candidate)", 20.0, 7.0, 27.0,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderClippedSeat, th.PID, false, 7.0, walked)))
	rspaHold(&remainder, RootCauseShareHolderRemainderSeat, false, rest, 20.0)
	clipped := RootCauseRankItem{Type: "runnable_wait", Thread: th, ChainRelevance: "on_chain", Causality: "on_wakeup_chain", DominantState: string(StateRunnable),
		RunnableMs: 7.0, ChainAnchoredMs: 7.0, ChainAnchorFullMs: 27.0}
	rspaSetSentence(&clipped, rspaAnchoredSentence(th, "runnable", 7.0, 27.0,
		rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, th.PID, false, 20.0, rest)))
	rspaHold(&clipped, RootCauseShareHolderClippedSeat, false, walked, 7.0)
	board := []RootCauseRankItem{remainder, clipped}
	resolveRankShareClaims(board, nil)
	return board
}

func hmc377R9RepresentedBoard() []RootCauseRankItem {
	walked := []foldInterval{{start: 2.000, end: 2.007}}
	th := ThreadRef{Comm: "dep", PID: 300}
	seat := RootCauseRankItem{Rank: 1, Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: th, ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
		DominantState: string(StateRunnable), RunnableMs: 7.0}
	rspaHold(&seat, RootCauseShareHolderChainSeat, false, walked, 7.0)
	sat := RootCauseRankItem{Rank: 2, Type: "runnable_wait", Source: "window_stats", Thread: th, ChainRelevance: "adjacent", DominantState: string(StateRunnable),
		RunnableMs: 7.0, ChainAnchorRepresentedByChainSeat: true, Summary: "adjacent runnable satellite"}
	rspaAppendSentence(&sat, rspaRepresentedDemotionSummary(7.0, 7.0,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, th.PID, false, 7.0, walked)))
	board := []RootCauseRankItem{seat, sat}
	resolveRankShareClaims(board, nil)
	return board
}

func TestHMC377R9WithdrawnHolderUnresolvesTheEngineClaim(t *testing.T) {
	t.Run("no withdrawn holder: unchanged", func(t *testing.T) {
		for _, board := range [][]RootCauseRankItem{hmc377R9PairBoard(), hmc377R9RepresentedBoard()} {
			published := append([]RootCauseRankItem(nil), board...)
			WithdrawRankShareHolders(published, nil, []bool{false, false})
			if !reflect.DeepEqual(published, board) {
				t.Fatalf("a board without a withdrawn holder changed:\n%+v\n%+v", published, board)
			}
		}
	})
	t.Run("split pair: the withdrawn clipped seat is named by no claim", func(t *testing.T) {
		board := hmc377R9PairBoard()
		if c := board[0].ShareClaims[0]; !c.Resolved || !strings.Contains(board[0].Summary, rspaSummaryOwnedByChainSeat) {
			t.Fatalf("precondition: the engine resolved the remainder's claim: %+v %q", c, board[0].Summary)
		}
		engineSummary := board[0].Summary
		published := append([]RootCauseRankItem(nil), board...)
		WithdrawRankShareHolders(published, nil, []bool{false, true})
		rem, seat := published[0], published[1]
		if c := rem.ShareClaims[0]; c.Resolved || c.Exact || len(c.HolderKeys) != 0 {
			t.Errorf("the claim on a withdrawn holder stays resolved: %+v", c)
		}
		if !strings.Contains(rem.Summary, rspaSummaryOwnedByChainSeatUnpublished) || strings.Contains(rem.Summary, rspaSummaryOwnedByChainSeat+" ") {
			t.Errorf("the remainder's span keeps its held form: %q", rem.Summary)
		}
		if seat.ShareHolderKey != "" {
			t.Errorf("the withdrawn seat keeps the holder key %q", seat.ShareHolderKey)
		}
		// The withdrawn seat's own claim names a published holder (the
		// remainder): it keeps its resolution and the remainder its key.
		if c := seat.ShareClaims[0]; !c.Resolved || len(c.HolderKeys) != 1 || c.HolderKeys[0] != rem.ShareHolderKey || rem.ShareHolderKey == "" {
			t.Errorf("a claim on a published holder changed: %+v (remainder key %q)", c, rem.ShareHolderKey)
		}
		if c := board[0].ShareClaims[0]; !c.Resolved || len(c.HolderKeys) != 1 || board[0].Summary != engineSummary || board[1].ShareHolderKey == "" {
			t.Errorf("the engine rows were mutated: %+v %q key=%q", c, board[0].Summary, board[1].ShareHolderKey)
		}
		again := append([]RootCauseRankItem(nil), published...)
		WithdrawRankShareHolders(again, nil, []bool{false, true})
		if !reflect.DeepEqual(again, published) {
			t.Errorf("re-validation is not idempotent")
		}
	})
	t.Run("represented satellite: keeps its own value", func(t *testing.T) {
		board := hmc377R9RepresentedBoard()
		sat := board[1]
		if c := sat.ShareClaims[0]; !c.Resolved || !c.Represented || !sat.ChainAnchorRepresentedByChainSeat {
			t.Fatalf("precondition: the engine resolved the represented claim: %+v marker=%t", c, sat.ChainAnchorRepresentedByChainSeat)
		}
		published := append([]RootCauseRankItem(nil), board...)
		WithdrawRankShareHolders(published, nil, []bool{true, false})
		got := published[1]
		if c := got.ShareClaims[0]; c.Resolved || c.Exact || len(c.HolderKeys) != 0 || !c.Represented {
			t.Errorf("the represented claim on a withdrawn holder: %+v", c)
		}
		if got.ChainAnchorRepresentedByChainSeat {
			t.Errorf("the satellite still wears the represented marker")
		}
		if !strings.Contains(got.Summary, rspaRepresentedCompact(7.0, 7.0)) || strings.Contains(got.Summary, "anchored share represented by the chain seat") {
			t.Errorf("the represented sentence keeps its held form: %q", got.Summary)
		}
		if published[0].ShareHolderKey != "" || !board[1].ChainAnchorRepresentedByChainSeat || !board[1].ShareClaims[0].Resolved {
			t.Errorf("holder key %q / the engine rows were mutated", published[0].ShareHolderKey)
		}
	})
	t.Run("detail rows re-validate against the same board", func(t *testing.T) {
		board := hmc377R9RepresentedBoard()
		detail := []RootCauseRankItem{board[1]}
		board = board[:1]
		resolveRankShareClaims(board, detail)
		if !detail[0].ShareClaims[0].Resolved {
			t.Fatalf("precondition: the detail row's claim resolves")
		}
		pd := append([]RootCauseRankItem(nil), detail...)
		pb := append([]RootCauseRankItem(nil), board...)
		WithdrawRankShareHolders(pb, pd, []bool{true})
		if c := pd[0].ShareClaims[0]; c.Resolved || pd[0].ChainAnchorRepresentedByChainSeat || pb[0].ShareHolderKey != "" {
			t.Errorf("the detail row's claim on a withdrawn holder: %+v marker=%t key=%q", c, pd[0].ChainAnchorRepresentedByChainSeat, pb[0].ShareHolderKey)
		}
	})
}
