package tracequery

import (
	"strings"
	"testing"
)

// WINFLAG-1 pins (§29.190④, 2026-07-21). Three arms per point:
//   正臂  — flagged real-0 window ([0,end] explicit / rebased whole-trace
//           backfill) restores the anchor/interval/envelope/identity lanes;
//   缺席臂 — the line-anchored unset form keeps every legacy absence;
//   兼容臂 — flag-off behavior is byte-identical to the pre-WINFLAG guards.

// --- carrier & derivation ---------------------------------------------------

func TestWinflagQueryResultTimeWindowStartSetArms(t *testing.T) {
	// Explicit time_start=0 (the rebased [0,end] anchor form) → set.
	q := Query{TimeStart: 0, TimeEnd: 2.0, TimeStartSet: true}
	if w := queryResultTimeWindow(q); !w.StartSet || !w.StartDetermined() || !w.StartsAtDeterminedZero() {
		t.Fatalf("explicit 0 start must stamp the flag: %+v", w)
	}
	// Positive start (no parse flag) → set by value, not a zero-start run.
	q = Query{TimeStart: 1.5, TimeEnd: 2.0}
	if w := queryResultTimeWindow(q); !w.StartSet || !w.StartDetermined() || w.StartsAtDeterminedZero() {
		t.Fatalf("positive start must be determined and non-zero-start: %+v", w)
	}
	// Line-anchored unset form → NOT set (the honest-absence family).
	q = Query{TimeStart: 0, TimeEnd: 5.0, LineStart: 100}
	if w := queryResultTimeWindow(q); w.StartSet || w.StartDetermined() {
		t.Fatalf("line-anchored unset 0 must stay indeterminate: %+v", w)
	}
}

func TestWinflagNormalizeQueryBackfillStampsProvenance(t *testing.T) {
	// Rebased trace: FirstTs is a real 0 — the whole-trace backfill must mark
	// the result window start DETERMINED (parity with FirstTs>0 traces).
	idx := &Index{FirstTs: 0, LastTs: 3.5}
	q := normalizeQuery(idx, Query{})
	if !q.timeStartBackfilled {
		t.Fatalf("whole-trace backfill must stamp the provenance flag: %+v", q)
	}
	if w := queryResultTimeWindow(q); !w.StartSet || !w.StartsAtDeterminedZero() {
		t.Fatalf("rebased whole-trace window must be a determined [0,end]: %+v", w)
	}
	if !queryWindowStartsAtDeterminedZero(q) {
		t.Fatalf("query-side twin must agree on the determined zero start")
	}
	// Line-anchored queries skip the backfill: no stamp, no determination.
	q = normalizeQuery(idx, Query{LineStart: 10})
	if q.timeStartBackfilled || queryResultTimeWindow(q).StartSet {
		t.Fatalf("line-anchored query must keep the unset-0 sentinel: %+v", q)
	}
	// Non-rebased trace: backfill writes a positive FirstTs — behavior parity
	// with the legacy value path (flag additionally set, values identical).
	idx = &Index{FirstTs: 1.25, LastTs: 3.5}
	q = normalizeQuery(idx, Query{})
	if q.TimeStart != 1.25 || !queryResultTimeWindow(q).StartSet {
		t.Fatalf("positive backfill keeps its value and is determined: %+v", q)
	}
}

func TestWinflagRankFoldStartUsableTruthTable(t *testing.T) {
	cases := []struct {
		start, end float64
		flag, want bool
	}{
		{1.0, 2.0, false, true},  // legacy positive interval
		{0, 2.0, false, false},   // 兼容臂: unflagged 0 stays the absence sentinel
		{0, 2.0, true, true},     // 正臂: flagged real 0 start
		{0, 0, true, false},      // zero-length absence never revives, even flagged
		{-0.5, 2.0, true, false}, // negative start never usable
		{1.0, 1.0, false, false}, // degenerate
	}
	for _, tc := range cases {
		if got := rankFoldStartUsable(tc.start, tc.end, tc.flag); got != tc.want {
			t.Fatalf("rankFoldStartUsable(%v, %v, %v) = %v, want %v", tc.start, tc.end, tc.flag, got, tc.want)
		}
	}
}

// --- (c) fold points ---------------------------------------------------------

func TestWinflagMemberIntervalInventoryZeroStartArms(t *testing.T) {
	fam := SemanticSpanFamily{Members: []TraceSpanSummary{
		{Name: "a", StartTs: 0, EndTs: 0.5, StartLine: 1, EndLine: 2},
		{Name: "b", StartTs: 0.6, EndTs: 0.9, StartLine: 3, EndLine: 4},
	}}
	// 兼容臂: without the flag the zero-start member voids the inventory
	// (all-or-nothing, byte-identical to the legacy reject).
	if got := fam.memberIntervalInventory(false); got != nil {
		t.Fatalf("unflagged zero-start member must void the inventory, got %v", got)
	}
	// 正臂: flagged real 0 keeps the COMPLETE inventory.
	got := fam.memberIntervalInventory(true)
	if len(got) != 2 || got[0].start != 0 || got[0].end != 0.5 {
		t.Fatalf("flagged inventory must keep the real 0-start span: %v", got)
	}
	// A genuinely absent (0,0) member still voids it, flag or not.
	fam.Members = append(fam.Members, TraceSpanSummary{Name: "c"})
	if got := fam.memberIntervalInventory(true); got != nil {
		t.Fatalf("(0,0) absence must void the inventory even flagged, got %v", got)
	}
}

func TestWinflagFamilyMemberKeyZeroStartArms(t *testing.T) {
	member := RootCauseRankItem{StartTs: 0, EndTs: 0.25, LineStart: 7, LineEnd: 9}
	// 兼容臂: unflagged 0-start falls to the line-range identity.
	if got := rootCauseFamilyMemberKey(member, false); got != "lines 7-9" {
		t.Fatalf("unflagged key must stay on the line fallback, got %q", got)
	}
	// 正臂: flagged real 0 keeps the window-form identity.
	if got := rootCauseFamilyMemberKey(member, true); got != "0.000000..0.250000" {
		t.Fatalf("flagged key must wear the window form, got %q", got)
	}
	// Mint-time MemberKey always wins, flag or not.
	member.MemberKey = "cpu=3"
	if got := rootCauseFamilyMemberKey(member, true); got != "cpu=3" {
		t.Fatalf("typed MemberKey must stay authoritative, got %q", got)
	}
}

func TestWinflagMergeFamilyEnvelopeZeroStartFloor(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "runnable_wait", Thread: ThreadRef{Comm: "w", PID: 9}, StartTs: 0, EndTs: 0.4,
			LineStart: 10, LineEnd: 20, CumulativeImpactMs: 8, ImpactMs: 8, Confidence: 0.8},
		{Type: "runnable_wait", Thread: ThreadRef{Comm: "w", PID: 9}, StartTs: 0.5, EndTs: 0.9,
			LineStart: 30, LineEnd: 40, CumulativeImpactMs: 3, ImpactMs: 3, Confidence: 0.8},
	}
	// 正臂: flagged run — the base's real ts==0 envelope floor holds; the
	// later positive member must not raise it.
	flagged := Query{TimeStart: 0, TimeEnd: 1.0, TimeStartSet: true}
	merged := mergeSameThreadTypeRankFamily(flagged, false, items, []int{0, 1})
	if merged.StartTs != 0 || merged.EndTs != 0.9 {
		t.Fatalf("flagged envelope must keep the real 0 floor: %v..%v", merged.StartTs, merged.EndTs)
	}
	// 正臂 (reverse order): a real 0-start member lowers a positive envelope.
	mergedRev := mergeSameThreadTypeRankFamily(flagged, false,
		[]RootCauseRankItem{items[1], items[0]}, []int{0, 1})
	// Representative sorting puts the larger member (the 0-start one) first
	// either way; assert the envelope regardless of roster order.
	if mergedRev.StartTs != 0 || mergedRev.EndTs != 0.9 {
		t.Fatalf("flagged envelope must reach the real 0 member: %v..%v", mergedRev.StartTs, mergedRev.EndTs)
	}
	// 兼容臂: unflagged run keeps the legacy fold — 0 reads as "unset yet"
	// and the positive member start becomes the envelope.
	unflagged := Query{TimeStart: 0, TimeEnd: 1.0, LineStart: 5}
	legacy := mergeSameThreadTypeRankFamily(unflagged, false, items, []int{0, 1})
	if legacy.StartTs != 0.5 {
		t.Fatalf("unflagged envelope must keep the legacy behavior, got %v", legacy.StartTs)
	}
	// 正臂 roster face: the flagged fold's roster wears the window-form key
	// for the real 0-start member (identity no longer degrades to lines).
	foundWindowKey := false
	for _, entry := range merged.MemberRoster {
		if strings.HasPrefix(entry, "0.000000..0.400000") {
			foundWindowKey = true
		}
	}
	if !foundWindowKey {
		t.Fatalf("flagged roster must carry the window-form member key: %v", merged.MemberRoster)
	}
}

func TestWinflagSelfGapDisclosureZeroStartSpanArm(t *testing.T) {
	items := []RootCauseRankItem{
		{
			Source: "thread_timeline.self_running_fold", SubjectIsAnalysisTarget: true,
			selfGapRunningIntervals: []foldInterval{{start: 0.0, end: 0.2}},
			LineStart:               1, LineEnd: 2,
		},
		{
			SubjectIsAnalysisTarget: true, SemanticClass: "binder_transaction",
			StartTs: 0, EndTs: 0.1, LineStart: 5, LineEnd: 8,
		},
	}
	// 兼容臂: unflagged — the zero-start single-member span mints no claim.
	stampSelfGapSemanticOverlapDisclosure(items, false)
	if len(items[0].SelfGapSemanticOverlaps) != 0 {
		t.Fatalf("unflagged zero-start span must mint nothing: %+v", items[0].SelfGapSemanticOverlaps)
	}
	// 正臂: flagged — the real [0,0.1] span overlaps the running union.
	stampSelfGapSemanticOverlapDisclosure(items, true)
	if len(items[0].SelfGapSemanticOverlaps) != 1 {
		t.Fatalf("flagged real 0-start span must mint the overlap claim: %+v", items[0].SelfGapSemanticOverlaps)
	}
	if got := items[0].SelfGapSemanticOverlaps[0].OverlapMs; got < 99.9 || got > 100.1 {
		t.Fatalf("overlap must be the exact 100ms intersection, got %v", got)
	}
}

func TestWinflagDemoteLockDominatedZeroStartWaitArm(t *testing.T) {
	mkItems := func() []RootCauseRankItem {
		return []RootCauseRankItem{{
			Type: "priority_inversion_candidate", Thread: ThreadRef{Comm: "w", PID: 7},
			StartTs: 0, EndTs: 0.05, Summary: "inversion candidate",
		}}
	}
	chain := ChainResult{Target: ThreadRef{Comm: "app", PID: 7}}
	stats := WindowStats{
		Window: TimeWindow{StartTs: 0, EndTs: 0.1, StartSet: true},
		TraceSpans: []TraceSpanSummary{{
			Thread: ThreadRef{Comm: "app", PID: 7}, Kind: "monitor_contention",
			Name: "Lock contention on a monitor lock (owner tid: 99)", StartTs: 0, EndTs: 0.06,
			StartLine: 1, EndLine: 2,
		}},
	}
	// 正臂: flagged stats window — the real [0,0.05] wait is eligible and the
	// covering resolved lock demotes it.
	items := mkItems()
	demoteLockDominatedInversionCandidates(chain, stats, items)
	if !items[0].PriorityInversionLockDominated {
		t.Fatalf("flagged zero-start wait must be demotable: %+v", items[0])
	}
	// 兼容臂: same geometry without the flag skips the item (legacy guard).
	stats.Window.StartSet = false
	items = mkItems()
	demoteLockDominatedInversionCandidates(chain, stats, items)
	if items[0].PriorityInversionLockDominated {
		t.Fatalf("unflagged zero-start wait must keep the legacy skip: %+v", items[0])
	}
}

func TestWinflagStreamStateAccumulateZeroStartEnvelope(t *testing.T) {
	th := ThreadRef{Comm: "w", PID: 3}
	// 正臂: flagged — plain min: the real 0-start segment holds the floor and
	// a later positive segment must not raise it.
	dst := map[string]ThreadDuration{}
	streamStateAccumulateDuration(dst, ThreadDuration{Thread: th, DurationMs: 1, StartTs: 0, EndTs: 0.001, LineStart: 1, LineEnd: 2}, true)
	streamStateAccumulateDuration(dst, ThreadDuration{Thread: th, DurationMs: 1, StartTs: 0.5, EndTs: 0.501, LineStart: 3, LineEnd: 4}, true)
	got := dst[threadKey(th)]
	if got.StartTs != 0 || got.EndTs != 0.501 {
		t.Fatalf("flagged envelope must keep the real 0 floor: %+v", got)
	}
	// 正臂 (reverse): a real 0-start segment lowers a positive envelope.
	dst = map[string]ThreadDuration{}
	streamStateAccumulateDuration(dst, ThreadDuration{Thread: th, DurationMs: 1, StartTs: 0.5, EndTs: 0.501, LineStart: 3, LineEnd: 4}, true)
	streamStateAccumulateDuration(dst, ThreadDuration{Thread: th, DurationMs: 1, StartTs: 0, EndTs: 0.001, LineStart: 1, LineEnd: 2}, true)
	if got := dst[threadKey(th)]; got.StartTs != 0 {
		t.Fatalf("flagged min must reach the real 0 segment: %+v", got)
	}
	// 兼容臂: unflagged keeps the legacy merge — the 0 envelope reads as
	// "unset yet" and is overwritten by the positive segment.
	dst = map[string]ThreadDuration{}
	streamStateAccumulateDuration(dst, ThreadDuration{Thread: th, DurationMs: 1, StartTs: 0, EndTs: 0.001, LineStart: 1, LineEnd: 2}, false)
	streamStateAccumulateDuration(dst, ThreadDuration{Thread: th, DurationMs: 1, StartTs: 0.5, EndTs: 0.501, LineStart: 3, LineEnd: 4}, false)
	if got := dst[threadKey(th)]; got.StartTs != 0.5 {
		t.Fatalf("unflagged merge must keep the legacy behavior: %+v", got)
	}
}

// --- recon boundary ----------------------------------------------------------

// The recon compares seat windows by struct equality; the flag is a
// result-side carrier, never part of the window VALUE identity — a flagged
// query window and its flag-free stats-scalar twin must stay equal.
func TestWinflagCrossTypeReconWindowIdentityFlagBlind(t *testing.T) {
	item := RootCauseRankItem{Source: "wakeup_chain.causal_impacts"}
	flagged := TimeWindow{StartTs: 1.0, EndTs: 2.0, StartSet: true}
	got, ok := crossTypeRankSeatWindow(item, flagged)
	if !ok {
		t.Fatalf("chain-lane window must resolve")
	}
	twin := RootCauseRankItem{StatsWindowStartTs: 1.0, StatsWindowEndTs: 2.0}
	scalar, ok := crossTypeRankSeatWindow(twin, TimeWindow{})
	if !ok {
		t.Fatalf("stats-scalar window must resolve")
	}
	if got != scalar {
		t.Fatalf("window identity must be flag-blind: %+v vs %+v", got, scalar)
	}
}
