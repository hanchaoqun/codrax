package types

// RTC-2 typed-signal pins (docs/design/real_trace_campaign_20260705.md §4
// 案 e2, 批 #67): cross-trace time-base disjointness is a PURE-ARITHMETIC
// precise signal over the partitions' existing typed time surfaces (member
// node StartTs/EndTs envelope ∪ anchor window), consumed only by
// soft-guidance display rows. Pinned here:
//   - TimeBaseSpan is the union envelope and fails closed (ok=false) without
//     time evidence;
//   - TraceCausalProjectionTimeBasesDisjoint requires ≥2 projections, a span
//     on EVERY projection, and an empty intersection on EVERY pair; touching
//     endpoints count as intersecting. Inverting the emptiness comparison
//     flips the disjoint/overlap cases below (mutation must go red).
//
// F1 distinction (anti-ping-pong): the anchor lane still only accepts
// selected_window notes — these helpers consume the envelope for time-base
// COMPARABILITY only and never anchor a window (see the block comment on the
// helpers).

import "testing"

func rtc2Node(start, end float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{Subject: "t-1", StartTs: start, EndTs: end}
}

func TestTraceCausalProjectionTimeBaseSpanEnvelope(t *testing.T) {
	projection := TraceCausalProjection{
		PrimaryRootCauses: []TraceCausalProjectionNode{rtc2Node(34579.470, 34579.480)},
		BackgroundCauses:  []TraceCausalProjectionNode{rtc2Node(34579.450627, 34579.500)},
		SemanticSpans:     []TraceCausalProjectionNode{rtc2Node(34579.590, 34579.595184)},
		WindowStartTs:     34579.472865,
		WindowEndTs:       34579.475857,
	}
	start, end, ok := projection.TimeBaseSpan()
	if !ok || start != 34579.450627 || end != 34579.595184 {
		t.Fatalf("envelope must union member spans and the anchor window: %v..%v ok=%v", start, end, ok)
	}

	// Window-only projections (no node carries its own trace span) still have
	// a time base.
	windowOnly := TraceCausalProjection{WindowStartTs: 2942.244845, WindowEndTs: 2942.245401}
	start, end, ok = windowOnly.TimeBaseSpan()
	if !ok || start != 2942.244845 || end != 2942.245401 {
		t.Fatalf("anchor window alone must yield the span: %v..%v ok=%v", start, end, ok)
	}

	// A point span (start == end) is time evidence.
	point := TraceCausalProjection{OnChainCauses: []TraceCausalProjectionNode{rtc2Node(10.5, 10.5)}}
	if start, end, ok = point.TimeBaseSpan(); !ok || start != 10.5 || end != 10.5 {
		t.Fatalf("point spans must count as time evidence: %v..%v ok=%v", start, end, ok)
	}

	// Fail closed: line-span-only projections carry NO time base; malformed
	// node spans (negative start, end before start, the (0,0) absence pair)
	// never contribute. EVOLUTION RECORD (§29.183 G8, 2026-07-21): the
	// former "zero start" reject arm inverted — a rebased [0,end] span IS
	// time evidence (positive pin below); negative start takes its seat.
	for name, projection := range map[string]TraceCausalProjection{
		"empty":            {},
		"negative start":   {PrimaryRootCauses: []TraceCausalProjectionNode{rtc2Node(-1, 12)}},
		"absence pair":     {PrimaryRootCauses: []TraceCausalProjectionNode{rtc2Node(0, 0)}},
		"end before start": {PrimaryRootCauses: []TraceCausalProjectionNode{rtc2Node(12, 11)}},
		"inverted window":  {WindowStartTs: 12, WindowEndTs: 11},
	} {
		if _, _, ok := projection.TimeBaseSpan(); ok {
			t.Fatalf("%s projection must report no time base", name)
		}
	}
	zeroStart := TraceCausalProjection{PrimaryRootCauses: []TraceCausalProjectionNode{rtc2Node(0, 12)}}
	if start, end, ok = zeroStart.TimeBaseSpan(); !ok || start != 0 || end != 12 {
		t.Fatalf("a rebased [0,end] span must yield the time base (§29.183 G8): %v..%v ok=%v", start, end, ok)
	}
}

func TestTraceCausalProjectionTimeBasesDisjoint(t *testing.T) {
	spanProjection := func(start, end float64) TraceCausalProjection {
		return TraceCausalProjection{PrimaryRootCauses: []TraceCausalProjectionNode{rtc2Node(start, end)}}
	}
	a := spanProjection(34579.450627, 34579.595184) // e2 long capture
	b := spanProjection(2942.244845, 2942.245401)   // e2 short excerpt

	if !TraceCausalProjectionTimeBasesDisjoint([]TraceCausalProjection{a, b}) {
		t.Fatalf("the e2 34579.x vs 2942.x pair must classify as disjoint time bases")
	}
	// Overlap (even partial) → shared time base, zero emission.
	if TraceCausalProjectionTimeBasesDisjoint([]TraceCausalProjection{a, spanProjection(34579.500, 34579.700)}) {
		t.Fatalf("overlapping spans must never claim disjoint time bases")
	}
	// Touching endpoints intersect at an instant → not disjoint.
	if TraceCausalProjectionTimeBasesDisjoint([]TraceCausalProjection{
		spanProjection(10, 20), spanProjection(20, 30),
	}) {
		t.Fatalf("touching endpoints must count as intersecting")
	}
	// Fail closed: a span-less projection blocks the claim for the whole set.
	if TraceCausalProjectionTimeBasesDisjoint([]TraceCausalProjection{a, {}}) {
		t.Fatalf("a projection without time evidence must fail the disjointness claim closed")
	}
	// Single partition / empty set never emit.
	if TraceCausalProjectionTimeBasesDisjoint([]TraceCausalProjection{a}) ||
		TraceCausalProjectionTimeBasesDisjoint(nil) {
		t.Fatalf("fewer than two projections must never claim disjoint time bases")
	}
	// ≥3 partitions: EVERY pair must be disjoint.
	c := spanProjection(9000, 9001)
	if !TraceCausalProjectionTimeBasesDisjoint([]TraceCausalProjection{a, b, c}) {
		t.Fatalf("three pairwise-disjoint spans must classify as disjoint")
	}
	if TraceCausalProjectionTimeBasesDisjoint([]TraceCausalProjection{a, b, spanProjection(2942.245, 2960)}) {
		t.Fatalf("one intersecting pair must fail the whole set")
	}
}

// The compile path feeds the signal: two artifacts whose records carry
// disjoint trace spans → the partitioned set classifies as disjoint; moving
// one artifact's records onto the other's time base flips it.
func TestTraceCausalProjectionSetTimeBasesDisjointFromRecords(t *testing.T) {
	records := func(bStart, bEnd float64) []ObservationRecord {
		return []ObservationRecord{
			partitionTestRecord("a-run", "donghu.systrace", "root_cause_primary", "root_cause_primary:a",
				"main-59566", "running", "144.557", 144.557, 100, 200,
				ObservationSpan{LineStart: 100, LineEnd: 200, StartTs: 34579.450627, EndTs: 34579.595184},
				"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain"),
			partitionTestRecord("b-run", "donghu_short.systrace", "root_cause_primary", "root_cause_primary:b",
				"ColdPool-6", "running", "0.367", 0.367, 10, 20,
				ObservationSpan{LineStart: 10, LineEnd: 20, StartTs: bStart, EndTs: bEnd},
				"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain"),
		}
	}
	disjoint := TraceCausalProjectionSetFromObservationRecords(records(2942.244845, 2942.245401))
	if len(disjoint.Projections) != 2 {
		t.Fatalf("fixture must compile two projections: %+v", disjoint.Projections)
	}
	if !TraceCausalProjectionTimeBasesDisjoint(disjoint.Projections) {
		t.Fatalf("record-level disjoint spans must surface through the compiled set")
	}
	overlapping := TraceCausalProjectionSetFromObservationRecords(records(34579.500, 34579.600))
	if len(overlapping.Projections) != 2 ||
		TraceCausalProjectionTimeBasesDisjoint(overlapping.Projections) {
		t.Fatalf("overlapping record spans must not classify as disjoint: %+v", overlapping.Projections)
	}
}
