package tracequery

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 13 (R-377-13 ①): the helper the case-B arms call
// (rspaShowClippedReadyAccount) — white-box; the whole-original test in
// rank_one_object_r13_hmc377_test.go pins the arms that call it.

// A case-B clipped ready-time seat shows its inside instants: the measured
// identity is minted from them (and only from them — a seat whose published
// ready time the inventory does not reproduce gets none).
func TestHMC377R13ClippedSeatMeasuredInventoryIsItsInsideShare(t *testing.T) {
	inside := []foldInterval{{start: 4050.611375, end: 4050.616067}}
	item := RootCauseRankItem{
		Thread: ThreadRef{PID: 59777}, Type: "runnable_wait", Source: "window_stats", DominantState: string(StateRunnable),
		RunnableMs: 4.692, EffectiveImpactMs: 4.692, ImpactMs: 4.692, StatsWindowStartTs: 4050.096232, StatsWindowEndTs: 4050.749322,
		runnableIntervals: []foldInterval{{start: 4050.611283, end: 4050.616067}, {start: 4050.680000, end: 4050.680290}},
	}
	rspaShowClippedReadyAccount(&item, inside)
	rank := RootCauseRankResult{Items: []RootCauseRankItem{item}, Window: TimeWindow{StartTs: 4050.096232, EndTs: 4050.749322}}
	stampStateAccountPublicationKeys(nil, &rank)
	want := types.TraceSchedulerStateAccountIdentity(59777, "runnable", 4050.096232, 4050.749322, [][2]float64{{4050.611375, 4050.616067}}, 4.692)
	if want == "" || rank.Items[0].MeasuredStateAccountKey != want {
		t.Fatalf("the clipped seat shows its inside share: got %q want %q", rank.Items[0].MeasuredStateAccountKey, want)
	}
	if rank.Items[0].StateAccountKey != "" {
		t.Fatalf("the seat's own census inventory does not reproduce its price, so no priced identity: %q", rank.Items[0].StateAccountKey)
	}
	stale := item
	stale.RunnableMs = 4.0
	rank = RootCauseRankResult{Items: []RootCauseRankItem{stale}, Window: rank.Window}
	stampStateAccountPublicationKeys(nil, &rank)
	if rank.Items[0].MeasuredStateAccountKey != "" {
		t.Fatalf("an inventory that does not reproduce the published ready time mints nothing: %q", rank.Items[0].MeasuredStateAccountKey)
	}
}
