package types

import "strings"

const RuntimeRawFactDisplayTeaching = "Published raw measurements and acquisition metadata/status also support summary/table/diagram with the exact observation_id/view selector and no copied text/items/columns/diagram. All three surfaces reuse the same retained rows, exact storage types, original references, unknown/invalid/missing values and coverage notes. Parser labels remain recorded labels, not device/workload identities; raw values do not acquire units or state meanings, and acquisition status is not a measured value. A fact diagram has no relationship edges."

// A raw publication seal protects the already validated producer projection,
// not its physical interpretation. The existing private source receipt still
// owns freshness/epoch/window checks. Neither JSON nor a valid table shape can
// recreate this seal, and the contract only qualifies the final current table.
type runtimeRawMeasurementWitness struct{ digest, projection string }

func rawMeasurementPredicate(predicate string) bool {
	return predicate == "measurements_observation" || predicate == "process_measurements_observation" || predicate == "unpositioned_measurements_observation" || predicate == "packet_inventory_observation"
}

// PublishRuntimeRawMeasurement is called by the native raw-record producers
// after their domain payload validation and lossless table projection. It does
// not infer units, state meanings, hardware identity, completeness or causality.
func PublishRuntimeRawMeasurement(r ObservationRecord) (ObservationRecord, bool) {
	if !rawMeasurementPredicate(r.Predicate) || r.Role != AnswerAggregateRoleSupportingCoverage {
		return r, false
	}
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok {
		return r, false
	}
	for _, table := range p.Tables {
		if table.MemberSet != nil || table.EventData != nil {
			return r, false
		}
	}
	r.runtimeRawMeasurementPublication = &runtimeRawMeasurementWitness{digest: runtimeSamplingDigest(r), projection: runtimeRawProjectionDigest(r, p)}
	return r, true
}

func runtimeRawProjectionDigest(r ObservationRecord, p RuntimeMeasurementPublication) string {
	// Only native source-decoration fields may change before finalization.
	// Keep semantic fields (including Unit/Object/Value) and projected rows/
	// notes exact. Capacity disclosure is the sole later shared note.
	r.SourceRef.TimeDomain, r.SourceRef.CanonicalTimeDomain, r.SourceRef.ClockAlignment = "", "", ""
	r.SourceRef.ClockCalibrated = false
	r.SourceRef.ClockOffsetSec, r.SourceRef.ClockSlope = nil, nil
	r.Span.Selector, r.SupportRefs = "", nil
	var notes []string
	for _, note := range r.RichNotes {
		if !strings.HasPrefix(note, TraceNoteKeyRuntimeMeasurement+"=") && note != TraceNoteKeyCapacityTruncated+"=true" {
			notes = append(notes, note)
		}
	}
	r.RichNotes = notes
	p.Source = ObservationSourceRef{}
	return runtimeSamplingDigest(struct {
		Record      ObservationRecord
		Publication RuntimeMeasurementPublication
	}{r, p})
}

// FinalizeRuntimeRawMeasurement belongs only to the native producer's existing
// final source-decoration step. Capture/clock provenance is attached after the
// raw payload is validated. The private initial seal must still match every
// table field and exact query/payload identity before sealing the final record.
// Consumers never call this function to repair a decoded/mutated publication.
func FinalizeRuntimeRawMeasurement(r ObservationRecord) ObservationRecord {
	w := r.runtimeRawMeasurementPublication
	if w == nil || w.projection == "" {
		return r
	}
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok || w.projection != runtimeRawProjectionDigest(r, p) {
		return r
	}
	r.runtimeRawMeasurementPublication = &runtimeRawMeasurementWitness{digest: runtimeSamplingDigest(r), projection: w.projection}
	return r
}

func runtimeRawMeasurementValid(r ObservationRecord) bool {
	return rawMeasurementPredicate(r.Predicate) && r.runtimeRawMeasurementPublication != nil &&
		r.runtimeRawMeasurementPublication.digest != "" && r.runtimeRawMeasurementPublication.digest == runtimeSamplingDigest(r)
}

// HasNativeRawFacts also covers current producer-built acquisition metadata
// and status. It authorizes verbatim display only, including invalid/missing
// fields and failed/not-executed states; it is not measurement-success proof.
func (t RuntimeMeasurementTable) HasNativeRawFacts() bool {
	return t.nativeRawFactsDigest != "" && t.nativeRawFactsDigest == runtimeSamplingDigest(t)
}

func sealRuntimeRawFactTable(t *RuntimeMeasurementTable) {
	t.nativeRawFactsDigest = runtimeSamplingDigest(*t)
}

func qualifyRuntimeRawFactPublication(p *RuntimeMeasurementPublication, current bool) {
	if p == nil || !p.rawFacts || !current {
		return
	}
	for i := range p.Tables {
		sealRuntimeRawFactTable(&p.Tables[i])
	}
}
