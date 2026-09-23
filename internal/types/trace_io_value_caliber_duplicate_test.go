package types

import (
	"reflect"
	"testing"
)

func TestTraceIOValueCaliberDuplicatePublicationSelectedAxes(t *testing.T) {
	for _, mode := range []string{"adopt", "keep", "unknown_donor", "split_cumulative", "retained_effective", "adopt_target", "exact", "exact_unknown"} {
		t.Run(mode, func(t *testing.T) {
			a := TraceCausalProjectionNode{EvidenceID: "a", ImpactMS: 1, CumulativeImpactMS: 1, IOValueCaliber: TraceIOValueCaliberRQResidence}
			b := TraceCausalProjectionNode{EvidenceID: "b", ImpactMS: 1.03, CumulativeImpactMS: 1.03, IOValueCaliber: TraceIOValueCaliberIssuerBlocked}
			want := TraceIOValueCaliberIssuerBlocked
			switch mode {
			case "keep":
				a.ImpactMS, a.CumulativeImpactMS, b.ImpactMS, b.CumulativeImpactMS = 1.03, 1.03, 1, 1
				want = TraceIOValueCaliberRQResidence
			case "unknown_donor":
				b.IOValueCaliber, want = "", ""
			case "split_cumulative":
				a.CumulativeImpactMS, want = 2, TraceIOValueCaliberMixed
			case "retained_effective":
				a.EffectiveImpactMS, want = .5, TraceIOValueCaliberMixed
			case "adopt_target":
				a.ImpactMS, a.CumulativeImpactMS, b.ImpactMS, b.CumulativeImpactMS = 1.03, 1.03, 1, 1
				b.TargetImpactMS, want = 2, TraceIOValueCaliberMixed
			case "exact", "exact_unknown":
				// Equal Impact intentionally leaves even a larger second axis alone.
				b.ImpactMS, b.CumulativeImpactMS, b.TargetImpactMS = 1, 2, 3
				want = TraceIOValueCaliberRQResidence
				if mode == "exact_unknown" {
					a.IOValueCaliber, want = "", ""
				}
			}
			plainA, plainB := a, b
			plainA.IOValueCaliber, plainB.IOValueCaliber = "", ""
			traceCausalProjectionAbsorbDuplicatePublication(&plainA, plainB)
			traceCausalProjectionAbsorbDuplicatePublication(&a, b)
			if a.IOValueCaliber != want {
				t.Fatalf("%s ruler = %q, want %q", mode, a.IOValueCaliber, want)
			}
			a.IOValueCaliber = ""
			if !reflect.DeepEqual(a, plainA) {
				t.Fatal("duplicate-publication metadata changed a numeric, evidence or causal field")
			}
		})
	}
}

func TestTraceIOValueCaliberPublicDuplicatePublicationValueOwners(t *testing.T) {
	for _, mode := range []string{"max", "split_axes", "unknown_max"} {
		t.Run(mode, func(t *testing.T) {
			plain := []ObservationRecord{
				nearDedupRecord("a", 1, 2908, 3094),
				nearDedupRecord("b", 1.03, 2911, 3114),
			}
			want, cumulative := TraceIOValueCaliberIssuerBlocked, 1.03
			if mode == "split_axes" {
				// The existing sort selects the larger cumulative estimate first;
				// V4 then adopts the other publication's larger Impact, not its ruler.
				plain[0].RichNotes[1] = "cumulative_impact_ms=2.000"
				want, cumulative = TraceIOValueCaliberMixed, 2
			}
			baseline := TraceCausalProjectionFromObservationRecords(plain)
			records := append([]ObservationRecord(nil), plain...)
			rulers := []string{TraceIOValueCaliberRQResidence, TraceIOValueCaliberIssuerBlocked}
			if mode == "unknown_max" {
				rulers[1], want = "", ""
			}
			for i := range records {
				records[i].RichNotes = append(append([]string(nil), records[i].RichNotes...), "io_value_caliber="+rulers[i])
			}
			got := TraceCausalProjectionFromObservationRecords(records)
			if len(got.AdjacentCauses) != 1 || len(baseline.AdjacentCauses) != 1 {
				t.Fatalf("native IO publication pair did not fold: %+v", got)
			}
			node := got.AdjacentCauses[0]
			if node.DuplicatePublications != 2 || node.ImpactMS != 1.03 || node.CumulativeImpactMS != cumulative || node.IOValueCaliber != want {
				t.Fatalf("wrong value owner for %s: %+v", mode, node)
			}
			node.IOValueCaliber = ""
			if !reflect.DeepEqual(node, baseline.AdjacentCauses[0]) {
				t.Fatal("public projection changed fields beyond the IO ruler")
			}
		})
	}
}

func TestTraceIOValueCaliberOnChainOverflowValueOwner(t *testing.T) {
	for _, mode := range []string{"max", "non_io_max", "unknown_max", "tie", "nested_io", "nested_non_io"} {
		t.Run(mode, func(t *testing.T) {
			rows := []TraceCausalProjectionNode{
				{EvidenceID: "a", Subject: "worker-1", TypeToken: "io_latency", ImpactMS: 4, IOValueCaliber: TraceIOValueCaliberRQResidence},
				{EvidenceID: "b", Subject: "worker-2", TypeToken: "io_latency", ImpactMS: 8, IOValueCaliber: TraceIOValueCaliberIssuerBlocked},
			}
			want, value := TraceIOValueCaliberIssuerBlocked, 8.0
			switch mode {
			case "non_io_max", "nested_non_io":
				rows[1].TypeToken, rows[1].IOValueCaliber, want = "running", "", ""
			case "unknown_max":
				rows[1].IOValueCaliber, want = "", ""
			case "tie":
				rows[1].ImpactMS, value, want = 4, 4, TraceIOValueCaliberRQResidence
			}
			fold := func(in []TraceCausalProjectionNode) TraceCausalProjectionNode {
				out := traceCausalProjectionOverflowFoldRow(in)
				if mode == "nested_io" || mode == "nested_non_io" {
					out = traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{in[0], out})
				}
				return out
			}
			plain := append([]TraceCausalProjectionNode(nil), rows...)
			for i := range plain {
				plain[i].IOValueCaliber = ""
			}
			baseline, got := fold(plain), fold(rows)
			if got.IOValueCaliber != want || got.ImpactMS != value || got.TypeToken != "" || got.Object != "" {
				t.Fatalf("overflow value owner changed or invented an IO identity: %+v", got)
			}
			got.IOValueCaliber = ""
			if !reflect.DeepEqual(got, baseline) {
				t.Fatal("overflow metadata changed numeric values, membership or qualification")
			}
		})
	}
}

func TestTraceIOValueCaliberFoldPublishedSecondaryAxes(t *testing.T) {
	for _, mode := range []string{"max_target", "max_cleared_effective", "max_actual", "sum_zero_target", "sum_zero_effective", "background_target", "background_target_same"} {
		t.Run(mode, func(t *testing.T) {
			rows := []TraceCausalProjectionNode{
				{Subject: "worker", Object: "io_latency", ImpactMS: 4, IOValueCaliber: TraceIOValueCaliberRQResidence},
				{Subject: "worker", Object: "io_latency", ImpactMS: 8, IOValueCaliber: TraceIOValueCaliberIssuerBlocked},
			}
			want := TraceIOValueCaliberMixed
			merge := TraceCausalProjectionMergeOccurrenceRows
			switch mode {
			case "max_target", "max_cleared_effective", "max_actual":
				rows[0].QueryWindowStartTs, rows[0].QueryWindowEndTs = 1, 2
				rows[1].QueryWindowStartTs, rows[1].QueryWindowEndTs = 1.2, 2.2
				if mode == "max_target" {
					rows[0].TargetImpactMS = 2
				} else if mode == "max_actual" {
					rows[0].ActualImpactMS = 2
				} else {
					rows[0].EffectiveImpactMS, rows[1].EffectiveImpactMS = 1, 2
					want = TraceIOValueCaliberIssuerBlocked // cleared values supply no ruler
				}
			case "sum_zero_target", "sum_zero_effective":
				rows[1].ImpactMS = 0
				if mode == "sum_zero_target" {
					rows[1].TargetImpactMS = 2
				} else {
					rows[0].EffectiveImpactMS, rows[1].EffectiveImpactMS = 1, 2
				}
			case "background_target", "background_target_same":
				rows[0].Object, rows[1].Object = "unknown-thread", "unknown-thread"
				rows[0].TargetImpactMS = 2
				if mode == "background_target_same" {
					rows[1].TargetImpactMS, want = 3, TraceIOValueCaliberIssuerBlocked
				}
				kept := make([]TraceCausalProjectionNode, traceCausalProjectionUnknownBackgroundKeep)
				for i := range kept {
					kept[i] = TraceCausalProjectionNode{Object: "unknown-thread", ImpactMS: 100}
				}
				rows = append(kept, rows...)
				merge = func(in []TraceCausalProjectionNode) TraceCausalProjectionNode {
					out := traceCausalProjectionFoldUnknownBackground(in)
					if len(out) != len(kept)+1 {
						t.Fatalf("missing background fold: %d", len(out))
					}
					return out[len(out)-1]
				}
			}
			plain := append([]TraceCausalProjectionNode(nil), rows...)
			for i := range plain {
				plain[i].IOValueCaliber = ""
			}
			baseline, got := merge(plain), merge(rows)
			if got.IOValueCaliber != want {
				t.Fatalf("%s ruler = %q, want %q", mode, got.IOValueCaliber, want)
			}
			got.IOValueCaliber = ""
			if !reflect.DeepEqual(got, baseline) {
				t.Fatal("secondary-axis metadata changed an existing field")
			}
		})
	}
}
