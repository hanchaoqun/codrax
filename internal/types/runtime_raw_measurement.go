package types

import (
	"encoding/json"
	"io"
	"strings"
)

const RuntimeRawFactDisplayTeaching = "Published raw measurements and acquisition metadata/status also support summary/table/diagram with the exact observation_id/view selector and no copied text/items/columns/diagram. All three surfaces reuse the same retained rows, exact storage types, original references, unknown/invalid/missing values and coverage notes. Parser labels remain recorded labels, not device/workload identities; raw values do not acquire units or state meanings, and acquisition status is not a measured value. A fact diagram has no relationship edges."

// A publication seal protects the already validated producer projection,
// not its physical interpretation. The existing private source receipt still
// owns freshness/epoch/window checks. Neither JSON nor a valid table shape can
// recreate this seal, and the contract only qualifies the final current table.
// The optional endpoint is display metadata only. Reusing this seal for a
// native interval does not put its predicate in the raw-fact authority lane.
type runtimeRawMeasurementWitness struct {
	digest, projection string
	queryWindowEnd     runtimeMeasurementQueryEnd
}

// Known-interval witnesses store this value as an immutable JSON string in
// projection. Raw witnesses retain their strict projection digest instead.
// Optional display notes may be deleted, not added, replaced, reordered or
// moved between tables. All other facts and the current source remain exact.
// The baseline is never shared as a mutable slice or reconstructed from wire
// observations: only the native publisher can issue the private witness.
type runtimeMeasurementQueryWindowProof struct {
	Projection string
	Core       string
	Notes      [][]string
}

func rawMeasurementPredicate(predicate string) bool {
	return predicate == "measurements_observation" || predicate == "process_measurements_observation" || predicate == "unpositioned_measurements_observation" || predicate == "packet_inventory_observation" || predicate == "packet_flows_observation"
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

// PublishRuntimeMeasurementQueryWindow belongs to the native interval
// producers, after their typed window and complete table validation. It
// preserves the publication's query ruler, not a member envelope or a request
// window. Legacy receipts, point populations and other producers stay unknown.
// An existing seal cannot be replaced to repair mutated facts or endpoints.
func PublishRuntimeMeasurementQueryWindow(r ObservationRecord, start, end float64, endInclusive bool) ObservationRecord {
	if r.runtimeRawMeasurementPublication != nil || r.Role != AnswerAggregateRoleSupportingCoverage {
		return r
	}
	switch r.Predicate {
	case TraceBusinessSpanPredicate, "io_inflight", "thread_timeline_observation", "cpu_frequency_timeline_observation":
	default:
		return r
	}
	a, b, known := TraceObservationContinuousQueryWindow(r.SourceRef)
	if !known || a != start || b != end {
		return r
	}
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok {
		return r
	}
	proof := runtimeMeasurementQueryWindowProof{
		Projection: runtimeRawProjectionDigest(r, p),
		Core:       runtimeMeasurementEndpointCore(r, p),
		Notes:      make([][]string, len(p.Tables)),
	}
	for i := range p.Tables {
		proof.Notes[i] = p.Tables[i].Notes
	}
	encoded, _ := json.Marshal(proof) // the private value contains only strings
	r.runtimeRawMeasurementPublication = &runtimeRawMeasurementWitness{
		digest: runtimeSamplingDigest(r), projection: string(encoded),
		queryWindowEnd: runtimeMeasurementNativeQueryEnd(endInclusive),
	}
	return r
}

func runtimeMeasurementSealedQueryEnd(r ObservationRecord, p RuntimeMeasurementPublication) runtimeMeasurementQueryEnd {
	w := r.runtimeRawMeasurementPublication
	proof, ok := runtimeMeasurementEndpointProof(w, p)
	if !ok || proof.Core != runtimeMeasurementEndpointCore(r, p) {
		return runtimeMeasurementQueryEndUnknown
	}
	return w.queryWindowEnd
}

func runtimeMeasurementEndpointProof(w *runtimeRawMeasurementWitness, p RuntimeMeasurementPublication) (runtimeMeasurementQueryWindowProof, bool) {
	var proof runtimeMeasurementQueryWindowProof
	if w == nil || w.digest == "" || (w.queryWindowEnd != runtimeMeasurementQueryEndExcluded && w.queryWindowEnd != runtimeMeasurementQueryEndIncluded) {
		return proof, false
	}
	d := json.NewDecoder(strings.NewReader(w.projection))
	d.DisallowUnknownFields()
	if d.Decode(&proof) != nil || d.Decode(new(any)) != io.EOF || proof.Projection == "" || proof.Core == "" || len(proof.Notes) != len(p.Tables) {
		return runtimeMeasurementQueryWindowProof{}, false
	}
	for i, table := range p.Tables {
		// Match occurrences, not a set: duplicate multiplicity and order are
		// part of the producer's optional-note baseline for this exact table.
		next := 0
		for _, note := range proof.Notes[i] {
			if next < len(table.Notes) && table.Notes[next] == note {
				next++
			}
		}
		if next != len(table.Notes) {
			return runtimeMeasurementQueryWindowProof{}, false
		}
	}
	return proof, true
}

func runtimeMeasurementEndpointCore(r ObservationRecord, p RuntimeMeasurementPublication) string {
	// Remove only the optional Notes and their serialized envelope. Unlike
	// the native finalization projection, current source decoration is bound
	// here too; a consumer cannot repair or re-sign changed source identity.
	p.Tables = append([]RuntimeMeasurementTable(nil), p.Tables...)
	for i := range p.Tables {
		p.Tables[i].Notes = nil
	}
	var notes []string
	for _, note := range r.RichNotes {
		if !strings.HasPrefix(note, TraceNoteKeyRuntimeMeasurement+"=") {
			notes = append(notes, note)
		}
	}
	r.RichNotes = notes
	return runtimeSamplingDigest(struct {
		Record      ObservationRecord
		Publication RuntimeMeasurementPublication
	}{r, p})
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
// Known endpoints additionally permit deletion of original optional Notes;
// their strict producer projection is checked with those original Notes.
// Consumers never call this function to repair a decoded/mutated publication.
func FinalizeRuntimeRawMeasurement(r ObservationRecord) ObservationRecord {
	w := r.runtimeRawMeasurementPublication
	if w == nil || w.projection == "" {
		return r
	}
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok {
		return r
	}
	if w.queryWindowEnd != runtimeMeasurementQueryEndUnknown {
		proof, valid := runtimeMeasurementEndpointProof(w, p)
		if !valid {
			return r
		}
		originalNotes := p
		originalNotes.Tables = append([]RuntimeMeasurementTable(nil), p.Tables...)
		for i := range originalNotes.Tables {
			originalNotes.Tables[i].Notes = proof.Notes[i]
		}
		if proof.Projection != runtimeRawProjectionDigest(r, originalNotes) {
			return r
		}
		proof.Core = runtimeMeasurementEndpointCore(r, p)
		encoded, _ := json.Marshal(proof)
		r.runtimeRawMeasurementPublication = &runtimeRawMeasurementWitness{digest: runtimeSamplingDigest(r), projection: string(encoded), queryWindowEnd: w.queryWindowEnd}
		return r
	}
	if w.projection != runtimeRawProjectionDigest(r, p) {
		return r
	}
	r.runtimeRawMeasurementPublication = &runtimeRawMeasurementWitness{digest: runtimeSamplingDigest(r), projection: w.projection, queryWindowEnd: w.queryWindowEnd}
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
