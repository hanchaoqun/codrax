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
//
// EVOLUTION RECORD (HMC377 round 10, batch-owner ruling R-377-10 ①④,
// 2026-11-21): WithdrawRankShareHolders no longer re-validates claims by its
// own rule (R9-1's partial copy of the resolver, which unresolved a whole
// multi-holder claim when one holder was withdrawn — review 9 U1). A withdrawn
// row now holds no share and makes no claim (its split and claim fields go
// with its seat) and the one resolver re-runs on the published copy; the
// decision for absorbed detail rows rides beside the board's. Pins moved:
//   - split pair: the withdrawn seat's own remainder claim was kept resolved
//     and the remainder kept its holder key; now the withdrawn seat makes no
//     claim (ShareClaims nil, split fields zero) and the remainder holds no
//     key (no claim names it). Raw evidence of the old behaviour:
//     fix9/review-holders/perturb/out/nec_w11098.perturb.txt (PB[2] context row
//     still CLAIM remainder/remainder_seat resolved holders=[seat-18]) and the
//     held split 「同源二分:全窗1.130ms=本行锚定0.820ms+其余0.310ms(◇余段席)」
//     on that row in nec_w11098.fence_zh.txt (register R9H-5);
//   - the claimant's span takes the context form (the holding row is
//     published only as context), no longer the compaction form "the owning
//     seat is not on the published board" (register R9H-4, same perturb.txt
//     PB[18]/PB[19] SUMMARY).
//
// EVOLUTION RECORD (HMC377 round 11, batch-owner ruling R-377-11 ①,
// 2026-11-21): old → new. Old (round 10): a ◇ remainder whose anchored
// holder the gate withdrew kept only its remainder value (20.000) and said the
// owning row is context only — the anchored 7.000 had no ◎ carrier (review 10
// R10H-3: NetEase injection 11098 Σ0.310 of raw 1.130, 6934 0.075 of 0.106;
// Douyin 11317 92.594 of 94.225, 10879 247.364 of 248.214 —
// ~/hmc-work/hmc377/fix10/review-holders/perturb/out/{ne_w11098,dy_w11317}.perturb.txt,
// raw/rawall_{ne11077,dy11105}.json). New: each segment the withdrawn seat
// held for a claim no other published seat covers passes to one published
// successor — the ◇ row's anchored claim comes first — so the remainder
// carries its whole account 27.000 (ShareReturnedMs 7.000) and its span says
// it counts that share with its own value.

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
			WithdrawRankShareHolders(published, nil, []bool{false, false}, nil)
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
		WithdrawRankShareHolders(published, nil, []bool{false, true}, nil)
		rem, seat := published[0], published[1]
		if c := rem.ShareClaims[0]; c.Resolved || c.Exact || len(c.HolderKeys) != 0 || !near(c.ReturnedMs, 7.0, 0.0005) {
			t.Errorf("the claim on a withdrawn holder must pass its segments back to the remainder: %+v", c)
		}
		if !strings.Contains(rem.Summary, rspaSummaryOwnedReturned) || strings.Contains(rem.Summary, rspaSummaryOwnedByChainSeat+" ") ||
			strings.Contains(rem.Summary, rspaSummaryOwnedByChainSeatUnpublished) {
			t.Errorf("the remainder's span must say the holding row is context only and this row counts the share: %q", rem.Summary)
		}
		if !near(rem.RunnableMs, 27.0, 0.0005) || !near(rem.ShareReturnedMs, 7.0, 0.0005) {
			t.Errorf("the remainder carries its whole account (one carrier): runnable=%.3f returned=%.3f", rem.RunnableMs, rem.ShareReturnedMs)
		}
		if seat.ShareHolderKey != "" {
			t.Errorf("the withdrawn seat keeps the holder key %q", seat.ShareHolderKey)
		}
		// The withdrawn seat makes no claim and keeps no split: no claim
		// names the remainder, which therefore holds no key.
		if len(seat.ShareClaims) != 0 || seat.ChainAnchoredMs != 0 || seat.ChainAnchorFullMs != 0 || rem.ShareHolderKey != "" {
			t.Errorf("the withdrawn seat still claims or splits: claims=%+v anchored=%.3f full=%.3f (remainder key %q)",
				seat.ShareClaims, seat.ChainAnchoredMs, seat.ChainAnchorFullMs, rem.ShareHolderKey)
		}
		if c := board[0].ShareClaims[0]; !c.Resolved || len(c.HolderKeys) != 1 || board[0].Summary != engineSummary || board[1].ShareHolderKey == "" {
			t.Errorf("the engine rows were mutated: %+v %q key=%q", c, board[0].Summary, board[1].ShareHolderKey)
		}
		again := append([]RootCauseRankItem(nil), published...)
		WithdrawRankShareHolders(again, nil, []bool{false, true}, nil)
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
		WithdrawRankShareHolders(published, nil, []bool{true, false}, nil)
		got := published[1]
		if c := got.ShareClaims[0]; c.Resolved || c.Exact || len(c.HolderKeys) != 0 || !c.Represented {
			t.Errorf("the represented claim on a withdrawn holder: %+v", c)
		}
		if got.ChainAnchorRepresentedByChainSeat {
			t.Errorf("the satellite still wears the represented marker")
		}
		if !strings.Contains(got.Summary, rspaRepresentedContext(7.0, 7.0)) || strings.Contains(got.Summary, "anchored share represented by the chain seat") {
			t.Errorf("the represented sentence must say the holding row is published only as context: %q", got.Summary)
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
		// EVOLUTION RECORD (HMC377 round 12, R-377-12 ①): the publication
		// returns its rows — a detail row whose claim names the withdrawn
		// holder (and that no published successor takes) owns those instants
		// and returns to the board; it is read from the returned slices.
		//   old: the detail row is read in place from pd[0];
		//   new: the row is the released board row after the withdrawn seat.
		pb, pd = hmc377R12Withdraw(pb, pd, []bool{true}, nil)
		if len(pb) != 2 || len(pd) != 0 {
			t.Fatalf("the detail row owning the withdrawn holder's instants returns to the board: board %d detail %d", len(pb), len(pd))
		}
		if c := pb[1].ShareClaims[0]; c.Resolved || pb[1].ChainAnchorRepresentedByChainSeat || pb[0].ShareHolderKey != "" || pb[1].AbsorbedByRankFamily {
			t.Errorf("the detail row's claim on a withdrawn holder: %+v marker=%t key=%q absorbed=%t", c, pb[1].ChainAnchorRepresentedByChainSeat, pb[0].ShareHolderKey, pb[1].AbsorbedByRankFamily)
		}
	})
}
