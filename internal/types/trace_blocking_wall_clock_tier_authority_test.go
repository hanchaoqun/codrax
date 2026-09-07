package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func blockingTierAuthorityRecord() ObservationRecord {
	r := traceBlockingWallClockAuthorityRecord("rank-target", "binder_wait", 1.409, 13762.835861, 13762.837270,
		TraceNoteKeyTier+"="+TraceCausalTierTargetSelfState, TraceNoteKeyCapacityTruncated+"=true")
	// The real tool publisher uses a non-principal display predicate while
	// retaining the engine-owned target-self-state tier and exact measurement.
	r.Predicate, r.ClaimKey = "root_cause_context_only", "root_cause_context_only"
	return r
}

func TestB1602BlockingTargetSelfTierSurvivesDisplayPredicate(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	r := blockingTierAuthorityRecord()
	before, _ := json.Marshal(r)
	got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{r}}, &rm)
	if len(got) != 1 || got[0].Type != "binder_wait" || got[0].CoverageStatus != "lower_bound_capacity_truncated" || len(got[0].Occurrences) != 1 ||
		got[0].Occurrences[0].StartTs != r.Span.StartTs || got[0].Occurrences[0].EndTs != r.Span.EndTs {
		t.Fatalf("typed target-self measurement lost to display identity: %+v", got)
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("authority mutated the source row")
	}
	legacy := r
	legacy.Predicate, legacy.ClaimKey = "root_cause_target_self_state", "root_cause_target_self_state"
	if old := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{legacy}}, &rm); !reflect.DeepEqual(old, got) {
		t.Fatalf("display identities changed the same tier measurement: old=%+v new=%+v", old, got)
	}
}

func TestB1602BlockingTargetSelfKeepsPreciseAdmission(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*ObservationRecord)
	}{
		{"context tier", func(r *ObservationRecord) { r.RichNotes[2] = TraceNoteKeyTier + "=context_only" }},
		{"unknown tier", func(r *ObservationRecord) { r.RichNotes[2] = TraceNoteKeyTier + "=future_tier" }},
		{"missing tier", func(r *ObservationRecord) { r.RichNotes[2] = "" }},
		{"pacing context", func(r *ObservationRecord) {
			r.Object = "pacing_idle"
			r.RichNotes[0] = "type=pacing_idle"
			r.RichNotes[2] = TraceNoteKeyTier + "=context_only"
		}},
		{"other dimension", func(r *ObservationRecord) { r.Predicate, r.ClaimKey = "ipc_graph", "ipc_graph:sync_request" }},
		{"other target", func(r *ObservationRecord) { r.Subject = "worker-42" }},
		{"missing window", func(r *ObservationRecord) { r.RichNotes[1] = "" }},
		{"missing artifact", func(r *ObservationRecord) { r.SourceRef = ObservationSourceRef{} }},
		{"transaction envelope", func(r *ObservationRecord) { r.Span.StartTs = 13762.835811 }},
		{"zero", func(r *ObservationRecord) { r.Value = "0" }},
		{"nonfinite", func(r *ObservationRecord) { r.Value = "NaN" }},
		{"model origin", func(r *ObservationRecord) { r.Producer = "emit_investigation_complete" }},
		{"soft grounding", func(r *ObservationRecord) { r.GroundingPolicy = ClaimGroundingSoft }},
		{"gap", func(r *ObservationRecord) { r.Object = "missing_wakeup"; r.RichNotes[0] = "type=missing_wakeup" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := blockingTierAuthorityRecord()
			test.edit(&r)
			rm := traceValueOccurrenceAuthorityRequest()
			if got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{r}}, &rm); len(got) != 0 {
				t.Fatalf("unqualified measurement acquired blocking authority: %+v", got)
			}
		})
	}
}

func TestB1602BlockingTargetSelfKeepsQueryScopesSeparate(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	first := blockingTierAuthorityRecord()
	second := blockingTierAuthorityRecord()
	second.ID = "other-query"
	second.RichNotes[1] = "selected_window=13762.800000..13762.900000"
	got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{first, second}}, &rm)
	if len(got) != 2 {
		t.Fatalf("different query windows must retain separate accounts: %+v", got)
	}
	for _, authority := range got {
		if len(authority.Occurrences) != 1 || len(authority.Occurrences[0].RecordIDs) != 1 {
			t.Fatalf("foreign query supplied a member to another scope: %+v", authority)
		}
	}
}
