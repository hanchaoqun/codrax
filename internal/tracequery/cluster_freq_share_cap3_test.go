package tracequery

// cluster_freq_share_cap3_test.go — CAP-3 (§29.11) co-movement criterion unit
// pins. freqTimelinesCoMove = the historical whole-array identity (fast path,
// byte-preserved) OR the boundary-trimmed STATE alignment
// (freqStateCoMoveTrimmed). Each pin below guards one rule of the trimmed
// form against its strongest mutation:
//
//	head trim + carry agreement   → merge   (unwitnessed region is one-sided)
//	head carry DISAGREEMENT       → split   (junction guard, M7)
//	carry consumption mid-stream  → split   (first-change-only rule, M5)
//	mid-stream missing change     → split   (state carries forward — merging
//	                                         would guess hardware state)
//	mid-stream delayed change     → split   (skew bound retained, M6)
//	re-announced standing value   → merge   (state lane: not a transition)
//	parked constants, offset entry→ split   (zero aligned evidence, M3)
//	parked constants, shared entry→ split   (复核 P1 a1': entry announcement
//	                                         is not a transition — floor=2)
//	carry + single transition     → split   (复核 P1 a2-head variant)
//	two trailing changes          → split   (a cut takes ≤1 burst, M4)
//	trailing change off-tail      → split   (global stream-tail anchor, M8)
//	tail cut behind 1 aligned pair→ split   (evidence floor, M9 — the §26 R5d
//	                                         cross-class witness shape)
//	head + tail trims together    → merge   (one carve has both boundaries)

import (
	"reflect"
	"testing"
)

func cap3TL(pairs ...[2]float64) []freqSample {
	out := make([]freqSample, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, freqSample{ts: p[0], khz: int(p[1])})
	}
	return out
}

func TestFreqTimelinesCoMoveTrimForms(t *testing.T) {
	const tail = 100.0
	cases := []struct {
		name string
		a, b []freqSample
		want bool
	}{
		{
			name: "head_trim_carry_agreement_merges",
			// b enters late (its first row announces the standing 1618000 it
			// carried); a witnessed the transition history before that.
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}, [2]float64{40, 1882000}),
			b:    cap3TL([2]float64{25, 1618000}, [2]float64{30.000001, 1224000}, [2]float64{40.000001, 1882000}),
			want: true,
		},
		{
			name: "head_trim_carry_disagreement_splits",
			// b's entry announcement (1500000) contradicts a's carried state
			// (1618000) at first witness — real disagreement.
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}, [2]float64{40, 1882000}),
			b:    cap3TL([2]float64{25, 1500000}, [2]float64{30.000001, 1224000}, [2]float64{40.000001, 1882000}),
			want: false,
		},
		{
			name: "mid_stream_missing_change_splits",
			// b never announced 1224000 while the stream went on — the trace
			// CLAIMS b held 1618000 there; later changes realign but the
			// divergence is witnessed (carry consumption is first-change-only).
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}, [2]float64{40, 1882000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}, [2]float64{40.000001, 1882000}),
			want: false,
		},
		{
			name: "mid_stream_delayed_change_splits",
			// equal values, one change 1ms late (preempted notifier form) —
			// the states genuinely differ for that 1ms; skew bound holds.
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.001, 1618000}, [2]float64{30.000001, 1224000}),
			want: false,
		},
		{
			name: "reannounced_standing_value_merges",
			// b re-announces 1618000 mid-stream — not a transition; the state
			// functions are identical (dedup form).
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}, [2]float64{25, 1618000}, [2]float64{30.000001, 1224000}),
			want: true,
		},
		{
			name: "parked_constants_offset_entry_split",
			// two clusters parked at one value with different entry times:
			// carry agreement alone, zero co-witnessed transitions — merging
			// would fuse foreign clusters and corrupt the §26 cluster count.
			a:    cap3TL([2]float64{10, 1430000}),
			b:    cap3TL([2]float64{50, 1430000}),
			want: false,
		},
		{
			name: "parked_constants_shared_entry_announcement_split",
			// 复核 P1 (a1' REPRO): two foreign clusters parked at ONE value,
			// first announcements within the skew bound (one all-policy
			// sweep first-emission form), different re-announce cadence
			// afterwards (the old whole-array identity split on length —
			// dedup folds both to a single change point). The one aligned
			// "pair" is the shared entry ANNOUNCEMENT — a shared STATE, not
			// a co-witnessed TRANSITION — below clusterFreqTrimmedMinAligned:
			// split.
			a:    cap3TL([2]float64{10, 1430000}, [2]float64{20, 1430000}, [2]float64{30, 1430000}),
			b:    cap3TL([2]float64{10.000001, 1430000}, [2]float64{15, 1430000}),
			want: false,
		},
		{
			name: "head_trim_carry_plus_single_transition_split",
			// 复核 P1 (a2-head variant): junction carry coincidence + exactly
			// ONE aligned transition — still only one co-witnessed transition,
			// the same coincidence floor §28.5 distrusts: split.
			a:    cap3TL([2]float64{10, 1430000}, [2]float64{20, 1530000}),
			b:    cap3TL([2]float64{15, 1430000}, [2]float64{20.000001, 1530000}),
			want: false,
		},
		{
			name: "two_trailing_changes_split",
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{90, 1224000}, [2]float64{tail, 1882000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}),
			want: false,
		},
		{
			name: "two_trailing_changes_at_tail_split",
			// BOTH extras inside the skew window of the stream tail: still a
			// split — one boundary cut takes member rows of ONE burst, never
			// two distinct transitions (M4: allowance stays exactly 1).
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{tail - 1e-5, 1224000}, [2]float64{tail, 1882000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}),
			want: false,
		},
		{
			name: "missing_change_realigned_at_tail_splits",
			// b missed 1618000 mid-stream and realigns exactly at the stream
			// tail: an in-order gap walk would swallow the divergence into
			// the tail exemption (M5) — the middle stays strictly 1:1.
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{tail, 1224000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{tail + 1e-6, 1224000}),
			want: false,
		},
		{
			name: "trailing_change_off_global_tail_splits",
			// the extra change sits at 50s while the stream tail is 100s —
			// the shorter side witnessed the stream continuing without it.
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{50, 1224000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}),
			want: false,
		},
		{
			name: "tail_cut_behind_single_aligned_pair_splits",
			// §26 R5d cross-class witness shape: one coincident parked value
			// + one real trailing transition — below the tail evidence floor.
			a:    cap3TL([2]float64{10, 2000000}, [2]float64{tail, 2400000}),
			b:    cap3TL([2]float64{10.000001, 2000000}),
			want: false,
		},
		{
			name: "tail_cut_behind_two_aligned_pairs_merges",
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{tail, 1224000}),
			b:    cap3TL([2]float64{10.000001, 1090000}, [2]float64{20.000001, 1618000}),
			want: true,
		},
		{
			name: "head_and_tail_trims_together_merge",
			// one carve straddles a burst at BOTH boundaries: b lost its head
			// row (gate) and a carries the tail row (budget cut took b's).
			a:    cap3TL([2]float64{10, 1090000}, [2]float64{20, 1618000}, [2]float64{30, 1224000}, [2]float64{tail, 1882000}),
			b:    cap3TL([2]float64{20.000001, 1618000}, [2]float64{30.000001, 1224000}),
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := freqTimelinesCoMove(tc.a, tc.b, tail); got != tc.want {
				t.Fatalf("coMove(a,b) = %v, want %v", got, tc.want)
			}
			// Symmetry: the criterion must not depend on argument order.
			if got := freqTimelinesCoMove(tc.b, tc.a, tail); got != tc.want {
				t.Fatalf("coMove(b,a) = %v, want %v (asymmetric criterion)", got, tc.want)
			}
		})
	}
}

// The historical fast path is byte-preserved: whole-array identity merges —
// including the adjudicated constant-equal-value form — stay exactly as
// CFR-2 pinned them, trimmed-form rules notwithstanding.
func TestFreqTimelinesCoMoveFastPathPreserved(t *testing.T) {
	a := cap3TL([2]float64{10, 1430000}, [2]float64{20, 1430000})
	b := cap3TL([2]float64{10.000001, 1430000}, [2]float64{20.000001, 1430000})
	if !freqTimelinesSameEmission(a, b) {
		t.Fatalf("constant-equal-value identical emission must merge on the fast path")
	}
	if !freqTimelinesCoMove(a, b, 20.000001) {
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
	cache := newChainQueryCache(idx)
	cache.buildFreqIndex()
	if reflect.ValueOf(cache.freqByCPU).Pointer() != reflect.ValueOf(m1).Pointer() {
		t.Fatalf("chainQueryCache must share the Index memo, not rebuild a second basis")
	}
}
