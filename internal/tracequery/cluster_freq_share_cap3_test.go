package tracequery

// cluster_freq_share_cap3_test.go — pair-level co-movement criterion unit
// pins. EVOLUTION RECORD (CLUSTERSTREAM-1, §29.193/§29.193.1, 2026-07-21):
// this file pinned the CAP-3 (§29.11) boundary-TRIMMED whole-sequence
// identity (head junction / strict middle / tail exemption). That criterion
// is RETIRED — its single-veto structure sentenced whole clusters on one
// unpaired mid-stream change (fleet case1: 2517 频点行 killed by one
// mid_alignment_mismatch) — and the pins below re-derive every fixture under
// the witness criterion:
//
//	freqTimelinesCoMove = SAME-EMISSION identity (fast path, byte-preserved)
//	                    ∨ (pro ≥ clusterFreqCoWitnessFloor ∧ con == 0)
//
//	pro — paired same-value transitions within the fixed 15µs skew bound;
//	      entry announcements never count (公告不铸见证, §28.5 复核 P1 form
//	      structurally excluded);
//	con — paired DIFFERENT-value transitions inside one skew window
//	      (one-vote veto); only paired transitions compare — carried state
//	      never does (DHMINE §29.172 multi-value co-movement burst is pro,
//	      not con).
//
// Truth-table flips vs the trimmed criterion, each deliberate (§29.193.1):
//
//	mid-stream missing change   split → MERGE  (the case1 heal: one lost row
//	                                            = one lost witness)
//	head carry disagreement     split → MERGE  (announcements are not
//	                                            transitions; two co-witnessed
//	                                            transitions carry the merge)
//	tail cut behind 2 aligned   merge → SPLIT  (the announcement pair no
//	                                            longer counts: pro=1 < floor
//	                                            — 宁漏勿假 on sparse evidence)
//	head+tail trims together    merge → SPLIT  (same floor arithmetic)

import (
	"reflect"
	"testing"
)

func cap3TL(pairs ...[2]float64) []freqSample {
	out := make([]freqSample, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, freqSample{ts: p[0], khz: int64(p[1])})
	}
	return out
}

func TestFreqTimelinesCoMoveWitnessForms(t *testing.T) {
	const tail = 100.0
	cases := []struct {
		name string
		a, b []freqSample
		want bool
	}{
		{
			name: "late_entry_two_witnessed_transitions_merge",
			// b enters late (its first row announces the standing 1618000 it
			// carried); the 1224000 and 1882000 transitions pair → pro=2.
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}, [2]float64{40, 1882000}),
			b:    cap3TL([2]float64{25, 1618000}, [2]float64{30.000001, 1224000}, [2]float64{40.000001, 1882000}),
			want: true,
		},
		{
			name: "entry_announcement_disagreement_does_not_veto",
			// EVOLUTION (§29.193.1, was head_trim_carry_disagreement_splits):
			// b's entry announcement (1500000) is NOT a transition and mints
			// neither pro nor con; the two co-witnessed transitions carry the
			// merge (只比配对变迁,不比携带态).
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}, [2]float64{40, 1882000}),
			b:    cap3TL([2]float64{25, 1500000}, [2]float64{30.000001, 1224000}, [2]float64{40.000001, 1882000}),
			want: true,
		},
		{
			name: "mid_stream_missing_change_heals",
			// EVOLUTION (§29.193.1, was mid_stream_missing_change_splits —
			// the case1 heal form): b lost its 1224000 member row; the pair
			// still accumulates pro=2 (1618000, 1882000) with zero con — one
			// lost row is one lost witness, not a death sentence.
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}, [2]float64{40, 1882000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}, [2]float64{40.000001, 1882000}),
			want: true,
		},
		{
			name: "mid_stream_delayed_change_below_floor_splits",
			// equal values, one change 1ms late (preempted notifier form) —
			// the late transition pairs with nothing inside the FIXED bound
			// (§29.129 既裁③: never widened), leaving pro=1 < floor.
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.001, 1618000}, [2]float64{30.000001, 1224000}),
			want: false,
		},
		{
			name: "reannounced_standing_value_merges",
			// b re-announces 1618000 mid-stream — deduped, not a transition;
			// pro=2 (1618000, 1224000).
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}, [2]float64{25, 1618000}, [2]float64{30.000001, 1224000}),
			want: true,
		},
		{
			name: "parked_constants_offset_entry_split",
			// two clusters parked at one value with different entry times:
			// zero transitions, zero pro — and same-emission fails on the
			// 40s announcement skew.
			a:    cap3TL([2]float64{10, 1430000}),
			b:    cap3TL([2]float64{50, 1430000}),
			want: false,
		},
		{
			name: "parked_constants_shared_entry_announcement_split",
			// §28.5 复核 P1 (a1' REPRO), the announcement false-merge form:
			// two foreign clusters parked at ONE value, first announcements
			// within the skew bound (one all-policy sweep), different
			// re-announce cadence. 公告不铸见证: the shared entry mints no
			// pro — pro=0, split. (Same-emission fails on length.)
			a:    cap3TL([2]float64{10, 1430000}, [2]float64{20, 1430000}, [2]float64{30, 1430000}),
			b:    cap3TL([2]float64{10.000001, 1430000}, [2]float64{15, 1430000}),
			want: false,
		},
		{
			name: "single_co_witnessed_transition_floor_splits",
			// 地板臂 (件5): exactly ONE co-witnessed transition — pro=1 does
			// not merge (§28.5 coincidence floor carried into the witness
			// criterion at the same constant).
			a:    cap3TL([2]float64{10, 1430000}, [2]float64{20, 1530000}),
			b:    cap3TL([2]float64{15, 1430000}, [2]float64{20.000001, 1530000}),
			want: false,
		},
		{
			name: "two_trailing_changes_below_floor_split",
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{90, 1224000}, [2]float64{tail, 1882000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}),
			want: false,
		},
		{
			name: "missing_change_realigned_at_tail_below_floor_splits",
			// b missed 1618000 mid-stream and realigns at the stream tail:
			// pro=1 (1224000) < floor — still splits (the retired in-order
			// gap walk is not resurrected by the witness matcher: matching is
			// value-keyed, so the realigned pair pro-matches and the missing
			// change is simply absent evidence).
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{tail, 1224000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{tail + 1e-6, 1224000}),
			want: false,
		},
		{
			name: "tail_cut_behind_single_aligned_pair_splits",
			// §26 R5d cross-class witness shape: one coincident parked value
			// (announcement — never a witness) + one unpaired trailing
			// transition — pro=0.
			a:    cap3TL([2]float64{10, 2000000}, [2]float64{tail, 2400000}),
			b:    cap3TL([2]float64{10.000001, 2000000}),
			want: false,
		},
		{
			name: "tail_cut_behind_two_aligned_pairs_now_splits",
			// EVOLUTION (§29.193.1, was tail_cut_behind_two_aligned_pairs_
			// merges): the trimmed floor counted the entry-announcement pair;
			// the witness floor does not (公告不铸见证) — pro=1 < 2, honest
			// split on sparse evidence (宁漏勿假; a real capture accumulates
			// pro over the whole file, see the case1-shape pin).
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{tail, 1224000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}),
			want: false,
		},
		{
			name: "head_and_tail_trims_together_below_floor_split",
			// EVOLUTION (§29.193.1, was head_and_tail_trims_together_merge):
			// pro=1 (1224000) < floor under the witness counting — same
			// sparse-evidence arithmetic as the tail-cut flip above.
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}, [2]float64{tail, 1882000}),
			b:    cap3TL([2]float64{20.000001, 1618000}, [2]float64{30.000001, 1224000}),
			want: false,
		},
		{
			name: "case1_shape_one_unpaired_point_among_many_heals",
			// The fleet case1 shape at fixture scale: five co-witnessed
			// transitions, ONE unpaired mid-stream point (a's 1300000 @50 has
			// no b row) — pro=5, con=0 → merge. Under the trimmed criterion
			// this exact shape was a mid_alignment_mismatch death sentence.
			a: cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000},
				[2]float64{40, 1882000}, [2]float64{50, 1300000}, [2]float64{60, 1618000}, [2]float64{70, 1090000}),
			b: cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}, [2]float64{30.000001, 1224000},
				[2]float64{40.000001, 1882000}, [2]float64{60.000001, 1618000}, [2]float64{70.000001, 1090000}),
			want: true,
		},
		{
			name: "persistent_divergence_mints_con_and_splits",
			// 真分歧持续: the two sides transition INSIDE one skew window to
			// DIFFERENT values — con one-vote veto, regardless of the pro
			// witnesses around it (con arm, 件5).
			a: cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000},
				[2]float64{40, 1882000}),
			b: cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}, [2]float64{30.000002, 1300000},
				[2]float64{40.000001, 1882000}),
			want: false,
		},
		{
			name: "dhmine_multi_value_comove_burst_is_pro_not_con",
			// DHMINE §29.172 (donghu cpu12+13 {1675000→1200000}): one burst
			// carries a two-step transition on BOTH members — value-keyed
			// pairing matches 1675000↔1675000 and 1200000↔1200000 (pro=2);
			// a carried-state comparison would have minted a false con
			// (during the 2µs member skew one side reads 1200000 while the
			// other still carries 1675000). b's extra re-announcement breaks
			// the same-emission fast path, so THIS pin exercises the witness
			// lane itself.
			a: cap3TL([2]float64{10, 1430000}, [2]float64{20, 1675000}, [2]float64{20.000005, 1200000}),
			b: cap3TL([2]float64{10.000001, 1430000}, [2]float64{20.000002, 1675000}, [2]float64{20.000007, 1200000},
				[2]float64{25, 1200000}),
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := freqTimelinesCoMove(tc.a, tc.b); got != tc.want {
				t.Fatalf("coMove(a,b) = %v, want %v", got, tc.want)
			}
			// Symmetry: the criterion must not depend on argument order.
			if got := freqTimelinesCoMove(tc.b, tc.a); got != tc.want {
				t.Fatalf("coMove(b,a) = %v, want %v (asymmetric criterion)", got, tc.want)
			}
		})
	}
}

// The historical fast path is byte-preserved: whole-array identity merges —
// including the adjudicated constant-equal-value form — stay exactly as
// CFR-2 pinned them (CLUSTERSTREAM-1 件2: sameEmission remains the SECOND
// merge lane, so all-parked cores with identical cadence keep merging with
// zero transitions — the witness lane alone could never reach them).
func TestFreqTimelinesCoMoveFastPathPreserved(t *testing.T) {
	a := cap3TL([2]float64{10, 1430000}, [2]float64{20, 1430000})
	b := cap3TL([2]float64{10.000001, 1430000}, [2]float64{20.000001, 1430000})
	if !freqTimelinesSameEmission(a, b) {
		t.Fatalf("constant-equal-value identical emission must merge on the fast path")
	}
	if !freqTimelinesCoMove(a, b) {
		t.Fatalf("coMove must include the fast path")
	}
}

// 复核 P3: the Index-global collector is memoized once per Index — repeated
// window-face/fold resolutions share ONE map (read-only by contract).
func TestIndexFreqSampleTimelinesMemoized(t *testing.T) {
	idx := buildTraceIndex(t, "cap3_memo.systrace",
		"  tppmgr-sched-in-5850  (    2) [001] .... 1.000000: cpu_frequency: state=1430000 cpu_id=0\n"+
			"  tppmgr-sched-in-5850  (    2) [001] .... 1.000001: cpu_frequency: state=1430000 cpu_id=1\n")
	m1 := indexFreqSampleTimelines(idx)
	m2 := indexFreqSampleTimelines(idx)
	if reflect.ValueOf(m1).Pointer() != reflect.ValueOf(m2).Pointer() {
		t.Fatalf("collector must memoize on the Index (one map, shared)")
	}
	if len(m1[0]) != 1 || len(m1[1]) != 1 {
		t.Fatalf("memoized content wrong: %v", m1)
	}
	// The fold cache shares the SAME memo (single basis by construction).
	cache := newChainQueryCache(idx, nil)
	cache.buildFreqIndex()
	if reflect.ValueOf(cache.freqByCPU).Pointer() != reflect.ValueOf(m1).Pointer() {
		t.Fatalf("chainQueryCache must share the Index memo, not rebuild a second basis")
	}
}
