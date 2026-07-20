package tracequery

// rank_state_edge_anchor_partsplit_test.go — PARTSPLIT-1 (§29.150④ user
// ruling, 2026-07-19) engine pins: the R4-mirror refusal record (disclosure-
// only bisection measures on the refused gated composite seat) + the
// result-level NON-SEAT side channel. The refusal itself (value/lane/ordinal
// zero-motion) stays pinned in TestBareCensusEdgeStateSeatInversionR4Mirror
// arm ② (evolved 拒转+披露 form) and the live TestRSPATiebaWitnessBoard
// 23088 pin.

import (
	"context"
	"math"
	"os"
	"reflect"
	"testing"
)

// TestPartsplitRefusalRecordAbsentOnNonRefusalForms — the four disclosure
// fields mint ONLY at the refusal site: a fully-pre-edge inversion seat
// (converts whole) and a fully-post-edge one (negative sentinel) both stay
// record-free (admission = the atomic stamp; absence silent).
func TestPartsplitRefusalRecordAbsentOnNonRefusalForms(t *testing.T) {
	chain := r3SyntheticChain()
	base := o3cRunnableSeat()
	base.Type = "priority_inversion_runnable_wait"
	base.GatedRunnableMs = 0.8
	base.EffectiveImpactMs = 0.8
	// Fully pre-edge → whole-seat conversion, no refusal record.
	full := base
	full.runnableIntervals = []foldInterval{{start: 6.001, end: 6.003}, {start: 6.004, end: 6.005}}
	full.RunnableMs, full.CumulativeImpactMs = 3.0, 3.0
	items := anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{full})
	if items[0].GatedCompositeEdgePreShareMs != 0 || items[0].GatedCompositeEdgeAnchorTs != 0 ||
		items[0].GatedCompositeEdgeAnchorVia != "" {
		t.Fatalf("fully pre-edge conversion must not wear the refusal record: %+v", items[0])
	}
	// Fully post-edge → 边后=解除 grants nothing AND records nothing.
	post := base
	post.runnableIntervals = []foldInterval{{start: 6.007, end: 6.009}}
	post.RunnableMs, post.CumulativeImpactMs = 2.0, 2.0
	before := post
	items = anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{post})
	if len(items) != 1 || !reflect.DeepEqual(items[0], before) {
		t.Fatalf("fully post-edge inversion seat must stay byte-identical (no record): %+v", items)
	}
	// Σ-broken inventory → fail closed, no record (缺测度不发).
	broken := base
	broken.RunnableMs = 9.0
	before = broken
	items = anchorBareCensusEdgeStateSeats(chain, []RootCauseRankItem{broken})
	if len(items) != 1 || !reflect.DeepEqual(items[0], before) {
		t.Fatalf("Σ-broken inventory must stay byte-identical (no record): %+v", items)
	}
}

// TestPartsplitHarvestSideChannel — the harvest builds one deduped record per
// stamped (pid, boundary) from pool ∪ published, with the publishedness bit
// reflecting the FINAL board; unstamped rows harvest nothing.
func TestPartsplitHarvestSideChannel(t *testing.T) {
	stamped := o3cRunnableSeat()
	stamped.Type = "priority_inversion_runnable_wait"
	stamped.GatedCompositeEdgePreShareMs = 3.5
	stamped.GatedCompositeEdgePostShareMs = 1.5
	stamped.GatedCompositeEdgeAnchorTs = 6.006
	stamped.GatedCompositeEdgeAnchorVia = HostWakeupEdgeAnchorViaDirect
	clean := o3cRunnableSeat()
	// Pool holds the stamped row twice (build+enrich union) plus a clean row;
	// the published board holds neither → SeatPublished=false.
	out := harvestGatedCompositeEdgeShareDisclosures(
		[]RootCauseRankItem{stamped, stamped, clean}, nil)
	if len(out) != 1 {
		t.Fatalf("harvest must dedupe by (pid, boundary) and skip unstamped rows: %+v", out)
	}
	d := out[0]
	if d.Thread.PID != 300 || math.Abs(d.PreMs-3.5) > 0.0005 || math.Abs(d.PostMs-1.5) > 0.0005 ||
		math.Abs(d.AccountMs-5.0) > 0.0005 || d.BoundaryTs != 6.006 ||
		d.Via != HostWakeupEdgeAnchorViaDirect || d.SeatPublished ||
		d.LineStart != 10 || d.LineEnd != 20 {
		t.Fatalf("harvest record drifted: %+v", d)
	}
	// The published board carrying the stamped seat flips the honesty bit.
	out = harvestGatedCompositeEdgeShareDisclosures(
		[]RootCauseRankItem{stamped}, []RootCauseRankItem{stamped})
	if len(out) != 1 || !out[0].SeatPublished {
		t.Fatalf("a published refused seat must harvest SeatPublished=true: %+v", out)
	}
	// Zero stamped rows → nil (absence silent).
	if out := harvestGatedCompositeEdgeShareDisclosures([]RootCauseRankItem{clean}, []RootCauseRankItem{clean}); out != nil {
		t.Fatalf("unstamped population must harvest nothing: %+v", out)
	}
}

// TestPartsplitDonghu2955WitnessDisclosure — live witness #2 (§29.150④
// target): the donghu 2955 board's OS_FFRT_2_0-2614 inversion seat (19.563
// census runnable, inversion-recast) refuses conversion with pre=9.618 /
// post=9.945 (X+Y==19.563 to the µs, boundary 13763.006174, via=direct) and
// discloses on the side channel as a cap-dead pool seat.
func TestPartsplitDonghu2955WitnessDisclosure(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var ffrt *GatedCompositeEdgeShareDisclosure
	for i := range rank.GatedCompositeEdgeShareDisclosures {
		if rank.GatedCompositeEdgeShareDisclosures[i].Thread.PID == 2614 {
			ffrt = &rank.GatedCompositeEdgeShareDisclosures[i]
		}
	}
	if ffrt == nil {
		t.Fatalf("donghu 2614 refusal must disclose on the side channel: %+v", rank.GatedCompositeEdgeShareDisclosures)
	}
	if math.Abs(ffrt.PreMs-9.618) > 0.002 || math.Abs(ffrt.PostMs-9.945) > 0.002 ||
		math.Abs(ffrt.AccountMs-19.563) > 0.002 ||
		math.Abs(ffrt.BoundaryTs-13763.006174) > 0.000001 ||
		ffrt.Via != HostWakeupEdgeAnchorViaDirect || ffrt.SeatPublished {
		t.Fatalf("donghu 2614 witness disclosure drifted: %+v", ffrt)
	}
	if math.Abs(ffrt.PreMs+ffrt.PostMs-ffrt.AccountMs) > 0.0000005 {
		t.Fatalf("donghu 2614 X+Y==account identity broken: %.6f + %.6f != %.6f",
			ffrt.PreMs, ffrt.PostMs, ffrt.AccountMs)
	}
	// The refused seat itself stays whole in the pool: background lane, no
	// state basis, full census value (value/lane/ordinal zero-motion).
	var seat *RootCauseRankItem
	for i := range rank.preTruncationItems {
		item := &rank.preTruncationItems[i]
		if item.Thread.PID == 2614 && item.Type == "priority_inversion_runnable_wait" {
			seat = item
			break
		}
	}
	if seat == nil || seat.OnChainBasis != "" || seat.ChainRelevance != "background" ||
		math.Abs(seat.RunnableMs-19.563) > 0.002 || seat.ChainAnchorRemainderSeat {
		t.Fatalf("donghu 2614 refused seat must stay whole on its home lane: %+v", seat)
	}
}
