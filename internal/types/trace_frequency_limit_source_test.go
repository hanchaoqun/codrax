package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func frequencySourceTestWitness() TraceFrequencyLimitAuthority {
	return TraceFrequencyLimitAuthority{
		CPU: 4, MinFrequencyKHz: 558000, MaxFrequencyKHz: 2100000, LimitRowCount: 2,
		WitnessLine: 3, WitnessTs: 1.2, WindowStartTs: 1, WindowEndTs: 2,
		Authority: "direct_in_window_policy_limit", ObservedAt: "2026-09-09T03:00:00Z",
		SourceRef: &ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/capture/a/same.systrace",
			PayloadRef: "/session/a/result.json", RawRef: "/session/a/result.txt", QueryScopeID: "window/0/lines1..4"},
	}
}

func TestB1631FrequencySourceMatchingRequiresSameResultBeforeWindow(t *testing.T) {
	w := frequencySourceTestWitness()
	r := TraceFrequencyLimitSourceRecord(w)
	r.Subject, r.Predicate = "UI-7", "running_time"
	if !TraceFrequencyLimitMatchesRecord(w, r) || TraceFrequencyLimitSourceKey(w) == "" {
		t.Fatal("fully witnessed same-result policy must pair with target")
	}
	for _, tc := range []struct {
		name string
		edit func(*ObservationRecord)
		want bool
	}{
		{"same_basename_other_capture", func(r *ObservationRecord) { r.SourceRef.Path = "/capture/b/same.systrace" }, false},
		{"case_sensitive_paths", func(r *ObservationRecord) { r.SourceRef.Path = "/capture/A/same.systrace" }, false},
		{"same_basename_other_payload", func(r *ObservationRecord) { r.SourceRef.PayloadRef = "/session/b/result.json" }, false},
		{"different_raw", func(r *ObservationRecord) { r.SourceRef.RawRef = "/session/b/result.txt" }, false},
		{"different_filters", func(r *ObservationRecord) { r.SourceRef.QueryScopeID = "window/0/lines5..8" }, false},
		{"different_child", func(r *ObservationRecord) { r.SourceRef.QueryScopeID = "window/1/lines1..4" }, false},
		{"missing_scope", func(r *ObservationRecord) { r.SourceRef.QueryScopeID = "" }, false},
		{"different_execution", func(r *ObservationRecord) { r.ObservedAt = "2026-09-09T04:00:00Z" }, false},
		{"missing_execution", func(r *ObservationRecord) { r.ObservedAt = "" }, false},
		{"model_origin", func(r *ObservationRecord) { r.Origin = AnswerEvidenceOriginSystemInference }, false},
		{"model_producer", func(r *ObservationRecord) { r.Producer = "explorer" }, false},
		{"suffixed_query_producer", func(r *ObservationRecord) { r.Producer = "trace_query:run2" }, true},
		{"missing_query_window", func(r *ObservationRecord) { r.RichNotes = nil; r.Span = ObservationSpan{StartTs: 1, EndTs: 2} }, false},
		{"different_query_window", func(r *ObservationRecord) { r.RichNotes = []string{"selected_window=1.1..2.1"} }, false},
		{"ledger_enrichment", func(r *ObservationRecord) { r.SourceRef.CaptureIdentityPath = "/original/same.systrace" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := TraceFrequencyLimitSourceRecord(w)
			candidate.Subject = r.Subject
			tc.edit(&candidate)
			if got := TraceFrequencyLimitMatchesRecord(w, candidate); got != tc.want {
				t.Fatalf("match=%v want %v: %+v", got, tc.want, candidate)
			}
		})
	}
	w.SourceRef.CaptureIdentityPath = "/original/a.systrace"
	r.SourceRef.CaptureIdentityPath = "/original/b.systrace"
	if TraceFrequencyLimitMatchesRecord(w, r) {
		t.Fatal("conflicting canonical captures matched")
	}
}

func TestB1631FrequencySourceUnknownAndConflictsNeverDedup(t *testing.T) {
	w := frequencySourceTestWitness()
	other := CloneTraceFrequencyLimitAuthority(w)
	other.SourceRef.Path = "/capture/b/same.systrace"
	conflict := CloneTraceFrequencyLimitAuthority(w)
	conflict.MaxFrequencyKHz = 1800000
	legacy := w
	legacy.SourceRef, legacy.ObservedAt = nil, ""
	in := []TraceFrequencyLimitAuthority{w, w, other, conflict, legacy, legacy}
	before, _ := json.Marshal(in)
	got := DedupTraceFrequencyLimitAuthorities(in, 0)
	if len(got) != 5 || !reflect.DeepEqual(got, []TraceFrequencyLimitAuthority{w, other, conflict, legacy, legacy}) {
		t.Fatalf("wrong identity/value dedup or order: %+v", got)
	}
	if len(DedupTraceFrequencyLimitAuthorities(in, 2)) != 2 {
		t.Fatal("caller display budget changed")
	}
	for _, edit := range []func(*TraceFrequencyLimitAuthority){
		func(w *TraceFrequencyLimitAuthority) { w.SourceRef = nil },
		func(w *TraceFrequencyLimitAuthority) { w.ObservedAt = "" },
		func(w *TraceFrequencyLimitAuthority) { w.SourceRef.QueryScopeID = "" },
		func(w *TraceFrequencyLimitAuthority) { w.SourceRef.PayloadRef, w.SourceRef.RawRef = "", "" },
		func(w *TraceFrequencyLimitAuthority) { w.SourceRef.Path = "" },
		func(w *TraceFrequencyLimitAuthority) { w.SourceRef.Path = "trace_query" },
		func(w *TraceFrequencyLimitAuthority) { w.WindowStartTs, w.WindowEndTs = 0, 0 },
	} {
		unknown := CloneTraceFrequencyLimitAuthority(w)
		edit(&unknown)
		if TraceFrequencyLimitSourceKey(unknown) != "" || TraceFrequencyLimitMatchesRecord(unknown, TraceFrequencyLimitSourceRecord(unknown)) {
			t.Fatalf("missing source acquired result authority: %+v", unknown)
		}
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("source matching/dedup mutated input")
	}
}

func TestB1631FrequencySourceDescriptorsAndDedupDetachReceipts(t *testing.T) {
	w := frequencySourceTestWitness()
	offset, slope := 0.1, 1.0
	w.SourceRef.ClockOffsetSec, w.SourceRef.ClockSlope = &offset, &slope
	r := TraceFrequencyLimitSourceRecord(w)
	if r.Subject != "" || r.ID != "" || r.Predicate != "" {
		t.Fatal("source descriptor fabricated a target/evidence")
	}
	r.SourceRef.Path = "changed"
	*r.SourceRef.ClockOffsetSec, *r.SourceRef.ClockSlope = 3, 4
	out := DedupTraceFrequencyLimitAuthorities([]TraceFrequencyLimitAuthority{w}, 8)
	out[0].SourceRef.QueryScopeID = "changed"
	*out[0].SourceRef.ClockOffsetSec, *out[0].SourceRef.ClockSlope = 5, 6
	if w.SourceRef.Path != "/capture/a/same.systrace" || w.SourceRef.QueryScopeID != "window/0/lines1..4" || offset != .1 || slope != 1 {
		t.Fatal("mutable source receipt escaped defensive clone")
	}
}

func TestB1631RuntimeAccountSameResultKeepsLogicalChildrenSeparate(t *testing.T) {
	w := frequencySourceTestWitness()
	a, b := TraceFrequencyLimitSourceRecord(w), TraceFrequencyLimitSourceRecord(w)
	if !TraceRuntimeAccountRecordsSameResult(a, b) {
		t.Fatal("same complete source rejected")
	}
	b.SourceRef.QueryScopeID += "/other-child"
	if TraceRuntimeAccountRecordsSameResult(a, b) {
		t.Fatal("shared parent payload authorized a different query child")
	}
	a.SourceRef.QueryScopeID, b.SourceRef.QueryScopeID = "", ""
	if !TraceRuntimeAccountRecordsSameResult(a, b) {
		t.Fatal("legacy same-result account behavior changed")
	}
}
