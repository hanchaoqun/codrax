package types

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func b1638B2BOriginRecord() ObservationRecord {
	offset, slope := 0.25, 1.0
	return ObservationRecord{
		ID: "native:1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Subject: "worker-42", Predicate: "wakeup_causal_impact", Value: "3.500", Unit: "ms",
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact,
			Path: "/captures/A.trace", CaptureIdentityPath: "/captures/A.trace",
			PayloadRef: "/results/A.json", RawRef: "/results/A.txt", QueryScopeID: "query-A",
			ClockOffsetSec: &offset, ClockSlope: &slope},
		ObservedAt: "2026-09-10T00:00:00Z",
		MeasurementSources: TraceSchedulerMeasurementSourcesFromDomain(&TraceSchedulerMeasurementDomain{
			Version: 1, Status: "constructed", Method: "thread_timeline", TargetTID: 42,
			WindowStartTs: 1, WindowEndTs: 2, QueryLineStart: 4, QueryLineEnd: 80, PartitionID: "partition-A",
		}),
	}
}

// Exercise the existing producer-record compiler and node constructor. The
// source must be bound after tool defaults/requalification, not at emission.
func TestB1638B2BFinalObservationBindsMeasurementOrigin(t *testing.T) {
	raw := b1638B2BOriginRecord()
	raw.SourceRef.Kind = ObservationSourceCurrentSource
	raw.SourceRef.RawRef, raw.SourceRef.PayloadRef, raw.ObservedAt = "", "", ""
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{{
		ToolName: "trace_query", Success: true, RawRef: "/results/published.json",
		Timestamp: time.Date(2026, 9, 10, 1, 2, 3, 0, time.UTC), Observations: []ObservationRecord{raw},
	}}})
	var record ObservationRecord
	for _, candidate := range ledger.Records {
		if candidate.ID == raw.ID {
			record = candidate
		}
	}
	if record.ID == "" || record.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		record.SourceRef.RawRef != "/results/published.json" || record.SourceRef.ToolCallID == "" || record.ObservedAt == "" {
		t.Fatalf("invalid final-record premise: %+v", record)
	}
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleCausalHop, record)
	wire, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		Origins []struct {
			SourceRef  ObservationSourceRef              `json:"source_ref"`
			ObservedAt string                            `json:"observed_at"`
			Sources    *TraceSchedulerMeasurementSources `json:"measurement_sources"`
		} `json:"measurement_origins"`
	}
	if err := json.Unmarshal(wire, &published); err != nil {
		t.Fatal(err)
	}
	if len(published.Origins) != 1 {
		t.Fatalf("final observation lost its native-source binding: origins=%d", len(published.Origins))
	}
	origin := published.Origins[0]
	if !reflect.DeepEqual(origin.SourceRef, record.SourceRef) || origin.ObservedAt != record.ObservedAt ||
		!reflect.DeepEqual(origin.Sources, record.MeasurementSources) {
		t.Fatalf("origin did not retain the final source tuple: %+v", origin)
	}
	if node.Subject != record.Subject || node.Value != record.Value || node.Unit != record.Unit {
		t.Fatalf("source propagation changed row content: %+v", node)
	}
}

func TestB1638B2BMeasurementOriginPreservesUnknownAndExactSource(t *testing.T) {
	known := b1638B2BOriginRecord()
	for _, tc := range []struct {
		name string
		edit func(*ObservationRecord)
	}{
		{"known", func(*ObservationRecord) {}},
		{"known_parent_unknown_native", func(r *ObservationRecord) { r.MeasurementSources = nil }},
		{"unknown_parent_known_native", func(r *ObservationRecord) { r.SourceRef = ObservationSourceRef{}; r.ObservedAt = "" }},
		{"fully_unknown", func(r *ObservationRecord) {
			r.SourceRef = ObservationSourceRef{}
			r.ObservedAt = ""
			r.MeasurementSources = nil
		}},
		{"mixed_native_sources", func(r *ObservationRecord) { r.MeasurementSources.HasUnknown = true }},
		{"path_case_is_not_normalized", func(r *ObservationRecord) { r.SourceRef.Path = "/captures/a.trace" }},
		{"clock_and_query_are_not_inferred", func(r *ObservationRecord) {
			r.SourceRef.QueryScopeID = ""
			r.SourceRef.ClockOffsetSec = nil
			r.SourceRef.ClockSlope = nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := known
			r.MeasurementSources = CloneTraceSchedulerMeasurementSources(known.MeasurementSources)
			tc.edit(&r)
			got := TraceSchedulerMeasurementOriginsFromRecord(r)
			want := []TraceSchedulerMeasurementOrigin{{SourceRef: r.SourceRef, ObservedAt: r.ObservedAt, MeasurementSources: r.MeasurementSources}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("source was dropped, normalized or invented: got %+v want %+v", got, want)
			}
		})
	}
}

func TestB1638B2BMeasurementOriginsOwnAllNestedMemory(t *testing.T) {
	record := b1638B2BOriginRecord()
	from := TraceSchedulerMeasurementOriginsFromRecord(record)
	clone := CloneTraceSchedulerMeasurementOrigins(from)
	want, err := json.Marshal(clone)
	if err != nil {
		t.Fatal(err)
	}
	record.SourceRef.Path = "/changed/source"
	*record.SourceRef.ClockOffsetSec, *record.SourceRef.ClockSlope = 90, 91
	record.MeasurementSources.Domains[0].PartitionID = "changed-record"
	from[0].SourceRef.Path = "/changed/origin"
	*from[0].SourceRef.ClockOffsetSec, *from[0].SourceRef.ClockSlope = 92, 93
	from[0].MeasurementSources.Domains[0].PartitionID = "changed-origin"
	from[0].MeasurementSources.HasUnknown = true
	got, err := json.Marshal(clone)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("nested source memory aliased: %s != %s", got, want)
	}
	if CloneTraceSchedulerMeasurementOrigins(nil) != nil {
		t.Fatal("nil origins became present")
	}
	empty := CloneTraceSchedulerMeasurementOrigins([]TraceSchedulerMeasurementOrigin{})
	if empty == nil || len(empty) != 0 {
		t.Fatal("explicit empty slice was not preserved")
	}
}

func TestB1638B2BMeasurementOriginCloneDoesNotMergeContributors(t *testing.T) {
	a := TraceSchedulerMeasurementOriginsFromRecord(b1638B2BOriginRecord())[0]
	unknown := TraceSchedulerMeasurementOriginsFromRecord(ObservationRecord{})[0]
	b := a
	b.SourceRef.Path = "/captures/a.trace"
	c := a
	c.ObservedAt = "2026-09-10T00:00:01Z"
	in := []TraceSchedulerMeasurementOrigin{a, unknown, a, b, c}
	got := CloneTraceSchedulerMeasurementOrigins(in)
	if len(got) != len(in) || !reflect.DeepEqual(got, in) {
		t.Fatalf("clone merged or reordered origins: %+v", got)
	}
	got[0].MeasurementSources.Domains[0].PartitionID = "first-only"
	if got[2].MeasurementSources.Domains[0].PartitionID == "first-only" || in[0].MeasurementSources.Domains[0].PartitionID == "first-only" {
		t.Fatal("duplicate input origins share mutable storage after cloning")
	}
}
