package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func eventParticipantFixture240(t *testing.T) (ObservationRecord, ObservationRecord, *RequestModel) {
	t.Helper()
	r := testTraceEventSearchInventoryRecord()
	r.SourceRef.QueryWindowKnown, r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs = true, 1, 2
	r.SourceRef.ToolCallID, r.SourceRef.ArtifactID, r.SourceRef.CaptureIdentityPath = "call-a", "capture-a", "/capture.trace"
	known := true
	scan := &TraceEventSearchScanScope{TimeStart: 1, TimeEnd: 2, TimeStartApplied: true, TimeEndApplied: true, ObservedBasis: "parsed_events", ObservedCount: 1}
	r.EventSearchInventory.Query = TraceEventSearchInventoryQuery{View: "event_search", TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}
	r.EventSearchInventory.Coverage.ScanScope = scan
	r.EventSearchInventory.Rows = []TraceEventSearchInventoryRow{{Line: 2, TraceTimeSeconds: 1.5, EventType: "sched_wakeup", Comm: "record", EmitterTID: 11, EmitterTIDKnown: &known, EmitterTGID: 10, EmitterTGIDKnown: &known}}
	f := RuntimeWakeupEvent{IndexPath: r.SourceRef.Path, From: "waker", To: "wakee", Waker: RuntimeWakeupThread{PID: 11, Comm: "record"}, Wakee: RuntimeWakeupThread{PID: 42, Comm: "target"}, Timestamp: 1.5, Line: 2, ScanScope: CloneTraceEventSearchScanScope(scan)}
	w := ObservationRecord{ID: "wakeup-one", Producer: "trace_query", Origin: AnswerEvidenceOriginRuntimeArtifact, Role: AnswerAggregateRoleSupportingCoverage, GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane: ObservationProvenanceArtifactSpan, Predicate: "scheduler_wakeup_event", Unit: "event", Value: "1", SourceRef: r.SourceRef, SupportRefs: []string{"/capture.trace:2"}}
	setEventParticipantWitness240(&w, f)
	if _, ok := DecodeRuntimeWakeupEvent(w); !ok || !IsValidTraceEventSearchInventoryRecord(r) {
		t.Fatal("invalid typed fixture")
	}
	start, end := 1.0, 2.0
	rm := &RequestModel{RuntimeTargets: []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 42, Thread: "target-42", Source: "user_explicit"}},
		RuntimeQuestionProfile:      &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet, FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactOccurrenceTime}},
		RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}
	return r, w, rm
}

func setEventParticipantWitness240(r *ObservationRecord, f RuntimeWakeupEvent) {
	r.Subject, r.Object = RuntimeWakeupThreadLabel(f.Waker), RuntimeWakeupThreadLabel(f.Wakee)
	r.ClaimKey = r.Predicate + ":" + r.Subject + "->" + r.Object
	r.Span = ObservationSpan{LineStart: f.Line, LineEnd: f.Line, StartTs: f.Timestamp, EndTs: f.Timestamp}
	raw, _ := json.Marshal(f)
	r.RichNotes = []string{TraceNoteKeyWakeupEventInstance + "=" + string(raw)}
}

func TestEventParticipantExactWitnessBoundariesHMC240(t *testing.T) {
	r, w, rm := eventParticipantFixture240(t)
	row := r.EventSearchInventory.Rows[0]
	if !TraceEventRowMatchesDisplayTarget(r, row, rm, NewTraceEventParticipantDisplayRoster([]ObservationRecord{w})) || TraceEventRowMatchesEmitterTarget(row, rm) {
		t.Fatal("row-local participant display did not remain separate from emitter ownership")
	}
	for name, change := range map[string]func(*ObservationRecord, *RuntimeWakeupEvent){
		"other_path": func(w *ObservationRecord, f *RuntimeWakeupEvent) {
			w.SourceRef.Path = "/other.trace"
			f.IndexPath = w.SourceRef.Path
		},
		"other_query":    func(w *ObservationRecord, _ *RuntimeWakeupEvent) { w.SourceRef.QueryScopeID = "query-b" },
		"other_payload":  func(w *ObservationRecord, _ *RuntimeWakeupEvent) { w.SourceRef.PayloadRef = "payload-b" },
		"other_call":     func(w *ObservationRecord, _ *RuntimeWakeupEvent) { w.SourceRef.ToolCallID = "call-b" },
		"other_artifact": func(w *ObservationRecord, _ *RuntimeWakeupEvent) { w.SourceRef.ArtifactID = "capture-b" },
		"other_capture": func(w *ObservationRecord, _ *RuntimeWakeupEvent) {
			w.SourceRef.CaptureIdentityPath = "/capture-b.trace"
		},
		"other_window":    func(w *ObservationRecord, _ *RuntimeWakeupEvent) { w.SourceRef.QueryWindowEndTs = 3 },
		"other_line":      func(_ *ObservationRecord, f *RuntimeWakeupEvent) { f.Line = 3 },
		"other_time":      func(_ *ObservationRecord, f *RuntimeWakeupEvent) { f.Timestamp = 1.6 },
		"other_scan":      func(_ *ObservationRecord, f *RuntimeWakeupEvent) { f.ScanScope.ObservedCount++ },
		"other_emitter":   func(_ *ObservationRecord, f *RuntimeWakeupEvent) { f.Waker.PID = 12 },
		"other_wakee":     func(_ *ObservationRecord, f *RuntimeWakeupEvent) { f.Wakee.PID = 43 },
		"unknown_wakee":   func(_ *ObservationRecord, f *RuntimeWakeupEvent) { f.Wakee.PID = 0 },
		"overflow_wakee":  func(_ *ObservationRecord, f *RuntimeWakeupEvent) { f.Wakee.PID = RuntimeTargetMaxPID + 1 },
		"model_origin":    func(w *ObservationRecord, _ *RuntimeWakeupEvent) { w.Origin = AnswerEvidenceOriginSystemInference },
		"missing_support": func(w *ObservationRecord, _ *RuntimeWakeupEvent) { w.SupportRefs = nil },
		"wrong_role":      func(w *ObservationRecord, _ *RuntimeWakeupEvent) { w.Role = AnswerAggregateRolePrincipalAnswer },
		"chain_is_not_row_receipt": func(w *ObservationRecord, _ *RuntimeWakeupEvent) {
			w.Predicate = "wakeup_chain_edge"
			w.ProvenanceLane = ObservationProvenanceObservedDirectCause
			w.Unit = "ms"
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := w
			f, _ := DecodeRuntimeWakeupEvent(w)
			change(&bad, &f)
			setEventParticipantWitness240(&bad, f)
			if TraceEventRowMatchesDisplayTarget(r, row, rm, NewTraceEventParticipantDisplayRoster([]ObservationRecord{bad})) {
				t.Fatal("non-matching or unproven participant admitted")
			}
		})
	}
	for name, change := range map[string]func(*TraceEventSearchInventoryRow){
		"unknown_emitter": func(r *TraceEventSearchInventoryRow) { unknown := false; r.EmitterTIDKnown = &unknown },
		"legacy_emitter":  func(r *TraceEventSearchInventoryRow) { r.EmitterTIDKnown = nil },
		"different_kind":  func(r *TraceEventSearchInventoryRow) { r.EventType = "sched_switch" },
		"changed_member":  func(r *TraceEventSearchInventoryRow) { r.Raw = "changed member" },
	} {
		t.Run(name, func(t *testing.T) {
			other := row
			change(&other)
			if TraceEventRowMatchesDisplayTarget(r, other, rm, NewTraceEventParticipantDisplayRoster([]ObservationRecord{w})) {
				t.Fatal("changed member admitted")
			}
		})
	}
}

func TestEventParticipantConflictUnknownAndProcessHMC240(t *testing.T) {
	r, w, rm := eventParticipantFixture240(t)
	row := r.EventSearchInventory.Rows[0]
	conflict := w
	f, _ := DecodeRuntimeWakeupEvent(w)
	f.Wakee.PID = 43
	setEventParticipantWitness240(&conflict, f)
	for _, records := range [][]ObservationRecord{nil, {w, conflict}, {conflict, w}} {
		if TraceEventRowMatchesDisplayTarget(r, row, rm, NewTraceEventParticipantDisplayRoster(records)) {
			t.Fatal("ambiguous/absent receipt admitted")
		}
	}
	roster := NewTraceEventParticipantDisplayRoster([]ObservationRecord{w, w})
	if !TraceEventRowMatchesDisplayTarget(r, row, rm, roster) {
		t.Fatal("identical republication lost participant")
	}
	rm.RuntimeTargets[0].Kind = RuntimeTargetKindProcess
	if TraceEventRowMatchesDisplayTarget(r, row, rm, roster) {
		t.Fatal("wakee TID became process membership")
	}
	rm.RuntimeTargets[0].PID = 10
	if !TraceEventRowMatchesDisplayTarget(r, row, rm, roster) {
		t.Fatal("independently known emitter TGID lost")
	}
	rm.RuntimeTargets[0] = RuntimeTarget{Kind: RuntimeTargetKindThread, Thread: "target", Source: "user_explicit"}
	if !TraceEventRowMatchesDisplayTarget(r, row, rm, roster) {
		t.Fatal("exact recorded participant name lost")
	}
	rm.RuntimeTargets[0].PID = 99
	if TraceEventRowMatchesDisplayTarget(r, row, rm, roster) {
		t.Fatal("name overrode explicit wrong TID")
	}
	raw, _ := json.Marshal(roster)
	var restored TraceEventParticipantDisplayRoster
	if string(raw) != "{}" || json.Unmarshal(raw, &restored) != nil {
		t.Fatal("private witness serialized")
	}
	rm.RuntimeTargets[0].PID = 42
	if TraceEventRowMatchesDisplayTarget(r, row, rm, restored) {
		t.Fatal("JSON restored participant authority")
	}
	for _, key := range []string{"scheduler.wakee.tid", "scheduler.wakee.comm"} {
		bad := r
		bad.EventSearchInventory = CloneTraceEventSearchInventory(r.EventSearchInventory)
		value, kind := "43", "int64"
		if key == "scheduler.wakee.comm" {
			value, kind = "different", "text"
		}
		bad.EventSearchInventory.Rows[0].Semantics = &TraceEventSemantics{SchemaVersion: 1, Fields: []TraceEventSemanticField{{Key: key, Type: kind, Status: "known", Value: &value}}}
		if !IsValidTraceEventSearchInventoryRecord(bad) {
			t.Fatal("conflicting typed field fixture invalid")
		}
		if TraceEventRowMatchesDisplayTarget(bad, bad.EventSearchInventory.Rows[0], rm, roster) {
			t.Fatal("contradictory native field displayed under another participant")
		}
	}
}

func TestEventParticipantProjectionCloneAndCoalesceHMC240(t *testing.T) {
	r, w, rm := eventParticipantFixture240(t)
	c := runtimeEventContractHMC236(t, r)
	original := c.Tables[0]
	roster := NewTraceEventParticipantDisplayRoster([]ObservationRecord{w})
	kept, ok := scopeRuntimeEventTableForRequest(original, r, rm.RuntimeArtifactScopeProfile, rm, roster)
	if !ok || len(kept.EventData.Rows) != 1 || !reflect.DeepEqual(kept.EventData.Coverage, original.EventData.Coverage) {
		t.Fatal("native participant projection changed coverage")
	}
	noWindow, ok := scopeRuntimeEventTableForRequest(original, r, nil, rm, roster)
	if !ok || len(noWindow.EventData.Rows) != 1 || noWindow.nativeEventRequestExcluded != 0 {
		t.Fatal("participant selection invented a window for a no-window request")
	}
	dropped, ok := scopeRuntimeEventTableForRequest(original, r, rm.RuntimeArtifactScopeProfile, rm, TraceEventParticipantDisplayRoster{})
	if !ok || len(dropped.EventData.Rows) != 0 || dropped.nativeEventTargetExcluded != 1 || dropped.nativeEventRequestExcluded != 0 {
		t.Fatal("target exclusion confused with time or omission")
	}
	cloned := dropped.Clone()
	ref := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{}}
	if !BindRuntimeMeasurementReceipt(ref, &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{cloned}}) || !strings.Contains(strings.Join(ref.BoundTable.Notes, " "), "请求主体") || strings.Contains(strings.Join(ref.BoundTable.Notes, " "), "不满足请求时间窗") {
		t.Fatal("clone/rebind lost exact target exclusion disclosure")
	}
	pub := RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []RuntimeMeasurementTable{kept}}
	if got := coalesceRuntimeMeasurementPublications([]RuntimeMeasurementPublication{pub, pub}); len(got) != 1 {
		t.Fatal("identical scoped publications not coalesced")
	}
	other := pub
	other.Tables = []RuntimeMeasurementTable{dropped}
	if got := coalesceRuntimeMeasurementPublications([]RuntimeMeasurementPublication{pub, other}); len(got) != 0 {
		t.Fatal("different participant sets silently coalesced")
	}
	if len(original.EventData.Rows) != 1 || original.nativeEventTargetExcluded != 0 {
		t.Fatal("projection mutated producer table")
	}
	// Causal questions retain the previous whole-query target behavior.
	rm.RuntimeQuestionProfile = nil
	legacy, ok := scopeRuntimeEventTableForRequest(original, r, rm.RuntimeArtifactScopeProfile, rm, TraceEventParticipantDisplayRoster{})
	if !ok || len(legacy.EventData.Rows) != 1 {
		t.Fatal("participant projection changed a non-finite lane")
	}
}
