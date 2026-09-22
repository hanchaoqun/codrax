package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Typed legacy/transport controls complement the native public tool tests.
// They do not pretend that edited source receipts are new native measurements.
func TestSelfRunningDisclosureScopeKeepsIndependentRulersAndSnapshot(t *testing.T) {
	offset, slope := 1.25, 1.0
	record := selfrunDiscRecord(nil)
	record.RichNotes = append(record.RichNotes, "selected_window=0.000000..0.020000")
	record.SourceRef = ObservationSourceRef{
		Path: "/capture/a/trace.txt", CaptureIdentityPath: "/capture/a/original.htrace",
		QueryScopeID: "query-a", QueryTargetPID: 100, QueryTargetScope: "thread",
		QueryWindowKnown: true, QueryWindowStartTs: 0, QueryWindowEndTs: .05,
		ClockOffsetSec: &offset, ClockSlope: &slope,
		TraceExcerptScope: &PerfObservationSourceScope{ParentPreviewSHA256: "parent", ByteStart: 10, ByteEnd: 40},
	}
	before, _ := json.Marshal(record)
	d, ok := traceCausalProjectionSelfRunningFoldUnmeasuredFromRecord(record)
	if !ok || d.QuerySourceRef == nil || d.SelectedWindow == nil {
		t.Fatalf("own source/ruler missing: %+v", d)
	}
	if d.SelectedWindow.StartTs != 0 || d.SelectedWindow.EndTs != .02 || d.QuerySourceRef.QueryWindowEndTs != .05 || d.RunningMS != 19.8 || d.UnknownMS != 19.8 {
		t.Fatalf("parent query ruler overwrote the zero-origin selected window or value: %+v", d)
	}
	wire, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var restored TraceCausalProjectionSelfRunningFoldUnmeasured
	if err := json.Unmarshal(wire, &restored); err != nil || !reflect.DeepEqual(d, restored) {
		t.Fatalf("optional scope JSON did not round-trip: %v %+v", err, restored)
	}
	copy, _ := traceCausalProjectionSelfRunningFoldUnmeasuredFromRecord(record)
	if traceCausalProjectionSelfRunningDisclosureKey(d, "a") != traceCausalProjectionSelfRunningDisclosureKey(copy, "b") {
		t.Fatal("pointer allocation identity incorrectly affects display duplicate identity")
	}
	d.QuerySourceRef.Path = "/mutated"
	*d.QuerySourceRef.ClockOffsetSec = 99
	*d.QuerySourceRef.ClockSlope = 2
	d.QuerySourceRef.TraceExcerptScope.ByteStart = 99
	d.SelectedWindow.EndTs = 10
	after, _ := json.Marshal(record)
	if string(before) != string(after) || *copy.QuerySourceRef.ClockOffsetSec != 1.25 || copy.QuerySourceRef.TraceExcerptScope.ByteStart != 10 || copy.SelectedWindow.EndTs != .02 {
		t.Fatal("compiled receipt aliases the ledger or another compilation")
	}
	offset, slope, record.SourceRef.TraceExcerptScope.ByteStart = 3, 4, 50
	if *copy.QuerySourceRef.ClockOffsetSec != 1.25 || *copy.QuerySourceRef.ClockSlope != 1 || copy.QuerySourceRef.TraceExcerptScope.ByteStart != 10 {
		t.Fatal("later source mutation changed the compiled receipt")
	}
}

func TestSelfRunningDisclosureScopeUnknownDoesNotBorrowParent(t *testing.T) {
	for _, raw := range []string{"", "bad", "2..1", "1..1", "-1..2", "NaN..2", "0..+Inf", "-Inf..2"} {
		t.Run(raw, func(t *testing.T) {
			record := selfrunDiscRecord(nil)
			record.RichNotes = append(record.RichNotes, "selected_window="+raw)
			record.SourceRef = ObservationSourceRef{Path: "/capture/trace.txt", QueryWindowKnown: true, QueryWindowStartTs: 5, QueryWindowEndTs: 6, QueryTargetPID: 100}
			d, ok := traceCausalProjectionSelfRunningFoldUnmeasuredFromRecord(record)
			if !ok || d.SelectedWindow != nil || d.QuerySourceRef == nil || d.QuerySourceRef.QueryWindowStartTs != 5 || d.RunningMS != 19.8 || d.UnknownMS != 19.8 {
				t.Fatalf("missing/invalid row range borrowed parent scope or lost the original quantity: %+v", d)
			}
		})
	}
}

func TestSelfRunningDisclosureScopeDuplicateIdentity(t *testing.T) {
	seat := ObservationRecord{
		ID: "trace_query:q#root_cause_primary:1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "root_cause_primary", Subject: "dep-200", Object: "sleep_wait", Value: "12.000", Unit: "ms",
		RichNotes: []string{"rank=1", "tier=primary", "chain_relevance=on_chain", "dominant_state=s_sleep"},
	}
	base := selfrunDiscRecord(nil)
	base.Origin, base.Producer, base.Predicate = AnswerEvidenceOriginRuntimeArtifact, "trace_query", "self_running_fold_unmeasured"
	base.GroundingPolicy = ClaimGroundingHard
	base.SourceRef = ObservationSourceRef{Path: "/capture/a.txt", CaptureIdentityPath: "/capture/a.htrace", QueryScopeID: "q", QueryTargetPID: 100, QueryWindowKnown: true, QueryWindowStartTs: 2, QueryWindowEndTs: 3}
	base.RichNotes = append(base.RichNotes, "selected_window=2..3")
	for _, tc := range []struct {
		name   string
		mutate func(*ObservationRecord)
	}{
		{"different capture", func(r *ObservationRecord) { r.SourceRef.CaptureIdentityPath = "/capture/b.htrace" }},
		{"different carrier", func(r *ObservationRecord) { r.SourceRef.Path = "/capture/b.txt" }},
		{"different query", func(r *ObservationRecord) { r.SourceRef.QueryScopeID = "q2" }},
		{"different target", func(r *ObservationRecord) { r.SourceRef.QueryTargetPID = 200 }},
		{"different parent window", func(r *ObservationRecord) { r.SourceRef.QueryWindowEndTs = 4 }},
		{"different row window", func(r *ObservationRecord) { r.RichNotes[len(r.RichNotes)-1] = "selected_window=2..4" }},
		{"different measured value", func(r *ObservationRecord) {
			r.RichNotes = []string{"selected_window=2..3", "self_running_fold_unmeasured_running_ms=21.500", "self_running_fold_unmeasured_unknown_ms=21.500"}
		}},
		{"unknown source", func(r *ObservationRecord) { r.SourceRef = ObservationSourceRef{} }},
		{"unknown window", func(r *ObservationRecord) { r.RichNotes = r.RichNotes[:len(r.RichNotes)-1] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := base
			other.RichNotes = append([]string(nil), base.RichNotes...)
			tc.mutate(&other)
			for _, records := range [][]ObservationRecord{{base, other, base}, {other, base, other}} {
				p := CompileTraceCausalProjection(ObservationLedger{Records: append([]ObservationRecord{seat}, records...)})
				if len(p.SelfRunningFoldUnmeasured) != 2 {
					t.Fatalf("different scope/measurement merged or exact duplicate retained: %+v", p.SelfRunningFoldUnmeasured)
				}
				if only := CompileTraceCausalProjection(ObservationLedger{Records: records}); only.Active() || len(only.RankedSeats) != 0 || only.PrimaryRootCause != nil {
					t.Fatal("descriptive source provenance alone activated a causal board")
				}
			}
		})
	}
	unknown := base
	unknown.ID = "legacy-a"
	unknown.SourceRef, unknown.RichNotes = ObservationSourceRef{}, []string{"self_running_fold_unmeasured_running_ms=21.500", "self_running_fold_unmeasured_unknown_ms=21.500"}
	otherUnknown := unknown
	otherUnknown.RichNotes = []string{"self_running_fold_unmeasured_running_ms=22.500", "self_running_fold_unmeasured_unknown_ms=22.500"}
	sameValueDifferentRecord := unknown
	sameValueDifferentRecord.ID = "legacy-b"
	p := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{seat, unknown, otherUnknown, sameValueDifferentRecord, unknown}})
	if len(p.SelfRunningFoldUnmeasured) != 3 {
		t.Fatalf("empty source keys merged distinct original values: %+v", p.SelfRunningFoldUnmeasured)
	}
}
