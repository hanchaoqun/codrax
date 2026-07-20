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

// partsplitHarvestQuery — a 100ms bounded window: the 件③ floor is
// max(0.1, 1%×100) = 1.0ms.
func partsplitHarvestQuery() Query {
	return Query{PID: 100, TimeStart: 6.0, TimeEnd: 6.1}
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
	out := harvestGatedCompositeEdgeShareDisclosures(partsplitHarvestQuery(),
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
	out = harvestGatedCompositeEdgeShareDisclosures(partsplitHarvestQuery(),
		[]RootCauseRankItem{stamped}, []RootCauseRankItem{stamped})
	if len(out) != 1 || !out[0].SeatPublished {
		t.Fatalf("a published refused seat must harvest SeatPublished=true: %+v", out)
	}
	// Zero stamped rows → nil (absence silent).
	if out := harvestGatedCompositeEdgeShareDisclosures(partsplitHarvestQuery(),
		[]RootCauseRankItem{clean}, []RootCauseRankItem{clean}); out != nil {
		t.Fatalf("unstamped population must harvest nothing: %+v", out)
	}
}

// TestPartsplitHarvestFloorAndValueOrder — POOL2-1 件③ (§29.160③ user ruling
// 2026-07-20 「复用 SPANVIS 双分量地板…+行序改值降序;微真值降入 typed 记号
// (审计保留)」): a pre-edge share below max(0.1ms, 1%×窗) never issues a
// disclosure row (the typed stamp stays on the item — 宁降不删), issued rows
// order by pre-edge share DESC, and a windowless query keeps the dust
// component alone. Mutation arms: dropping the floor resurrects the micro
// row; asc ordering flips the head.
func TestPartsplitHarvestFloorAndValueOrder(t *testing.T) {
	mkStamped := func(pid int, pre, post, boundary float64) RootCauseRankItem {
		item := o3cRunnableSeat()
		item.Thread = ThreadRef{PID: pid, Comm: "w"}
		item.Type = "priority_inversion_runnable_wait"
		item.RunnableMs = pre + post
		item.GatedCompositeEdgePreShareMs = pre
		item.GatedCompositeEdgePostShareMs = post
		item.GatedCompositeEdgeAnchorTs = boundary
		item.GatedCompositeEdgeAnchorVia = HostWakeupEdgeAnchorViaDirect
		return item
	}
	// Window 100ms → floor 1.0ms. Pool order is deliberately value-ASC.
	micro := mkStamped(41, 0.049, 1.0, 6.001) // below floor → withheld
	mid := mkStamped(42, 1.2, 0.4, 6.002)
	big := mkStamped(43, 13.9, 0.1, 6.003)
	out := harvestGatedCompositeEdgeShareDisclosures(partsplitHarvestQuery(),
		[]RootCauseRankItem{micro, mid, big}, nil)
	if len(out) != 2 {
		t.Fatalf("the below-floor pre-share must not issue a row (typed stamp keeps the audit): %+v", out)
	}
	if out[0].Thread.PID != 43 || out[1].Thread.PID != 42 {
		t.Fatalf("rows must order by pre-edge share desc (行序改值降序): %+v", out)
	}
	// 宁降不删: the withheld row's typed stamp is untouched (audit survives).
	if micro.GatedCompositeEdgePreShareMs != 0.049 || micro.GatedCompositeEdgeAnchorTs != 6.001 {
		t.Fatalf("the typed stamp must survive the withheld row: %+v", micro)
	}
	// Micro window (2ms): the dust component rules — 0.05 stays out, 0.15
	// issues (both components exercised, SPANVIS constants verbatim).
	microQ := Query{PID: 100, TimeStart: 6.0, TimeEnd: 6.002}
	dust := mkStamped(44, 0.05, 0.2, 6.0005)
	real := mkStamped(45, 0.15, 0.2, 6.0006)
	out = harvestGatedCompositeEdgeShareDisclosures(microQ, []RootCauseRankItem{dust, real}, nil)
	if len(out) != 1 || out[0].Thread.PID != 45 {
		t.Fatalf("dust floor arm drifted: %+v", out)
	}
	// Windowless query → dust component alone (demote, never delete).
	out = harvestGatedCompositeEdgeShareDisclosures(Query{PID: 100},
		[]RootCauseRankItem{dust, real}, nil)
	if len(out) != 1 || out[0].Thread.PID != 45 {
		t.Fatalf("windowless harvest keeps the dust floor alone: %+v", out)
	}
}

// TestPartsplitTiebaTraceBoardFloorAcceptance — POOL2-1 件③ live acceptance
// (§29.160③ target, tieba trace board): the udk-irq micro shares (0.049 /
// 0.007 — window 144.557ms → floor 1.446ms) vanish from the disclosure
// channel while their typed stamps survive in the pool (audit), and the
// Binder:43397_19-23088 row (pre 13.982) leads the value-ordered channel.
func TestPartsplitTiebaTraceBoardFloorAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	const trace = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.450627, TimeEnd: 34579.595184,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	if len(rank.GatedCompositeEdgeShareDisclosures) != 1 {
		t.Fatalf("the micro udk-irq rows (0.049/0.007) must be withheld by the floor: %+v",
			rank.GatedCompositeEdgeShareDisclosures)
	}
	head := rank.GatedCompositeEdgeShareDisclosures[0]
	if head.Thread.PID != 23088 || math.Abs(head.PreMs-13.982) > 0.002 ||
		math.Abs(head.PostMs-0.020) > 0.002 || math.Abs(head.AccountMs-14.002) > 0.002 {
		t.Fatalf("the 23088 row must lead the value-ordered channel: %+v", head)
	}
	// 宁降不删: the withheld micro rows keep their typed stamps in the pool.
	stamped := map[int]float64{}
	for i := range rank.preTruncationItems {
		item := &rank.preTruncationItems[i]
		if item.GatedCompositeEdgePreShareMs > 0 && item.GatedCompositeEdgePreShareMs < 1.0 {
			stamped[item.Thread.PID] = item.GatedCompositeEdgePreShareMs
		}
	}
	if len(stamped) == 0 {
		t.Fatalf("the withheld micro rows must keep their typed stamps in the pool (audit)")
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
