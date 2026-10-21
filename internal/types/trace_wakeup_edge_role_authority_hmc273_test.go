package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func hmc273PriorityRoleRecord() ObservationRecord {
	r := traceWakeupEdgeRoleTestRecord("edge", "worker-200", "app-100", "20/ohos_cfs", "52/ohos_rt")
	r.RichNotes = append(r.RichNotes,
		"priority_relation=lower_priority_waker",
		TraceNoteKeyPriorityRelationCaliber+"=closed_range_stable",
		"waker_priority_artifact_source=artifact:0",
		"wakee_priority_artifact_source=artifact:0")
	return r
}

func TestHMC273PriorityRoleNoteWireAndSoftCarrier(t *testing.T) {
	for _, tc := range []struct{ key, wire string }{
		{TraceNoteKeyPriorityRelation, "priority_relation"},
		{TraceNoteKeyWakerPriorityArtifactSource, "waker_priority_artifact_source"},
		{TraceNoteKeyWakeePriorityArtifactSource, "wakee_priority_artifact_source"},
	} {
		t.Run(tc.wire, func(t *testing.T) {
			if tc.key != tc.wire {
				t.Fatalf("role note wire drifted: got %q, want %q", tc.key, tc.wire)
			}
			row, ok := TraceNoteKeyLookup(tc.wire)
			if !ok || row.Family != "gating" || row.Carrier != TraceNoteCarrierSoftConsumer {
				t.Fatalf("parsed role provenance must remain soft context, not causal authority: %+v", row)
			}
		})
	}
}

func hmc273PriorityRoleRequest() *RequestModel {
	return &RequestModel{RuntimeTargets: []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 100, Thread: "app-100", Source: "user_explicit"}}}
}

func hmc273SetPriorityNote(r *ObservationRecord, key, value string) {
	for i, note := range r.RichNotes {
		if strings.HasPrefix(note, key+"=") {
			r.RichNotes[i] = key + "=" + value
			return
		}
	}
	r.RichNotes = append(r.RichNotes, key+"="+value)
}

func TestHMC273PriorityRoleTransportAndCaliber(t *testing.T) {
	for _, caliber := range []string{"exact_at_point", "closed_range_stable"} {
		for _, relation := range []string{"lower_priority_waker", "higher_priority_waker", "same_priority", "raw_priority_uninterpreted"} {
			t.Run(caliber+"/"+relation, func(t *testing.T) {
				r := hmc273PriorityRoleRecord()
				hmc273SetPriorityNote(&r, "priority_relation", relation)
				hmc273SetPriorityNote(&r, TraceNoteKeyPriorityRelationCaliber, caliber)
				hmc273SetPriorityNote(&r, TraceNoteKeyWakerPrioritySource, caliber)
				ledger := ObservationLedger{Records: []ObservationRecord{r, r}}
				before, _ := json.Marshal(ledger)
				got := BuildTraceWakeupEdgeRoleAuthorities(ledger, hmc273PriorityRoleRequest())
				if len(got) != 1 || got[0].PriorityRelation != relation || got[0].PriorityRelationCaliber != caliber ||
					got[0].WakerPriorityArtifactSource != "artifact:0" || got[0].WakeePriorityArtifactSource != "artifact:0" || got[0].SourceRecordID != r.ID {
					t.Fatalf("producer relation/caliber/source was lost or recalculated: %+v", got)
				}
				after, _ := json.Marshal(ledger)
				if string(before) != string(after) {
					t.Fatal("role handoff mutated its native ledger")
				}
			})
		}
	}
}

func TestHMC273PriorityRoleWeakUnknownAndSourceBoundaries(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{TraceNoteKeyPriorityRelationCaliber, ""},
		{TraceNoteKeyPriorityRelationCaliber, "unknown"},
		{TraceNoteKeyPriorityRelationCaliber, "advisory_nearest"},
		{TraceNoteKeyPriorityRelationCaliber, "future_caliber"},
		{"priority_relation", ""},
		{"priority_relation", "future_relation"},
		{TraceNoteKeyWakerPrioritySource, "advisory_nearest"},
		{TraceNoteKeyWakerPrioritySource, ""},
		{TraceNoteKeyWakeePriorityAuthority, "closed_range_stable"},
		{TraceNoteKeyWakeePriorityAuthority, ""},
		{TraceNoteKeyWakerPriorityArtifactSource, ""},
		{TraceNoteKeyWakeePriorityArtifactSource, "artifact:1"},
		{TraceNoteKeyWakeePriority, ""},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			r := hmc273PriorityRoleRecord()
			hmc273SetPriorityNote(&r, tc.key, tc.value)
			got := BuildTraceWakeupEdgeRoleAuthorities(ObservationLedger{Records: []ObservationRecord{r}}, hmc273PriorityRoleRequest())
			if len(got) != 1 || got[0].PriorityRelation != "" || got[0].WakerPriority != "20/ohos_cfs" || got[0].WakerCPU != "2" || got[0].WakeeTargetCPU != "1" {
				t.Fatalf("unqualified relation must fail closed without erasing raw endpoint context: %+v", got)
			}
			wantCaliber := "closed_range_stable"
			if tc.key == TraceNoteKeyPriorityRelationCaliber {
				wantCaliber = tc.value
			}
			if got[0].PriorityRelationCaliber != wantCaliber {
				t.Fatalf("unknown/weak caliber was erased or strengthened: %+v", got[0])
			}
		})
	}
}

func TestHMC273PriorityRoleConflictFingerprint(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"priority_relation", "higher_priority_waker"},
		{TraceNoteKeyPriorityRelationCaliber, "exact_at_point"},
		{TraceNoteKeyPriorityRelationCaliber, "unknown"},
		{TraceNoteKeyWakerPriorityArtifactSource, "artifact:1"},
		{TraceNoteKeyWakeePriorityArtifactSource, "artifact:1"},
		{TraceNoteKeyWakerPrioritySource, "advisory_nearest"},
		{TraceNoteKeyWakeePriorityAuthority, "unknown"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			first, second := hmc273PriorityRoleRecord(), hmc273PriorityRoleRecord()
			hmc273SetPriorityNote(&second, tc.key, tc.value)
			if got := BuildTraceWakeupEdgeRoleAuthorities(ObservationLedger{Records: []ObservationRecord{first, second}}, hmc273PriorityRoleRequest()); len(got) != 0 {
				t.Fatalf("same edge's conflicting relation/caliber/source selected a convenient copy: %+v", got)
			}
			first.RichNotes = append(first.RichNotes, tc.key+"="+tc.value)
			if got := BuildTraceWakeupEdgeRoleAuthorities(ObservationLedger{Records: []ObservationRecord{first}}, hmc273PriorityRoleRequest()); len(got) != 0 {
				t.Fatalf("conflicting copies within one producer note selected first value: %+v", got)
			}
		})
	}
	first, second := hmc273PriorityRoleRecord(), hmc273PriorityRoleRecord()
	first.RichNotes = append(first.RichNotes, "priority_relation=lower_priority_waker")
	if got := BuildTraceWakeupEdgeRoleAuthorities(ObservationLedger{Records: []ObservationRecord{first}}, hmc273PriorityRoleRequest()); len(got) != 1 {
		t.Fatal("identical note copies should remain admissible")
	}
	second.ID = "trace_query:independent#wakeup_chain_edge:1"
	hmc273SetPriorityNote(&second, "priority_relation", "higher_priority_waker")
	if got := BuildTraceWakeupEdgeRoleAuthorities(ObservationLedger{Records: []ObservationRecord{first, second}}, hmc273PriorityRoleRequest()); len(got) != 2 {
		t.Fatalf("independent query scope must not be merged or used to rewrite an earlier edge: %+v", got)
	}
}

func TestHMC273PriorityRoleRetainsSourceTargetWindowGates(t *testing.T) {
	for _, mutate := range []func(*ObservationRecord){
		func(r *ObservationRecord) { r.Origin = AnswerEvidenceOriginCurrentSource },
		func(r *ObservationRecord) { r.Producer = "model" },
		func(r *ObservationRecord) { r.GroundingPolicy = ClaimGroundingSoft },
		func(r *ObservationRecord) { r.Predicate = "nearby_wakeup" },
		func(r *ObservationRecord) { r.Object = "other-300" },
	} {
		r := hmc273PriorityRoleRecord()
		mutate(&r)
		if got := BuildTraceWakeupEdgeRoleAuthorities(ObservationLedger{Records: []ObservationRecord{r}}, hmc273PriorityRoleRequest()); len(got) != 0 {
			t.Fatalf("untrusted or wrong-target role entered recap: %+v", got)
		}
	}
	start, end := 1.0, 1.01
	rm := hmc273PriorityRoleRequest()
	rm.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000000..1.010000"}
	edge := hmc273PriorityRoleRecord()
	carrier := edge
	carrier.ID, carrier.Predicate = "trace_query:window#wakeup_chain:1", "wakeup_chain"
	carrier.RichNotes = []string{TraceNoteKeyWindow + "=1.000000..1.010000"}
	for _, wrongSource := range []bool{false, true} {
		stale := carrier
		if wrongSource {
			stale.SourceRef.Path = "/tmp/unrelated.systrace"
		} else {
			stale.RichNotes = []string{TraceNoteKeyWindow + "=0.000000..2.000000"}
		}
		if got := BuildTraceWakeupEdgeRoleAuthorities(ObservationLedger{Records: []ObservationRecord{edge, stale}}, rm); len(got) != 0 {
			t.Fatalf("wrong source/window carrier authorized edge: %+v", got)
		}
	}
	if got := BuildTraceWakeupEdgeRoleAuthorities(ObservationLedger{Records: []ObservationRecord{edge, carrier}}, rm); len(got) != 1 || got[0].PriorityRelation != "lower_priority_waker" {
		t.Fatalf("exact source/window carrier lost native priority relation: %+v", got)
	}
}
