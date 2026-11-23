package tracequery

import (
	"math"
	"strings"
	"testing"
)

// HMC377 round-11 merge (R-377-11 ① × ②). The two round-11 lanes each give
// every instant of a thread's ready time one carrier, but they meet on one
// claim: the carved anchored claim a CPU-restriction seat makes for its
// instants inside the chain object's windows (rank_subset_seat_carve.go) is
// also a claim of the one resolver's successor rule (rank_share_claims.go).
// When the publication gate withdraws the chain-object row that holds those
// instants, the naive merge (9f3622c52) returned them to the restriction
// seat twice — once as the successor's returned share (ShareReturnedMs, the
// ◇ account writer) and once by the carve writer, because the carved claim no
// longer resolved — so the seat published more than its own restricted
// instants (fix11/merge/explore/explore_naive_merge.log: 30.000 for a
// 25.000 ms account). With the thread's ◇ remainder ahead of it on the board,
// the remainder took the restriction seat's own instants instead, and the
// seat's sentence still said they were counted here. Its context form also
// said the holder was not on the published board while the gate had
// published it as context (R-377-8).
//
// Hand board in the review-10 shape: one walked window W (10 ms) held by a
// priority-inversion chain seat the gate withdraws, a ◇ remainder (40 ms
// outside W, anchored share 10 ms), and a restriction seat whose 25 ms
// straddle W (5 ms inside) and the rest (20 ms). Raw ready time 50 ms.

const hmc377MergeCarvePID = 4242

func hmc377MergeCarveWalked() []foldInterval { return []foldInterval{{start: 10.000, end: 10.010}} }

func hmc377MergeCarveBoard(remainderFirst bool) []RootCauseRankItem {
	thread := ThreadRef{Comm: "worker", PID: hmc377MergeCarvePID}
	chain := RootCauseRankItem{Rank: 1, Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: thread,
		DominantState: string(StateRunnable), RunnableMs: 10, GatedRunnableMs: 10, EffectiveImpactMs: 10, ImpactMs: 10, CumulativeImpactMs: 10,
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", LineStart: 10, LineEnd: 20}
	rspaHold(&chain, RootCauseShareHolderChainSeat, false, hmc377MergeCarveWalked(), 10)
	seat := RootCauseRankItem{Rank: 2, Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: thread,
		DominantState: string(StateRunnable), RunnableMs: 25, EffectiveImpactMs: 25, ImpactMs: 25, CumulativeImpactMs: 25,
		ProjectedImpactMs: 25, Summary: "restriction",
		cpuConstraintRestrictedIntervals: []foldInterval{{start: 10.000, end: 10.005}, {start: 10.040, end: 10.060}},
		ChainCredentialLaneDemoted:       true, Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent", LineStart: 30, LineEnd: 30}
	rest := []foldInterval{{start: 10.020, end: 10.060}}
	rem := RootCauseRankItem{Rank: 3, Type: "runnable_wait", Source: "window_stats", Thread: thread, DominantState: string(StateRunnable),
		RunnableMs: 40, EffectiveImpactMs: 40, ImpactMs: 40, CumulativeImpactMs: 40, ProjectedImpactMs: 40,
		ChainAnchorRemainderSeat: true, ChainAnchoredMs: 10, ChainAnchorFullMs: 50, Causality: "adjacent_to_wakeup_chain",
		ChainRelevance: "adjacent", LineStart: 40, LineEnd: 50}
	rspaSetSentence(&rem, rspaRemainderSentence(thread, "runnable (scheduling-pressure candidate)", 40, 10, 50,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377MergeCarvePID, false, 10, hmc377MergeCarveWalked())))
	rspaHold(&rem, RootCauseShareHolderRemainderSeat, false, rest, 40)
	if remainderFirst {
		return []RootCauseRankItem{chain, rem, seat}
	}
	return []RootCauseRankItem{chain, seat, rem}
}

func hmc377MergeCarveRow(t *testing.T, board []RootCauseRankItem, typ string) RootCauseRankItem {
	t.Helper()
	for _, it := range board {
		if it.Type == typ {
			return it
		}
	}
	t.Fatalf("no %s row on the board", typ)
	return RootCauseRankItem{}
}

func TestHMC377R11MergeCarvedShareOfAWithdrawnHolderHasOneCarrier(t *testing.T) {
	const tol = 0.002
	for _, tc := range []struct {
		name           string
		remainderFirst bool
	}{{"restriction_seat_first", false}, {"remainder_first", true}} {
		t.Run(tc.name, func(t *testing.T) {
			board := hmc377MergeCarveBoard(tc.remainderFirst)
			carveCPUConstraintReadyInstants(board)
			resolveRankShareClaims(board, nil)
			t.Run("engine_board", func(t *testing.T) {
				seat := hmc377MergeCarveRow(t, board, "cpu_affinity_or_cpuset")
				rem := hmc377MergeCarveRow(t, board, "runnable_wait")
				if math.Abs(seat.RunnableMs-20) > tol || math.Abs(rem.RunnableMs-20) > tol {
					t.Fatalf("engine board: restriction seat %.3f (want 20: its 5ms inside W counted by the chain seat), remainder %.3f (want 20)", seat.RunnableMs, rem.RunnableMs)
				}
			})
			published := append([]RootCauseRankItem(nil), board...)
			withdrawn := make([]bool, len(published))
			for i, it := range published {
				withdrawn[i] = it.Type == "priority_inversion_candidate"
			}
			hmc377R10Withdraw(published, nil, withdrawn, nil)
			seat := hmc377MergeCarveRow(t, published, "cpu_affinity_or_cpuset")
			rem := hmc377MergeCarveRow(t, published, "runnable_wait")
			t.Run("one_carrier", func(t *testing.T) {
				sum := 0.0
				for i, it := range published {
					if it.Thread.PID == hmc377MergeCarvePID && !withdrawn[i] {
						sum += it.RunnableMs
					}
				}
				if math.Abs(sum-50) > tol {
					t.Fatalf("published carriers of the thread sum to %.3f, raw ready time 50.000 (restriction seat %.3f, remainder %.3f)", sum, seat.RunnableMs, rem.RunnableMs)
				}
			})
			t.Run("cause_specific_seat_keeps_its_instants", func(t *testing.T) {
				for name, v := range map[string]float64{"runnable": seat.RunnableMs, "impact": seat.ImpactMs, "effective": seat.EffectiveImpactMs,
					"cumulative": seat.CumulativeImpactMs, "projected": seat.ProjectedImpactMs} {
					if math.Abs(v-25) > tol {
						t.Fatalf("restriction seat %s channel %.3f, want its own restricted account 25.000", name, v)
					}
				}
				if math.Abs(rem.RunnableMs-25) > tol || math.Abs(rem.EffectiveImpactMs-25) > tol {
					t.Fatalf("remainder %.3f / %.3f, want 25.000 (its 20 outside the restriction + the 5 of W outside it)", rem.RunnableMs, rem.EffectiveImpactMs)
				}
				if math.Abs(seat.ShareReturnedMs-5) > tol {
					t.Fatalf("restriction seat names the share it took back: ShareReturnedMs %.3f, want 5.000", seat.ShareReturnedMs)
				}
			})
			t.Run("sentence_names_the_context_holder", func(t *testing.T) {
				if strings.Contains(seat.Summary, "not on the published board") {
					t.Fatalf("the holder is published as context, never absent: %s", seat.Summary)
				}
				want := "5.000ms of it lies inside instants of this thread's chain seat, which " + rspaHolderContextOnly + ", so this row counts them with its own value"
				if !strings.Contains(seat.Summary, want) {
					t.Fatalf("restriction seat sentence must say it counts the 5ms itself:\n%s", seat.Summary)
				}
			})
			t.Run("idempotent", func(t *testing.T) {
				again := append([]RootCauseRankItem(nil), published...)
				resolveRankShareClaims(again, nil)
				for i := range again {
					if math.Abs(again[i].RunnableMs-published[i].RunnableMs) > tol || again[i].Summary != published[i].Summary {
						t.Fatalf("re-running the resolver moved row %d: %.3f -> %.3f", i, published[i].RunnableMs, again[i].RunnableMs)
					}
				}
			})
		})
	}
}
