package types

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestSchedulerRestrictionRequiresOwnedNativeReceipt(t *testing.T) {
	base := schedulerRestrictionTestNode("base", 0, 0, 1).MeasurementOrigins
	want, ok := TraceSchedulerMeasurementRestrictionKey(base)
	if !ok || want == "" {
		t.Fatal("valid unrestricted native receipt is not legacy")
	}
	for _, mutate := range []struct {
		name string
		fn   func(*TraceSchedulerMeasurementOrigin)
	}{
		{"missing_path", func(o *TraceSchedulerMeasurementOrigin) { o.SourceRef.Path = "" }},
		{"missing_kind", func(o *TraceSchedulerMeasurementOrigin) { o.SourceRef.Kind = "" }},
		{"missing_query", func(o *TraceSchedulerMeasurementOrigin) { o.SourceRef.QueryScopeID = "" }},
		{"missing_result", func(o *TraceSchedulerMeasurementOrigin) { o.SourceRef.PayloadRef, o.SourceRef.RawRef = "", "" }},
		{"missing_observed", func(o *TraceSchedulerMeasurementOrigin) { o.ObservedAt = "" }},
		{"future_version", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.Domains[0].Version = 2 }},
		{"future_status", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.Domains[0].Status = "complete" }},
		{"unknown_method", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.Domains[0].Method = "other" }},
		{"missing_tid", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.Domains[0].TargetTID = 0 }},
		{"missing_partition", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.Domains[0].PartitionID = "" }},
		{"nan_window", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.Domains[0].WindowStartTs = math.NaN() }},
		{"infinite_window", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.Domains[0].WindowEndTs = math.Inf(1) }},
		{"empty_window", func(o *TraceSchedulerMeasurementOrigin) {
			o.MeasurementSources.Domains[0].WindowEndTs = o.MeasurementSources.Domains[0].WindowStartTs
		}},
		{"negative_line", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.Domains[0].QueryLineStart = -1 }},
		{"reversed_line", func(o *TraceSchedulerMeasurementOrigin) {
			o.MeasurementSources.Domains[0].QueryLineStart, o.MeasurementSources.Domains[0].QueryLineEnd = 9, 8
		}},
		{"unknown_member", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.HasUnknown = true }},
		{"empty_native_set", func(o *TraceSchedulerMeasurementOrigin) { o.MeasurementSources.Domains = nil }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			copy := CloneTraceSchedulerMeasurementOrigins(base)
			mutate.fn(&copy[0])
			if _, grouped := TraceSchedulerMeasurementRestrictionKey(copy); grouped {
				t.Fatal("uncertain source became groupable")
			}
		})
	}
	// These dimensions are deliberately NOT an equality or grouping proof.
	for _, method := range []string{"thread_timeline", "off_cpu_sweep", "cpu_running_sweep", "state_churn_sweep"} {
		copy := CloneTraceSchedulerMeasurementOrigins(base)
		copy[0].MeasurementSources.Domains[0].Method = method
		copy[0].MeasurementSources.Domains[0].WindowStartTs, copy[0].MeasurementSources.Domains[0].WindowEndTs = 100, 200
		copy[0].MeasurementSources.Domains[0].PartitionID = "different-native-value"
		copy[0].SourceRef.QueryScopeID = "another-view-result"
		if key, grouped := TraceSchedulerMeasurementRestrictionKey(copy); !grouped || key != want {
			t.Fatal("local measurement identity incorrectly split shared original event restriction")
		}
	}
	for _, lines := range [][2]int{{0, 80}, {4, 0}, {4, 80}} {
		copy := schedulerRestrictionTestNode("bounded", lines[0], lines[1], 1).MeasurementOrigins
		if key, grouped := TraceSchedulerMeasurementRestrictionKey(copy); !grouped || key == want {
			t.Fatal("one-sided or bounded line restriction lost")
		}
	}
}

func schedulerRestrictionTestNode(id string, start, end int, ms float64) TraceCausalProjectionNode {
	r := b1638B2BOriginRecord()
	r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef = "parent-"+id, "/result/"+id+".json"
	d := &r.MeasurementSources.Domains[0]
	d.Status, d.QueryLineStart, d.QueryLineEnd = "constructed_partition", start, end
	d.WindowStartTs, d.WindowEndTs, d.PartitionID = 1+ms/1000, 2+ms/1000, "local-"+id
	return TraceCausalProjectionNode{
		EvidenceID: id, Subject: "worker-42", Object: "sleep", StateKind: "sleep",
		Predicate: "wakeup_causal_impact", Role: TraceCausalRoleCausalHop,
		ImpactMS: ms, CumulativeImpactMS: ms, QueryWindowStartTs: 1, QueryWindowEndTs: 3,
		MeasurementOrigins: TraceSchedulerMeasurementOriginsFromRecord(r),
	}
}

// Equal display windows cannot authorize summing different event selections.
// Within each selection, distinct local timelines and parent query receipts
// remain eligible for the old aggregation rules (cross-view supplementation).
func TestSchedulerRestrictionR2KeepsDifferentEventSelectionsSeparate(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		nodes := []TraceCausalProjectionNode{}
		for i := 0; i < 3; i++ {
			nodes = append(nodes, schedulerRestrictionTestNode(fmt.Sprint("main", i), 0, 0, float64(i+1)))
			nodes = append(nodes, schedulerRestrictionTestNode(fmt.Sprint("filtered", i), 1, 400, float64(10*(i+1))))
		}
		if reverse {
			for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
		got := traceCausalProjectionAggregateSameKind(nodes)
		if len(got) != 2 {
			t.Fatalf("different event selections merged: reverse=%t rows=%d, want two independent groups", reverse, len(got))
		}
		seen := map[float64]bool{}
		for _, row := range got {
			seen[row.ImpactMS] = true
			if row.MergedCount != 3 {
				t.Fatalf("local timelines were split or foreign members admitted: %+v", row)
			}
		}
		if !seen[6] || !seen[60] {
			t.Fatalf("separate values changed: %v", seen)
		}
	}
}

func TestSchedulerRestrictionUncertainMemberCannotBridgeSelections(t *testing.T) {
	for _, kind := range []string{"legacy", "unknown_native", "missing_parent", "two_filters"} {
		nodes := []TraceCausalProjectionNode{}
		for i := 0; i < 3; i++ {
			nodes = append(nodes, schedulerRestrictionTestNode(fmt.Sprint(i), 0, 0, 1))
		}
		extra := schedulerRestrictionTestNode("uncertain", 0, 0, 40)
		switch kind {
		case "legacy":
			extra.MeasurementOrigins = nil
		case "unknown_native":
			extra.MeasurementOrigins[0].MeasurementSources.HasUnknown = true
		case "missing_parent":
			extra.MeasurementOrigins[0].SourceRef = ObservationSourceRef{}
		case "two_filters":
			extra.MeasurementOrigins = append(extra.MeasurementOrigins, schedulerRestrictionTestNode("other", 1, 400, 2).MeasurementOrigins...)
		}
		nodes = append(nodes, extra)
		got := traceCausalProjectionAggregateSameKind(nodes)
		if len(got) != 2 {
			t.Fatalf("%s was borrowed into known group: %d rows", kind, len(got))
		}
		if got[0].ImpactMS != 3 || got[1].ImpactMS != 40 {
			t.Fatalf("%s lost independent facts: %v / %v", kind, got[0].ImpactMS, got[1].ImpactMS)
		}
	}
}

func TestSchedulerRestrictionDoesNotPartitionByLocalTimelineOrParentResult(t *testing.T) {
	nodes := []TraceCausalProjectionNode{}
	for i := 0; i < 8; i++ {
		nodes = append(nodes, schedulerRestrictionTestNode(fmt.Sprint(i), 1, 400, float64(i+1)))
	}
	got := traceCausalProjectionAggregateSameKind(nodes)
	if len(got) != 1 || got[0].ImpactMS != 36 || got[0].MergedCount != 8 {
		t.Fatalf("legal occurrence family fragmented: %+v", got)
	}
	for i := range nodes {
		nodes[i].MeasurementOrigins = nil
	}
	legacy := traceCausalProjectionAggregateSameKind(nodes)
	for i := range got {
		got[i].MeasurementOrigins = nil
	}
	for i := range legacy {
		legacy[i].MeasurementOrigins = nil
	}
	if !reflect.DeepEqual(got, legacy) {
		t.Fatal("native provenance changed another old field in a homogeneous group")
	}
}
