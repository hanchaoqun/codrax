package types

import (
	"fmt"
	"reflect"
	"testing"
)

func b1638B3OriginRow(i int) TraceCausalProjectionNode {
	r := b1638B2BOriginRecord()
	r.ID = fmt.Sprintf("native:%d", i)
	r.SourceRef.PayloadRef = fmt.Sprintf("/results/%d.json", i)
	r.SourceRef.QueryScopeID = fmt.Sprintf("query-%d", i)
	r.MeasurementSources.Domains[0].PartitionID = fmt.Sprintf("partition-%d", i)
	r.MeasurementSources.Domains[0].Status = "constructed_partition"
	return TraceCausalProjectionNode{
		EvidenceID: r.ID, Role: TraceCausalRoleCausalHop,
		Subject: "worker-42", Predicate: "wakeup_causal_impact", Object: "s_sleep",
		Value: "2.000", Unit: "ms", ImpactMS: 2, CumulativeImpactMS: 2,
		SupportRefs: []string{fmt.Sprintf("/captures/A.trace:%d", i*2+1)}, LineStart: i*2 + 1, LineEnd: i*2 + 2,
		StartTs: float64(i+1) / 100, EndTs: float64(i+1)/100 + .002,
		QueryWindowStartTs: .001, QueryWindowEndTs: 1,
		MeasurementOrigins: TraceSchedulerMeasurementOriginsFromRecord(r),
	}
}

// The source-only change must preserve every previous result field, including
// existing (not necessarily additive) numeric calibers. Origins are not a key.
func TestB1638B3R2RetainsActualMemberOriginsWithoutChangingOtherFields(t *testing.T) {
	for _, mode := range []string{"public_pair", "regular_three", "background_three", "selected_members", "overlap_union", "cross_window_max"} {
		t.Run(mode, func(t *testing.T) {
			rows := []TraceCausalProjectionNode{b1638B3OriginRow(0), b1638B3OriginRow(1), b1638B3OriginRow(2)}
			members := []int{0, 1, 2}
			merge := func(in []TraceCausalProjectionNode) TraceCausalProjectionNode {
				return traceCausalProjectionMergeSameKindMembers(in, 0, members)
			}
			switch mode {
			case "public_pair":
				rows, members = rows[:2], members[:2]
				merge = TraceCausalProjectionMergeOccurrenceRows
			case "regular_three", "background_three":
				merge = func(in []TraceCausalProjectionNode) TraceCausalProjectionNode {
					out := traceCausalProjectionAggregateSameKindLane(in, mode == "background_three")
					if len(out) != 1 {
						t.Fatalf("R2 fixture did not reach a fold: %d rows", len(out))
					}
					return out[0]
				}
			case "selected_members":
				members = []int{2, 0}
				merge = func(in []TraceCausalProjectionNode) TraceCausalProjectionNode {
					return traceCausalProjectionMergeSameKindMembers(in, 2, members)
				}
			case "overlap_union":
				rows[1].QueryWindowStartTs = .002
				rows[1].StartTs, rows[1].EndTs = rows[0].StartTs+.001, rows[0].EndTs+.001
			case "cross_window_max":
				rows[1].QueryWindowStartTs = .002
				for i := range rows {
					rows[i].StartTs, rows[i].EndTs = 0, 0
				}
			}
			without := append([]TraceCausalProjectionNode(nil), rows...)
			for i := range without {
				without[i].MeasurementOrigins = nil
			}
			wantFields := merge(without)
			got := merge(rows)
			var want []TraceSchedulerMeasurementOrigin
			for _, i := range members {
				want = append(want, rows[i].MeasurementOrigins...)
			}
			if !reflect.DeepEqual(got.MeasurementOrigins, want) {
				t.Errorf("source loss or seed double-count: got %d origins, want %d exact ordered member origins", len(got.MeasurementOrigins), len(want))
			}
			got.MeasurementOrigins, wantFields.MeasurementOrigins = nil, nil
			if !reflect.DeepEqual(got, wantFields) {
				t.Fatal("source metadata changed a pre-existing result field")
			}
		})
	}
}

func TestB1638B3OccurrenceOriginsKeepUnknownDuplicatesAndOwnMemory(t *testing.T) {
	a := b1638B3OriginRow(0).MeasurementOrigins
	b := b1638B3OriginRow(1).MeasurementOrigins
	b[0].MeasurementSources.HasUnknown = true
	for _, tc := range []struct {
		name   string
		inputs [][]TraceSchedulerMeasurementOrigin
		want   []TraceSchedulerMeasurementOrigin
	}{
		{"known_then_legacy", [][]TraceSchedulerMeasurementOrigin{a, nil}, []TraceSchedulerMeasurementOrigin{a[0], {}}},
		{"legacy_then_known", [][]TraceSchedulerMeasurementOrigin{nil, a}, []TraceSchedulerMeasurementOrigin{{}, a[0]}},
		{"all_legacy", [][]TraceSchedulerMeasurementOrigin{nil, {}}, []TraceSchedulerMeasurementOrigin{{}, {}}},
		{"duplicates_and_existing_unknown", [][]TraceSchedulerMeasurementOrigin{append(CloneTraceSchedulerMeasurementOrigins(a), b...), a}, []TraceSchedulerMeasurementOrigin{a[0], b[0], a[0]}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := make([]TraceCausalProjectionNode, len(tc.inputs))
			for i, origins := range tc.inputs {
				rows[i] = b1638B3OriginRow(i)
				rows[i].MeasurementOrigins = origins
			}
			got := TraceCausalProjectionMergeOccurrenceRows(rows)
			if !reflect.DeepEqual(got.MeasurementOrigins, tc.want) {
				t.Fatalf("unknown/duplicate contributor erased or invented: got %+v want %+v", got.MeasurementOrigins, tc.want)
			}
			before := CloneTraceSchedulerMeasurementOrigins(tc.want)
			for i := range got.MeasurementOrigins {
				o := &got.MeasurementOrigins[i]
				if o.MeasurementSources == nil {
					continue
				}
				o.MeasurementSources.Domains[0].PartitionID = "changed"
				*o.SourceRef.ClockOffsetSec, *o.SourceRef.ClockSlope = 9, 10
				for j := i + 1; j < len(got.MeasurementOrigins); j++ {
					if !reflect.DeepEqual(got.MeasurementOrigins[j], before[j]) {
						t.Fatal("output origins share nested storage")
					}
				}
			}
			if !reflect.DeepEqual(tc.want, before) {
				t.Fatal("merged origins alias input memory")
			}
		})
	}
}

func TestB1638B3OccurrenceSingletonOwnsSourceAndEmptyDoesNotInventOne(t *testing.T) {
	if got := TraceCausalProjectionMergeOccurrenceRows(nil); got.MeasurementOrigins != nil {
		t.Fatal("zero members invented an unknown contributor")
	}
	row := b1638B3OriginRow(0)
	want := CloneTraceSchedulerMeasurementOrigins(row.MeasurementOrigins)
	got := TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{row})
	got.MeasurementOrigins[0].MeasurementSources.Domains[0].PartitionID = "changed"
	*got.MeasurementOrigins[0].SourceRef.ClockSlope = 4
	if !reflect.DeepEqual(row.MeasurementOrigins, want) {
		t.Fatal("singleton result aliases source memory")
	}
	row.MeasurementOrigins = nil
	if got := TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{row}); !reflect.DeepEqual(got, row) {
		t.Fatal("unmerged legacy row changed")
	}
}

func TestB1638B3ConcatOriginsHasNoMemberCapOrSyntheticSeed(t *testing.T) {
	if got := ConcatTraceSchedulerMeasurementOrigins(); got != nil {
		t.Fatal("no contributors invented an unknown source")
	}
	members := make([][]TraceSchedulerMeasurementOrigin, 48)
	var want []TraceSchedulerMeasurementOrigin
	for i := range members {
		members[i] = b1638B3OriginRow(i).MeasurementOrigins
		want = append(want, members[i]...)
	}
	got := ConcatTraceSchedulerMeasurementOrigins(members...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("concat dropped or duplicated a contributor: got %d want %d", len(got), len(want))
	}
	// A legacy input denotes an actual unknown member, not an empty collection
	// of members. This may add source metadata to old JSON, never a value.
	if got := ConcatTraceSchedulerMeasurementOrigins(nil, []TraceSchedulerMeasurementOrigin{}); !reflect.DeepEqual(got, []TraceSchedulerMeasurementOrigin{{}, {}}) {
		t.Fatal("legacy contributors were erased")
	}
}
