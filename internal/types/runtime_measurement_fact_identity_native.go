package types

import "encoding/json"

// Immutable producer-only strings. The native digest binds every native
// field, including precision/endpoint detail not present in the formatted
// table. The projection seal binds the complete publication and query before
// the existing final source/clock decoration boundary.
type runtimeMeasurementFactWitness struct{ native, projection, record string }

// PublishRuntimeMeasurementFactIdentity is called only after a native producer
// has projected the entire retained measurement (not a preview). It supplies
// display equivalence only: missing source intervals stay missing, and this
// is neither population completeness nor a causal/completion credential.
func PublishRuntimeMeasurementFactIdentity(r ObservationRecord, native json.RawMessage) ObservationRecord {
	r.runtimeMeasurementFactIdentity = nil
	if r.Predicate != "thread_timeline_observation" && r.Predicate != "cpu_frequency_timeline_observation" {
		return r
	}
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok || r.Role != AnswerAggregateRoleSupportingCoverage || len(p.Tables) != 1 ||
		p.Tables[0].View != RuntimeMeasurementTimeline || p.Tables[0].MemberSet != nil || p.Tables[0].DisplayPopulation != nil ||
		len(native) == 0 || !json.Valid(native) || string(native) == "null" {
		return r
	}
	r.runtimeMeasurementFactIdentity = &runtimeMeasurementFactWitness{
		native: runtimeSamplingDigest(native), projection: runtimeRawProjectionDigest(r, p), record: runtimeSamplingDigest(r),
	}
	return r
}

// FinalizeRuntimeMeasurementFactIdentity permits exactly the same final
// source/clock decorations as the raw-fact producer. It cannot mint a missing
// seal or repair changed measurements, target, window, query or source.
func FinalizeRuntimeMeasurementFactIdentity(r ObservationRecord) ObservationRecord {
	w := r.runtimeMeasurementFactIdentity
	if w == nil {
		return r
	}
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok || w.projection != runtimeRawProjectionDigest(r, p) {
		return r
	}
	r.runtimeMeasurementFactIdentity = &runtimeMeasurementFactWitness{native: w.native, projection: w.projection, record: runtimeSamplingDigest(r)}
	return r
}

func qualifyRuntimeMeasurementEquivalentFact(p *RuntimeMeasurementPublication, r ObservationRecord, original string, replay TraceQueryWindowReplayRef) {
	w := r.runtimeMeasurementFactIdentity
	if p == nil || w == nil || original == "" || w.native == "" || w.record != runtimeSamplingDigest(r) ||
		w.projection != runtimeRawProjectionDigest(r, *p) || len(p.Tables) != 1 {
		return
	}
	if _, _, known := TraceObservationContinuousQueryWindow(p.Source); !known {
		return
	}
	ref := replay.source
	if replay.preparedSource != nil {
		ref = replay.preparedSource.source
	} else if replay.nativeNavigation != nil {
		ref = replay.nativeNavigation.source
	}
	if ref.generation == nil || ref.path != p.Source.Path {
		return
	}
	canonical := *p
	canonical.ObservationID = ""
	// Only query/publication locators differ. Target, range, endpoint caliber,
	// clock, source, every table field and all coverage notes remain exact.
	canonical.Source.RawRef, canonical.Source.PayloadRef, canonical.Source.QueryScopeID = "", "", ""
	canonical.Tables = []RuntimeMeasurementTable{p.Tables[0].Clone()}
	canonical.Tables[0].ObservationID = ""
	p.Tables[0].nativeFactIdentity = "measurement:" + runtimeSamplingDigest(struct {
		Original, Generation, Native, Predicate string
		Publication                             RuntimeMeasurementPublication
	}{original, ref.identity.CacheToken(), w.native, r.Predicate, canonical})
}
