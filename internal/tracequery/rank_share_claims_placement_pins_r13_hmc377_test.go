package tracequery

// HMC377 round 13, continuation — R-377-13 ⑦ pins on the claimless-detail
// placement's own internals (continuation mutants X09, X11):
//   - the owner-cycle guard: B4 ownership is acyclic, so no engine board
//     reaches it; it is what keeps the publication from spinning if a later
//     reconciliation change ever stamps a cycle, and it is pinned on the
//     placement itself — every row of a cycle is still placed exactly once, by
//     the withdrawn row's account;
//   - the B4 owner stamps are recomputed on every reconcile (the pass runs on
//     the build and again after the enrich additions): a row the second run
//     no longer pairs keeps no stamp of the first.

import (
	"testing"
	"time"
)

func TestHMC377R13ClaimlessOwnerCycleStillPlacesEveryRow(t *testing.T) {
	const family = "rank_pair:withdrawn"
	row := func(identity, owner string, l0 int) RootCauseRankItem {
		return RootCauseRankItem{Tier: RootCauseTierAbsorbed, Type: "scheduler_latency", Source: "scheduler_latency_stats", Thread: hmc377R11Dep,
			ImpactMs: 1, CumulativeImpactMs: 1, EffectiveImpactMs: 1, RunnableMs: 1, DominantState: "runnable",
			StartTs: 10.2, EndTs: 10.201, LineStart: l0, LineEnd: l0 + 1, AbsorbedByRankFamily: true, AbsorbedIntoFamily: family,
			b4Identity: identity, b4DirectOwner: owner}
	}
	type placed struct{ board, detail []RootCauseRankItem }
	done := make(chan placed, 1)
	go func() {
		board, detail := rspaPlaceClaimlessDetails([]RootCauseRankItem{hmc377R11OtherSeat()}, nil,
			[]RootCauseRankItem{row("owner:a", "owner:b", 100), row("owner:b", "owner:a", 110)},
			map[string]rspaWithdrawnAccount{family: {pid: hmc377R11Dep.PID}}, 1, TimeWindow{StartTs: 10, EndTs: 11},
			func(RootCauseRankItem) bool { return false })
		done <- placed{board, detail}
	}()
	select {
	case got := <-done:
		// No published row holds the withdrawn account: each row is published
		// once with its own value.
		if len(got.detail) != 0 || len(got.board) != 3 {
			t.Fatalf("board %d detail %d, want both rows of the cycle on the board once", len(got.board), len(got.detail))
		}
		for _, it := range got.board[1:] {
			if it.AbsorbedByRankFamily || it.AbsorbedIntoFamily != "" || !it.rspaReleasedDetail {
				t.Errorf("row %d-%d is not released: %+v", it.LineStart, it.LineEnd, it.AbsorbedIntoFamily)
			}
		}
		if got.board[1].LineStart == got.board[2].LineStart {
			t.Errorf("one row of the cycle is placed twice")
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("a cycle of claimless owners is never placed")
	}
}

func TestHMC377R13B4OwnerStampsAreRecomputedOnEveryReconcile(t *testing.T) {
	inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
	inv.inversionAccountRepresents = "inversion_account:rs"
	det := hmc377R13InvDetail(2.903, hmc377R11W(), []foldInterval{{start: 10.5, end: 10.501}}, "inversion_account:rs", 30)
	det.StatsWindowStartTs, det.StatsWindowEndTs = 10, 11
	rank := hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R13Rem(1.0, hmc377R11W(), 70), det, hmc377R13ChurnOf(det)})
	find := func(typ string) *RootCauseRankItem {
		for _, rows := range [][]RootCauseRankItem{rank.Items, rank.AbsorbedItems} {
			for i := range rows {
				if rows[i].Type == typ {
					return &rows[i]
				}
			}
		}
		t.Fatalf("no %s row", typ)
		return nil
	}
	d, c := find("priority_inversion_runnable_wait"), find("fragmented_runnable_wait")
	if d.b4Identity == "" || c.b4DirectOwner != d.b4Identity || !c.AbsorbedByRankFamily {
		t.Fatalf("precondition: the detail owns its churn twin: identity %q owner %q", d.b4Identity, c.b4DirectOwner)
	}
	// The churn account is measured again and no longer equals the detail's:
	// the next reconcile pairs it with no row.
	c.ImpactMs, c.CumulativeImpactMs, c.EffectiveImpactMs, c.ProjectedImpactMs, c.RunnableMs = 9, 9, 9, 9, 9
	c.StartTs, c.EndTs = 10.6, 10.609
	reconcileExactCrossTypeRankSeats(&rank)
	d, c = find("priority_inversion_runnable_wait"), find("fragmented_runnable_wait")
	if c.AbsorbedByRankFamily {
		t.Fatalf("precondition: the re-measured churn row is paired with no row")
	}
	if c.b4DirectOwner != "" || d.b4Identity != "" {
		t.Errorf("stamps of the earlier reconcile stay: churn owner %q, detail identity %q", c.b4DirectOwner, d.b4Identity)
	}
}
