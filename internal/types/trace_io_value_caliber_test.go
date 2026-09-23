package types

import (
	"fmt"
	"reflect"
	"testing"
)

func TestTraceIOValueCaliberPublicProjection(t *testing.T) {
	for _, caliber := range []string{TraceIOValueCaliberRQResidence, TraceIOValueCaliberBIOResidence,
		TraceIOValueCaliberIssuerBlocked, TraceIOValueCaliberMixed, "", "future_unknown"} {
		for _, closure := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/closed=%t", caliber, closure), func(t *testing.T) {
				record := traceProjectionTestRootWithNotes("io", "worker-12", "io_latency", "31.000", 31, .9, 1,
					[]string{"type=io_latency", "chain_relevance=on_chain", "io_value_caliber=" + caliber,
						fmt.Sprintf("resource_completion_closure=%t", closure)})
				record.Summary = "request residence and issuer wait disagree; prose has no authority"
				projection := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{record}})
				if len(projection.RankedSeats) != 1 {
					t.Fatalf("missing ranked record: %+v", projection)
				}
				node := projection.RankedSeats[0]
				if node.IOValueCaliber != NormalizeTraceIOValueCaliber(caliber) || node.ImpactMS != 31 || node.Rank != 1 {
					t.Fatalf("selected number/caliber lost or mutated: %+v", node)
				}
			})
		}
	}
}

func TestTraceIOValueCaliberFoldDoesNotBorrow(t *testing.T) {
	for _, tc := range []struct{ a, b, want string }{
		{TraceIOValueCaliberRQResidence, TraceIOValueCaliberRQResidence, TraceIOValueCaliberRQResidence},
		{TraceIOValueCaliberIssuerBlocked, TraceIOValueCaliberIssuerBlocked, TraceIOValueCaliberIssuerBlocked},
		{TraceIOValueCaliberRQResidence, TraceIOValueCaliberIssuerBlocked, TraceIOValueCaliberMixed},
		{TraceIOValueCaliberRQResidence, TraceIOValueCaliberBIOResidence, TraceIOValueCaliberMixed},
		{"", TraceIOValueCaliberIssuerBlocked, TraceIOValueCaliberMixed},
		{TraceIOValueCaliberIssuerBlocked, "", TraceIOValueCaliberMixed},
		{"unknown", "", ""},
	} {
		t.Run(tc.a+"/"+tc.b, func(t *testing.T) {
			nodes := []TraceCausalProjectionNode{
				{EvidenceID: "a", Subject: "worker", Object: "io_latency", ImpactMS: 4, IOValueCaliber: tc.a},
				{EvidenceID: "b", Subject: "worker", Object: "io_latency", ImpactMS: 4, IOValueCaliber: tc.b, ResourceCompletionClosure: true},
			}
			same := nodes[0]
			traceCausalProjectionAbsorbSameFact(&same, nodes[1], map[string]bool{}, nil)
			if same.IOValueCaliber != NormalizeTraceIOValueCaliber(tc.a) || same.ImpactMS != 4 {
				t.Fatalf("same-fact fold must retain the surviving value's ruler: %+v", same)
			}
			merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1})
			if merged.IOValueCaliber != tc.want {
				t.Fatalf("family inherited first ruler: %+v want %s", merged, tc.want)
			}
		})
	}
}

func TestTraceIOValueCaliberPublicOccurrenceValueOwners(t *testing.T) {
	for _, mode := range []string{"sum", "sum_zero_member", "max", "max_reversed", "max_tie", "max_unknown_owner", "union_contained", "union_partial"} {
		t.Run(mode, func(t *testing.T) {
			rows := []TraceCausalProjectionNode{
				{EvidenceID: "a", Subject: "worker", Object: "io_latency", ImpactMS: 8, CumulativeImpactMS: 8, IOValueCaliber: TraceIOValueCaliberRQResidence},
				{EvidenceID: "b", Subject: "worker", Object: "io_latency", ImpactMS: 4, CumulativeImpactMS: 4, IOValueCaliber: TraceIOValueCaliberIssuerBlocked},
			}
			want, value := TraceIOValueCaliberMixed, 12.0
			switch mode {
			case "sum_zero_member":
				rows[1].ImpactMS, rows[1].CumulativeImpactMS = 0, 0
				want, value = TraceIOValueCaliberRQResidence, 8
			case "max", "max_reversed", "max_tie", "max_unknown_owner":
				rows[0].QueryWindowStartTs, rows[0].QueryWindowEndTs = 1, 2
				rows[1].QueryWindowStartTs, rows[1].QueryWindowEndTs = 1.2, 2.2
				want, value = TraceIOValueCaliberRQResidence, 8
				if mode == "max_tie" {
					rows[1].ImpactMS, rows[1].CumulativeImpactMS = 8, 8
				}
				if mode == "max_unknown_owner" {
					rows[0].IOValueCaliber, want = "", ""
				}
				if mode == "max_reversed" {
					rows[0], rows[1] = rows[1], rows[0]
				}
			case "union_contained", "union_partial":
				rows[0].QueryWindowStartTs, rows[0].QueryWindowEndTs = 1, 2
				rows[1].QueryWindowStartTs, rows[1].QueryWindowEndTs = 1.2, 2.2
				rows[0].StartTs, rows[0].EndTs = 1.5, 1.508
				rows[1].StartTs, rows[1].EndTs = 1.502, 1.506
				want, value = TraceIOValueCaliberRQResidence, 8
				if mode == "union_partial" {
					rows[1].StartTs, rows[1].EndTs = 1.506, 1.510
					want, value = TraceIOValueCaliberMixed, 10
				}
			}
			without := append([]TraceCausalProjectionNode(nil), rows...)
			for i := range without {
				without[i].IOValueCaliber = ""
			}
			baseline := TraceCausalProjectionMergeOccurrenceRows(without)
			got := TraceCausalProjectionMergeOccurrenceRows(rows)
			if got.IOValueCaliber != want || !n2Close(got.ImpactMS, value) {
				t.Fatalf("%s ruler/value = %q/%.9f, want %q/%.9f", mode, got.IOValueCaliber, got.ImpactMS, want, value)
			}
			got.IOValueCaliber = ""
			if !reflect.DeepEqual(got, baseline) {
				t.Fatal("measurement metadata changed an existing numeric, causal or membership field")
			}
		})
	}
}

func TestTraceIOValueCaliberSameFactSelectedAxes(t *testing.T) {
	for _, mode := range []string{"own", "unknown_own", "adopt_cumulative", "replace_cumulative", "anchored_cumulative_kept", "adopt_effective", "adopt_effective_only", "adopt_actual", "adopt_target"} {
		t.Run(mode, func(t *testing.T) {
			a := TraceCausalProjectionNode{ImpactMS: 8, IOValueCaliber: TraceIOValueCaliberRQResidence}
			b := TraceCausalProjectionNode{ImpactMS: 8, IOValueCaliber: TraceIOValueCaliberIssuerBlocked}
			want := TraceIOValueCaliberRQResidence
			switch mode {
			case "unknown_own":
				a.IOValueCaliber, want = "", ""
			case "adopt_cumulative", "replace_cumulative":
				a.ImpactMS, b.CumulativeImpactMS = 0, 9
				if mode == "replace_cumulative" {
					a.CumulativeImpactMS = 5
				}
				want = TraceIOValueCaliberIssuerBlocked
			case "anchored_cumulative_kept":
				// Earlier disclosure backfill guards the later numeric MAX;
				// merely seeing the original empty anchor cannot select a donor.
				a.CumulativeImpactMS, b.CumulativeImpactMS, b.ChainAnchorFullMS = 5, 9, 10
			case "adopt_effective", "adopt_effective_only":
				b.EffectiveImpactMS, b.EffectiveImpactPublished = 4, true
				want = TraceIOValueCaliberMixed
				if mode == "adopt_effective_only" {
					a.ImpactMS, want = 0, TraceIOValueCaliberIssuerBlocked
				}
			case "adopt_actual":
				b.ActualImpactMS, want = 12, TraceIOValueCaliberMixed
			case "adopt_target":
				b.TargetImpactMS, want = 12, TraceIOValueCaliberMixed
			}
			plainA, plainB := a, b
			plainA.IOValueCaliber, plainB.IOValueCaliber = "", ""
			traceCausalProjectionAbsorbSameFact(&plainA, plainB, map[string]bool{}, nil)
			traceCausalProjectionAbsorbSameFact(&a, b, map[string]bool{}, nil)
			if a.IOValueCaliber != want {
				t.Fatalf("%s = %q, want %q", mode, a.IOValueCaliber, want)
			}
			a.IOValueCaliber = ""
			if !reflect.DeepEqual(a, plainA) {
				t.Fatal("same-fact metadata changed another value or field")
			}
		})
	}
}

func TestTraceIOValueCaliberBackgroundValueOwners(t *testing.T) {
	for _, mode := range []string{"mirror_contained", "mirror_partial", "mirror_max", "overflow_max", "overflow_tie", "overflow_unknown_owner"} {
		t.Run(mode, func(t *testing.T) {
			rows := []TraceCausalProjectionNode{
				{Subject: "worker", Object: "io_latency", ImpactMS: 8, StartTs: 1.5, EndTs: 1.508, IOValueCaliber: TraceIOValueCaliberRQResidence},
				{Subject: "worker", Object: "io_latency", ImpactMS: 4, StartTs: 1.502, EndTs: 1.506, IOValueCaliber: TraceIOValueCaliberIssuerBlocked},
			}
			want, value := TraceIOValueCaliberRQResidence, 8.0
			merge := func(in []TraceCausalProjectionNode) TraceCausalProjectionNode {
				return traceCausalProjectionMergeSameKindMembersLane(in, 0, []int{0, 1}, true)
			}
			switch mode {
			case "mirror_partial":
				rows[1].StartTs, rows[1].EndTs = 1.506, 1.510
				want, value = TraceIOValueCaliberMixed, 10
			case "mirror_max":
				rows[0].ImpactMS, value = 12, 12
			case "overflow_max", "overflow_tie", "overflow_unknown_owner":
				rows[0].Object, rows[1].Object = "unknown-thread", "unknown-thread"
				if mode == "overflow_tie" {
					rows[1].ImpactMS = rows[0].ImpactMS
				}
				if mode == "overflow_unknown_owner" {
					rows[0].IOValueCaliber, want = "", ""
				}
				kept := make([]TraceCausalProjectionNode, traceCausalProjectionUnknownBackgroundKeep)
				for i := range kept {
					kept[i] = TraceCausalProjectionNode{Subject: fmt.Sprint(i), Object: "unknown-thread", ImpactMS: 100}
				}
				rows = append(kept, rows...)
				merge = func(in []TraceCausalProjectionNode) TraceCausalProjectionNode {
					out := traceCausalProjectionFoldUnknownBackground(in)
					if len(out) != len(kept)+1 {
						t.Fatalf("did not reach unknown background fold: %d", len(out))
					}
					return out[len(out)-1]
				}
			}
			plain := append([]TraceCausalProjectionNode(nil), rows...)
			for i := range plain {
				plain[i].IOValueCaliber = ""
			}
			baseline, got := merge(plain), merge(rows)
			if got.IOValueCaliber != want || !n2Close(got.ImpactMS, value) {
				t.Fatalf("%s ruler/value = %q/%.9f, want %q/%.9f", mode, got.IOValueCaliber, got.ImpactMS, want, value)
			}
			got.IOValueCaliber = ""
			if !reflect.DeepEqual(got, baseline) {
				t.Fatal("background metadata changed another value or field")
			}
		})
	}
}
