package types

import (
	"encoding/json"
	"testing"
)

func rawProjectionRecordHMC268(t *testing.T) ObservationRecord {
	t.Helper()
	r := measurementProviderRecord(t)
	r.Predicate, r.Role = "measurements_observation", AnswerAggregateRoleSupportingCoverage
	var ok bool
	r, ok = PublishRuntimeRawMeasurement(r)
	if !ok || !runtimeRawMeasurementValid(r) {
		t.Fatal("producer did not seal validated raw projection")
	}
	return r
}

func rewriteRawProjectionHMC268(t *testing.T, r ObservationRecord, change func(*ObservationRecord, *RuntimeMeasurementPublication)) ObservationRecord {
	t.Helper()
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok {
		t.Fatal("missing initial projection")
	}
	change(&r, &p)
	p.Source = r.SourceRef
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(b)}
	return r
}

func TestHMC268RawProjectionFinalizationOnlyDecoratesSource(t *testing.T) {
	original := rawProjectionRecordHMC268(t)
	decorated := rewriteRawProjectionHMC268(t, original, func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
		r.SourceRef.TimeDomain, r.SourceRef.CanonicalTimeDomain, r.SourceRef.ClockAlignment = "capture", "monotonic", "calibrated"
		r.SourceRef.ClockCalibrated = true
		offset, slope := 0.1, 1.0
		r.SourceRef.ClockOffsetSec, r.SourceRef.ClockSlope = &offset, &slope
		r.Span.Selector = "source lines 1-10"
		r.SupportRefs = []string{"artifact:current"}
	})
	decorated.RichNotes = append(decorated.RichNotes, TraceNoteKeyCapacityTruncated+"=true")
	if runtimeRawMeasurementValid(decorated) {
		t.Fatal("source decoration remained valid before producer finalized it")
	}
	if !runtimeRawMeasurementValid(FinalizeRuntimeRawMeasurement(decorated)) || !runtimeRawMeasurementValid(original) {
		t.Fatal("valid source decoration failed or resealing mutated prior value")
	}
	for name, change := range map[string]func(*ObservationRecord, *RuntimeMeasurementPublication){
		"row": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) { p.Tables[0].Rows[0][0] = "60 Hz" },
		"notes": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Notes = []string{"identity proven"}
		},
		"columns": func(_ *ObservationRecord, p *RuntimeMeasurementPublication) {
			p.Tables[0].Columns[0] = "physical state"
		},
		"unit":    func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Unit = "Hz" },
		"object":  func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Object = "device identity" },
		"value":   func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Value = "60" },
		"summary": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Summary = "changed semantics" },
		"role": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
			r.Role = AnswerAggregateRolePrincipalAnswer
		},
		"authority": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
			r.ClaimAuthority = ObservationClaimAuthorityIndependentlyProven
		},
		"span":   func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.Span.EndTs = 7 },
		"window": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryWindowEndTs = 2 },
		"target": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryTargetThread = "other" },
		"lines": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
			r.SourceRef.QueryLineRangeKnown, r.SourceRef.QueryLineEnd = true, 3
		},
		"path": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.Path = "/different" },
		"payload": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) {
			r.SourceRef.PayloadRef = "/different.json"
		},
		"query": func(r *ObservationRecord, _ *RuntimeMeasurementPublication) { r.SourceRef.QueryScopeID = "different" },
	} {
		t.Run(name, func(t *testing.T) {
			r := rewriteRawProjectionHMC268(t, original, change)
			if runtimeRawMeasurementValid(FinalizeRuntimeRawMeasurement(r)) {
				t.Fatal("producer finalization resealed a non-decoration mutation")
			}
		})
	}
	var restored ObservationRecord
	b, _ := json.Marshal(original)
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if runtimeRawMeasurementValid(FinalizeRuntimeRawMeasurement(restored)) {
		t.Fatal("JSON restored producer-only seal")
	}
}

func TestHMC268RawFixedTableCloneAndPayloadIntegrity(t *testing.T) {
	r := rawProjectionRecordHMC268(t)
	p, _ := DecodeRuntimeMeasurementPublication(r)
	qualifyRuntimeRawFactPublication(&p, true)
	if !p.Tables[0].HasNativeRawFacts() {
		t.Fatal("current producer table not qualified")
	}
	clone := p.Tables[0].Clone()
	if !clone.HasNativeRawFacts() {
		t.Fatal("in-memory clone lost private qualification")
	}
	clone.Notes = append(clone.Notes, "invented unit")
	if clone.IsValid() || clone.HasNativeFixedFacts() {
		t.Fatal("changed table retained qualification")
	}
	var restored RuntimeMeasurementTable
	b, _ := json.Marshal(p.Tables[0])
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.HasNativeFixedFacts() {
		t.Fatal("serialized shape acquired fixed-fact qualification")
	}
}
