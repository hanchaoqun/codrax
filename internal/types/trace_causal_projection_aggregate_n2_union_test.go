package types

import (
	"fmt"
	"testing"
)

// §11-N2 mutation pins (real_trace_campaign_20260705.md, q2 specimen E10):
// the R2 ×N merge publishes the interval-UNION caliber — not the SUM — when
// members from DISTINCT query windows overlap in time, and keeps the SUM
// byte-identically for every disjoint / same-window / window-less shape.

// n2Record builds one per-occurrence causal-hop observation with an
// occurrence ts interval and (optionally) a typed selected_window identity —
// the exact q2 shape (two wakeup_chain queries over overlapping windows each
// publishing per-occurrence wakeup_causal rows for the same subject).
func n2Record(id, subject, object string, impact float64, startTs, endTs float64, lineStart, lineEnd int, window string) ObservationRecord {
	notes := []string{
		fmt.Sprintf("impact_ms=%.3f", impact),
		fmt.Sprintf("cumulative_impact_ms=%.3f", impact),
		"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
	}
	if window != "" {
		notes = append(notes, "selected_window="+window)
	}
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "wakeup_causal_impact",
		ClaimKey:        "wakeup_causal_impact:" + id,
		Subject:         subject,
		Object:          object,
		Value:           fmt.Sprintf("%.3f", impact),
		Unit:            "ms",
		Confidence:      0.8,
		SupportRefs:     []string{"obs:" + id},
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd, StartTs: startTs, EndTs: endTs},
		RichNotes:       notes,
	}
}

func n2FindMerged(t *testing.T, nodes []TraceCausalProjectionNode, count int) *TraceCausalProjectionNode {
	t.Helper()
	for i := range nodes {
		if nodes[i].MergedCount == count {
			return &nodes[i]
		}
	}
	t.Fatalf("×%d aggregate row missing: %+v", count, nodes)
	return nil
}

func n2Close(a, b float64) bool {
	diff := a - b
	return diff > -0.0005 && diff < 0.0005
}

// TestTraceCausalProjectionCrossWindowUnionQ2E10Shape pins the q2-E10
// specimen arithmetic: four per-occurrence rows from TWO query windows
// (2.25s window 3680.569–3682.819 and 201ms window 3680.800–3681.001), where
// the small window's 15.206ms occurrence [3680.7995–3680.8192] lies entirely
// inside the big window's 104.127ms occurrence [3680.6909–3680.8192]. The
// legacy SUM published 183.940ms; the union caliber must publish
// 168.734ms (= SUM − 15.206 double-counted re-measurement) — union ≠ SUM is
// the mutation pin (reverting R2 to unconditional SUM must red).
func TestTraceCausalProjectionCrossWindowUnionQ2E10Shape(t *testing.T) {
	const (
		bigWindow   = "3680.569..3682.819"
		smallWindow = "3680.800..3681.001"
	)
	records := []ObservationRecord{
		n2Record("E1", "binder:8815_1-6581", "runnable", 104.127, 3680.6909, 3680.8192, 117231, 140719, bigWindow),
		n2Record("E2", "binder:8815_1-6581", "runnable", 50.057, 3680.9000, 3680.9800, 141000, 152000, bigWindow),
		n2Record("E3", "binder:8815_1-6581", "runnable", 15.206, 3680.7995, 3680.8192, 136600, 140719, smallWindow),
		n2Record("E4", "binder:8815_1-6581", "runnable", 14.550, 3681.0000, 3681.0600, 153000, 158000, smallWindow),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	agg := n2FindMerged(t, got.OnChainCauses, 4)

	sum := 104.127 + 50.057 + 15.206 + 14.550 // 183.940 — the legacy double count
	union := sum - 15.206                     // 168.734 — contained re-measurement contributes 0
	if n2Close(agg.ImpactMS, sum) {
		t.Fatalf("union caliber must NOT publish the cross-window SUM %0.3f: %+v", sum, agg)
	}
	if !n2Close(agg.ImpactMS, union) || !n2Close(agg.CumulativeImpactMS, union) {
		t.Fatalf("union caliber value want %0.3f: %+v", union, agg)
	}
	if !agg.MergedIntervalUnion {
		t.Fatalf("union row must carry the typed caliber marker: %+v", agg)
	}
	if !n2Close(agg.MergedSumMS, sum) {
		t.Fatalf("union row must retain the lossless raw Σ %0.3f in MergedSumMS: %+v", sum, agg)
	}
	// The per-instance range stays lossless and the union value never drops
	// below the largest single member (greedy head contributes in full).
	if agg.MergedMinMS != 14.550 || agg.MergedMaxMS != 104.127 {
		t.Fatalf("per-instance min–max range lost: %+v", agg)
	}
	if agg.ImpactMS < agg.MergedMaxMS {
		t.Fatalf("union value must never drop below the largest member: %+v", agg)
	}
	// 窗身份 (§11-N2, 联动 q1-B6): the merge detail lists BOTH member query
	// windows, ascending; the merged row itself claims no single window.
	if len(agg.MergedQueryWindows) != 2 {
		t.Fatalf("×N window roster must list both member query windows: %+v", agg.MergedQueryWindows)
	}
	if agg.MergedQueryWindows[0].StartTs != 3680.569 || agg.MergedQueryWindows[0].EndTs != 3682.819 ||
		agg.MergedQueryWindows[1].StartTs != 3680.800 || agg.MergedQueryWindows[1].EndTs != 3681.001 {
		t.Fatalf("×N window roster wrong/unordered: %+v", agg.MergedQueryWindows)
	}
	if agg.QueryWindowStartTs != 0 || agg.QueryWindowEndTs != 0 {
		t.Fatalf("a multi-window merged row must not claim a single row-level query window: %+v", agg)
	}
}

// TestTraceCausalProjectionCrossWindowDisjointKeepsSum pins the untouched
// lane: members from two DIFFERENT query windows whose occurrence intervals
// are fully disjoint keep the SUM semantics bit-for-bit (the q1-B6 双窗独立
// adjudication is about not fabricating merges/dedup across windows — the
// union caliber must not engage without physical time overlap). The window
// roster is still disclosed (窗身份注 carrier).
func TestTraceCausalProjectionCrossWindowDisjointKeepsSum(t *testing.T) {
	records := []ObservationRecord{
		n2Record("E1", "worker-7", "runnable", 20.000, 100.100, 100.140, 1000, 1200, "100.000..101.000"),
		n2Record("E2", "worker-7", "runnable", 10.000, 100.200, 100.230, 1300, 1400, "100.000..101.000"),
		n2Record("E3", "worker-7", "runnable", 5.000, 101.500, 101.520, 2000, 2100, "101.400..101.800"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	agg := n2FindMerged(t, got.OnChainCauses, 3)

	if !n2Close(agg.ImpactMS, 35.0) || !n2Close(agg.CumulativeImpactMS, 35.0) {
		t.Fatalf("disjoint cross-window members must keep the SUM 35.000: %+v", agg)
	}
	if agg.MergedIntervalUnion || agg.MergedSumMS != 0 {
		t.Fatalf("disjoint members must not wear the union caliber: %+v", agg)
	}
	if len(agg.MergedQueryWindows) != 2 {
		t.Fatalf("disjoint cross-window ×N row still discloses its window sources: %+v", agg.MergedQueryWindows)
	}
}

// TestTraceCausalProjectionSameWindowOverlapKeepsSum pins the PRECISE gate:
// bare time overlap WITHOUT distinct query-window identity never engages the
// union lane. Same-window overlapping same-(subject,object) rows are DISTINCT
// facts (the E9/E10 9µs-apart strict pin family: within one query the engine
// carves disjoint occurrences; wide enclosing spans may still overlap), and
// the PTV6 review explicitly rejected envelope-overlap-only folding as a
// noisy signal. Two shapes: all members in ONE window, and members with NO
// window identity at all — both keep the SUM.
func TestTraceCausalProjectionSameWindowOverlapKeepsSum(t *testing.T) {
	oneWindow := []ObservationRecord{
		n2Record("E1", "worker-8", "io_latency", 40.000, 200.000, 200.900, 1000, 9000, "199.000..201.000"),
		n2Record("E2", "worker-8", "io_latency", 12.000, 200.100, 200.800, 1100, 8000, "199.000..201.000"),
		n2Record("E3", "worker-8", "io_latency", 7.000, 200.200, 200.700, 1200, 7000, "199.000..201.000"),
	}
	got := TraceCausalProjectionFromObservationRecords(oneWindow)
	agg := n2FindMerged(t, got.OnChainCauses, 3)
	if !n2Close(agg.ImpactMS, 59.0) || agg.MergedIntervalUnion {
		t.Fatalf("same-window overlapping members must keep the SUM 59.000 (distinct facts): %+v", agg)
	}
	// All members share ONE window → the merged row may keep it, and the
	// roster names exactly that window.
	if len(agg.MergedQueryWindows) != 1 ||
		agg.QueryWindowStartTs != 199.0 || agg.QueryWindowEndTs != 201.0 {
		t.Fatalf("single-window ×N row keeps its window identity: %+v", agg)
	}

	windowless := []ObservationRecord{
		n2Record("E1", "worker-9", "io_latency", 40.000, 300.000, 300.900, 1000, 9000, ""),
		n2Record("E2", "worker-9", "io_latency", 12.000, 300.100, 300.800, 1100, 8000, ""),
		n2Record("E3", "worker-9", "io_latency", 7.000, 300.200, 300.700, 1200, 7000, ""),
	}
	got = TraceCausalProjectionFromObservationRecords(windowless)
	agg = n2FindMerged(t, got.OnChainCauses, 3)
	if !n2Close(agg.ImpactMS, 59.0) || agg.MergedIntervalUnion {
		t.Fatalf("window-less overlapping members must keep the SUM (no cross-window evidence): %+v", agg)
	}
	if len(agg.MergedQueryWindows) != 0 {
		t.Fatalf("window-less members must not grow a window roster: %+v", agg.MergedQueryWindows)
	}
}

// TestTraceCausalProjectionCrossWindowPartialOverlapDeductionCap pins the
// deduction bound: a partial cross-window overlap deducts AT MOST the
// physically overlapping wall clock (never the whole member value). A
// [1.000–1.100] 60ms and B [1.090–1.190] 55ms overlap by exactly 10ms →
// union = 60 + (55−10) + 20 = 125, not the 135 SUM and not 60+20. (Member
// values sit outside the V4 ≤3% near band on purpose: exact/near-equal
// overlapping republications are V4's lane and fold BEFORE R2 — the union
// caliber owns the differing-value re-carve shape.)
func TestTraceCausalProjectionCrossWindowPartialOverlapDeductionCap(t *testing.T) {
	records := []ObservationRecord{
		n2Record("E1", "worker-3", "runnable", 60.000, 1.000, 1.100, 100, 200, "0.900..1.150"),
		n2Record("E2", "worker-3", "runnable", 55.000, 1.090, 1.190, 210, 300, "1.050..1.250"),
		n2Record("E3", "worker-3", "runnable", 20.000, 1.300, 1.350, 310, 400, "1.050..1.250"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	agg := n2FindMerged(t, got.OnChainCauses, 3)

	if !agg.MergedIntervalUnion {
		t.Fatalf("cross-window overlapping members must engage the union caliber: %+v", agg)
	}
	if !n2Close(agg.ImpactMS, 125.0) {
		t.Fatalf("partial overlap deduction must be capped at the 10ms physical overlap (want 125.000): %+v", agg)
	}
	if !n2Close(agg.MergedSumMS, 135.0) {
		t.Fatalf("raw Σ must stay lossless in MergedSumMS: %+v", agg)
	}
}

// TestTraceCausalProjectionUnionDensityGateFailsOpenToSum pins the F-2
// structural-premise gate (复核 2026-07-06): the union deduction is only
// sound when every member's value is wall clock contained in its own
// occurrence interval (value ≤ interval length). A density>1 member (a
// multi-CPU cpu·ms-style cumulative: 200ms of value over a 50ms interval)
// must fail the WHOLE group out of the union deduction.
//
// EVOLUTION RECORD (§21 CWD, cmp_01 revisit audit 2026-07-07,
// real_trace_campaign_20260705.md §21 — D-新P0 排队深度方向反转 engine half):
// the original pin asserted the fail-open TARGET was the legacy SUM
// (250.000). §21 rules that OVERLAPPING-query-window magnitudes must never
// SUM (墙钟跨窗不可加和 — the specimen SUMMED 4 overlapping-window
// supply_pressure observations to 34008.569ms and the flagship comparison's
// direction inverted), so for window-overlapping groups the fail-open target
// is now the member MAX with the raw Σ kept lossless in MergedSumMS. The bar
// is NOT lowered: the union deduction still never engages on a density>1
// member (MergedIntervalUnion stays false — the F-2 premise ruling stands),
// and disjoint-window density>1 groups keep the legacy SUM byte-identically
// (second leg below).
//
// Name note (§21.1 CWD-2 ⑦, 2026-07-07): the function name keeps the original
// "FailsOpenToSum" for grep history — after the §21 CWD evolution it names the
// SURVIVING disjoint-window SUM leg only; the overlapping-window leg fails
// open to the member MAX as recorded above.
func TestTraceCausalProjectionUnionDensityGateFailsOpenToSum(t *testing.T) {
	records := []ObservationRecord{
		// value 200ms > interval 50ms → density > 1 → premise broken;
		// windows 0.900..1.150 and 1.020..1.250 OVERLAP.
		n2Record("E1", "worker-5", "runnable", 200.000, 1.000, 1.050, 100, 200, "0.900..1.150"),
		n2Record("E2", "worker-5", "runnable", 30.000, 1.040, 1.100, 210, 300, "1.020..1.250"),
		n2Record("E3", "worker-5", "runnable", 20.000, 1.200, 1.240, 310, 400, "1.020..1.250"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	agg := n2FindMerged(t, got.OnChainCauses, 3)

	if agg.MergedIntervalUnion {
		t.Fatalf("density>1 member must keep the union deduction OFF (F-2 premise): %+v", agg)
	}
	if !n2Close(agg.ImpactMS, 200.0) || !n2Close(agg.CumulativeImpactMS, 200.0) {
		t.Fatalf("§21 CWD: overlapping-window density>1 group must publish the member MAX 200.000, never the SUM: %+v", agg)
	}
	if !agg.MergedCrossWindowMax {
		t.Fatalf("§21 CWD: the row must carry the typed cross-window MAX caliber marker: %+v", agg)
	}
	if !n2Close(agg.MergedSumMS, 250.0) {
		t.Fatalf("§21 CWD: the lossless raw Σ 250.000 must survive in MergedSumMS: %+v", agg)
	}
	// The MAX member (200ms) came from query window 0.900..1.150 — the display
	// density base must be that member's OWN window, never the anchor window.
	if agg.MergedMaxWindowStartTs != 0.9 || agg.MergedMaxWindowEndTs != 1.15 {
		t.Fatalf("§21 CWD: the max member's own query window must ride the row: %+v", agg)
	}
	if len(agg.MergedQueryWindows) != 2 {
		t.Fatalf("window roster disclosure survives the fail-open: %+v", agg.MergedQueryWindows)
	}

	// Second leg (bar keeper): DISJOINT-window density>1 members measured
	// disjoint wall clock — the legacy fail-open-to-SUM stays byte-identical
	// (§21 CWD engages on WINDOW overlap only, a precise typed comparison).
	disjoint := []ObservationRecord{
		n2Record("E1", "worker-6", "runnable", 200.000, 1.000, 1.050, 100, 200, "0.900..1.150"),
		n2Record("E2", "worker-6", "runnable", 30.000, 1.240, 1.300, 210, 300, "1.200..1.450"),
		n2Record("E3", "worker-6", "runnable", 20.000, 1.310, 1.350, 310, 400, "1.200..1.450"),
	}
	got = TraceCausalProjectionFromObservationRecords(disjoint)
	agg = n2FindMerged(t, got.OnChainCauses, 3)
	if !n2Close(agg.ImpactMS, 250.0) || agg.MergedIntervalUnion || agg.MergedCrossWindowMax || agg.MergedSumMS != 0 {
		t.Fatalf("disjoint-window density>1 group must keep the legacy SUM 250.000 fail-open: %+v", agg)
	}
}

// TestTraceCausalProjectionUnionWallClockRedLineUntouched pins the boundary:
// the N2 union caliber lives INSIDE one same-subject ×N row only. The R3
// cross-THREAD background fold keeps the member-MAX ruling (V3: wall clock
// never sums across threads) even when the folded members' ts intervals
// overlap and carry distinct query windows — no code path may reinterpret
// the cross-thread fold as a union or a sum.
func TestTraceCausalProjectionUnionWallClockRedLineUntouched(t *testing.T) {
	bg := func(id, subject string, impact float64, startTs, endTs float64, window string) ObservationRecord {
		record := n2Record(id, subject, "unknown-thread", impact, startTs, endTs, 100, 200, window)
		record.Predicate = "root_cause_background"
		record.ClaimKey = "root_cause_background:" + id
		record.RichNotes = []string{
			fmt.Sprintf("impact_ms=%.3f", impact),
			fmt.Sprintf("cumulative_impact_ms=%.3f", impact),
			"chain_relevance=background", "causality=background",
		}
		if window != "" {
			record.RichNotes = append(record.RichNotes, "selected_window="+window)
		}
		return record
	}
	records := []ObservationRecord{
		bg("E1", "thread-a-1", 117.928, 10.000, 10.120, "9.000..11.000"),
		bg("E2", "thread-b-2", 98.501, 10.010, 10.110, "9.500..10.500"),
		bg("E3", "thread-c-3", 12.689, 10.020, 10.100, "9.000..11.000"),
		bg("E4", "thread-d-4", 6.886, 10.030, 10.090, "9.500..10.500"),
		bg("E5", "thread-e-5", 4.355, 10.040, 10.080, "9.000..11.000"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	var fold *TraceCausalProjectionNode
	for i := range got.BackgroundCauses {
		if got.BackgroundCauses[i].MergedCount > 1 {
			fold = &got.BackgroundCauses[i]
		}
	}
	if fold == nil || fold.Subject != "" {
		t.Fatalf("unknown-thread background fold row missing: %+v", got.BackgroundCauses)
	}
	if fold.ImpactMS != 12.689 || fold.CumulativeImpactMS != 12.689 {
		t.Fatalf("cross-thread fold must keep the member MAX (never union, never sum): %+v", fold)
	}
	if fold.MergedIntervalUnion {
		t.Fatalf("cross-thread fold must never wear the same-subject union caliber: %+v", fold)
	}
}

// TestTraceCausalProjectionIntervalSetAlgebra unit-pins the shared interval
// authority (trace_causal_projection_interval.go) the R2 caliber and the
// future N4/N6 consumers read: merge-on-Add, clipping, union length, and the
// strict validity/overlap guards.
func TestTraceCausalProjectionIntervalSetAlgebra(t *testing.T) {
	var s TraceCausalProjectionIntervalSet
	if !s.Empty() || s.TotalSeconds() != 0 {
		t.Fatalf("zero value must be an empty set: %+v", s)
	}
	// EVOLUTION RECORD (§29.183 G8, 2026-07-21): [0,end] became a VALID
	// interval (rebased traces legally start at ts=0); the negative-start arm
	// replaces the former zero-start invalid representative and the (0,0)
	// absence pair stays excluded through end>start.
	s.Add(2.0, 1.0)  // invalid: end before start
	s.Add(-1.0, 1.0) // invalid: negative start
	s.Add(0, 0)      // invalid: the (0,0) absence pair (zero length)
	s.Add(1.0, 1.0)  // invalid: zero length
	if !s.Empty() {
		t.Fatalf("invalid intervals must be ignored: %+v", s.Spans())
	}
	var zeroStart TraceCausalProjectionIntervalSet
	zeroStart.Add(0, 1.0) // §29.183 G8: a real [0,end] interval unions
	if zeroStart.Empty() || !n2Close(zeroStart.TotalSeconds(), 1.0) {
		t.Fatalf("[0,end] must be a real interval (§29.183 G8): %+v", zeroStart.Spans())
	}
	s.Add(1.0, 2.0)
	s.Add(3.0, 4.0)
	s.Add(1.5, 3.5) // bridges both — one merged span [1.0, 4.0]
	spans := s.Spans()
	if len(spans) != 1 || spans[0].StartTs != 1.0 || spans[0].EndTs != 4.0 {
		t.Fatalf("chain-merge on Add failed: %+v", spans)
	}
	if !n2Close(s.TotalSeconds(), 3.0) {
		t.Fatalf("union length want 3.0: %f", s.TotalSeconds())
	}
	s.Add(5.0, 6.0)
	if !n2Close(s.OverlapSeconds(3.5, 5.5), 1.0) {
		t.Fatalf("clipped overlap across two spans want 1.0 (0.5+0.5): %f", s.OverlapSeconds(3.5, 5.5))
	}
	if s.OverlapSeconds(4.0, 5.0) != 0 {
		t.Fatalf("gap interval must overlap nothing: %f", s.OverlapSeconds(4.0, 5.0))
	}

	if !TraceCausalProjectionIntervalsOverlap(1.0, 2.0, 1.9, 3.0) {
		t.Fatalf("positive-length intersection must overlap")
	}
	if TraceCausalProjectionIntervalsOverlap(1.0, 2.0, 2.0, 3.0) {
		t.Fatalf("touching endpoints share no wall clock")
	}
	// EVOLUTION RECORD (§29.183 G8): the zero-start arm inverted — [0,end]
	// is a real interval and overlaps; the negative-start arm keeps the
	// invalid-never-overlaps guard.
	if !TraceCausalProjectionIntervalsOverlap(0, 2.0, 1.0, 3.0) {
		t.Fatalf("a real [0,end] interval must overlap (§29.183 G8)")
	}
	if TraceCausalProjectionIntervalsOverlap(-1.0, 2.0, 1.0, 3.0) {
		t.Fatalf("invalid interval must never overlap")
	}
}
