package tracequery

import (
	"math"
	"testing"
)

// HMC377 round 13 (batch-owner ruling R-377-13 ①, 2026-11-22): one object per
// physical thread on the tree; the engine side of the two shapes whose lines
// the tree could not relate.

// The ◇ remainder clone of a host-edge semantic seat publishes its own
// remainder on every value channel. Before, it kept the priced seat's
// ImpactMs — the anchored share — so its context row repeated the anchored
// part beside the seat (pop w22m tbs_basemodule_-58968 at 532dc208d: native
// rank item 19 impact_ms 0.292 with projected/cumulative/actual 0.910 and the
// sentence "remainder 0.910ms"; raw: the span VerifyClass …ThreadPoolSnapshot
// walls 1.202ms in the window, 0.292ms inside the analysed thread's waits its
// thread's wakeup ended).
func TestHMC377R13SemanticRemainderClonePublishesItsOwnRemainder(t *testing.T) {
	seat := RootCauseRankItem{
		Type: "class_verification", SemanticClass: "class_verification", Thread: ThreadRef{PID: 58968},
		ImpactMs: 0.292, ProjectedImpactMs: 0.292, CumulativeImpactMs: 0.292, EffectiveImpactMs: 0.292,
		ChainAnchoredMs: 0.292, ChainAnchorFullMs: 1.202, StartTs: 10835.09466, EndTs: 10835.095862, Confidence: 0.7,
	}
	seat.hostEdgeRemainderStartTs, seat.hostEdgeRemainderEndTs = 10835.09466, 10835.095862
	seat.hostEdgeRemainderIntervals = []foldInterval{{start: 10835.09466, end: 10835.095335}, {start: 10835.095627, end: 10835.095862}}
	rem, ok := semanticEdgeAnchorRemainderSeat(seat)
	if !ok {
		t.Fatal("the remainder clone must mint")
	}
	for name, got := range map[string]float64{"impact": rem.ImpactMs, "projected": rem.ProjectedImpactMs, "cumulative": rem.CumulativeImpactMs, "actual": rem.ActualImpactMs} {
		if math.Abs(got-0.910) > 0.0005 {
			t.Errorf("the remainder clone's %s channel is %.3f, its own remainder is 0.910", name, got)
		}
	}
	if rem.EffectiveImpactMs != 0 || !rem.ChainAnchorRemainderSeat {
		t.Fatalf("the clone prices nothing and stays the remainder half: %+v", rem)
	}
}

// On the complete original 105 元宝 容器重启 #2 (CODRAX_HMC377_YUANBAO_CONTAINER2;
// review window w17 [3820.346447, 3821.062086) TID 50874): the case-B clipped
// seats of dtmp-51025 and dtmp-51035 (the runnable_wait arm) and of 50924 (the
// R-377-4 inversion arm) show exactly the ready segments their chain hops are
// priced at. Raw (review-population raw_all.json, own parse): 51025 ready
// 5.493ms, of which [3820.898136, 3820.903491] = 5.355ms inside the hop window
// [3820.898136, 3820.904173]; 51035 ready 8.859ms, 7.126ms inside
// [3820.92113, 3820.928676]; 50924 ready 38.546ms, 7.168ms inside
// [3820.580302, 3820.587981].
func TestHMC377R13WholeOriginalClippedSeatShowsItsHopsAccount(t *testing.T) {
	path := hmc377F3Original(t, "CODRAX_HMC377_YUANBAO_CONTAINER2", "22c9ba3708a9bbc0c1c31703138859c8b00f9f6bfe39df1e1cdd9e18802edd4a", 62845037)
	rank, chain := hmc377F3Board(t, path, 50874, 3820.346447, 3821.062086)
	stampStateAccountPublicationKeys(&chain, &rank)
	for pid, anchored := range map[int]float64{51025: 5.355, 51035: 7.126, 50924: 7.168} {
		var seat *RootCauseRankItem
		for i := range rank.Items {
			it := &rank.Items[i]
			if it.Thread.PID == pid && it.Type == "runnable_wait" && it.Source == "window_stats" && it.ChainAnchorFullMs > 0 && !it.ChainAnchorRemainderSeat {
				seat = it
			}
		}
		if seat == nil || math.Abs(seat.RunnableMs-anchored) > 0.0015 {
			t.Fatalf("pid %d: the case-B clipped seat holds %.3f: %+v", pid, anchored, seat)
		}
		var hop string
		for i := range chain.CausalImpacts {
			impact := chain.CausalImpacts[i]
			if impact.Thread.PID == pid && impact.DominantState == string(StateRunnable) && math.Abs(impact.DominantImpactMs-anchored) <= 0.0015 {
				hop = impact.StateAccountKey
			}
		}
		if hop == "" || seat.MeasuredStateAccountKey != hop {
			t.Fatalf("pid %d: the clipped seat shows its hop's account: seat %q hop %q", pid, seat.MeasuredStateAccountKey, hop)
		}
	}
}
