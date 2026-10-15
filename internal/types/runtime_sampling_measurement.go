package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

const RuntimeSamplingMeasurementPredicate = "perf_sampling_facts"

// The native producer issues this non-serializable display witness only after
// checking its own unchanged sampling population and physical generation.
// It grants neither causal edges nor complete source/member-set authority.
type runtimeSamplingPublicationWitness struct {
	digest       string
	endInclusive bool
}

func runtimeSamplingDigest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// PublishRuntimeSamplingMeasurement binds fixed native display rows to the
// exact observation envelope. Model/JSON input cannot reconstruct the witness.
func PublishRuntimeSamplingMeasurement(r ObservationRecord, tables []RuntimeMeasurementTable, endInclusive bool) (ObservationRecord, bool) {
	if r.Predicate != RuntimeSamplingMeasurementPredicate || r.Negative || r.Role != AnswerAggregateRoleSupportingCoverage ||
		!RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Origin != AnswerEvidenceOriginRuntimeArtifact ||
		r.GroundingPolicy != ClaimGroundingHard || r.ProvenanceLane != ObservationProvenanceArtifactSpan ||
		r.SourceRef.Kind != ObservationSourceRuntimeArtifact || !r.SourceRef.QueryWindowKnown || r.SourceRef.QueryTargetScope != "thread" || r.Subject == "" || len(tables) == 0 {
		return r, false
	}
	for _, table := range tables {
		if !table.IsValid() || table.ObservationID != r.ID || table.MemberSet != nil || table.DisplayPopulation != nil || table.EventData != nil || table.DefaultPresentation {
			return r, false
		}
	}
	p := RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: tables}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 64<<10 {
		return r, false
	}
	r.RichNotes = append(append([]string(nil), r.RichNotes...), TraceNoteKeyRuntimeMeasurement+"="+string(raw))
	r.runtimeSamplingPublication = &runtimeSamplingPublicationWitness{digest: runtimeSamplingDigest(r), endInclusive: endInclusive}
	return r, true
}

func runtimeSamplingMeasurementWithinWindows(r ObservationRecord, scope *RuntimeArtifactScopeProfile) bool {
	if r.Predicate != RuntimeSamplingMeasurementPredicate || !scope.HasExplicitTimeWindows() {
		return true
	}
	if !runtimeSamplingPublicationValid(r) {
		return false
	}
	for _, window := range scope.ExplicitTimeWindows() {
		if r.SourceRef.QueryWindowStartTs >= *window.TimeStart && r.SourceRef.QueryWindowEndTs <= *window.TimeEnd &&
			(!r.runtimeSamplingPublication.endInclusive || r.SourceRef.QueryWindowEndTs < *window.TimeEnd) {
			return true
		}
	}
	return false
}

func runtimeSamplingPublicationValid(r ObservationRecord) bool {
	return r.runtimeSamplingPublication != nil && r.runtimeSamplingPublication.digest != "" && r.runtimeSamplingPublication.digest == runtimeSamplingDigest(r)
}

// HasNativeSamplingFacts is a display-only capability on the bound native
// table. JSON round trips and mutations cannot carry it to a different table.
func (t RuntimeMeasurementTable) HasNativeSamplingFacts() bool {
	return t.nativeSamplingFactsDigest != "" && t.nativeSamplingFactsDigest == runtimeSamplingDigest(t)
}

func runtimeSamplingMeasurementMatchesRequest(r ObservationRecord, rm *RequestModel) bool {
	if r.Predicate != RuntimeSamplingMeasurementPredicate || rm == nil {
		return true
	}
	var targets []RuntimeTarget
	hasTarget := false
	for _, target := range rm.RuntimeTargets {
		if RuntimeTargetIsExplorationCursorSource(target.Source) {
			continue
		}
		hasTarget = true
		if target.Kind != RuntimeTargetKindProcess {
			targets = append(targets, target)
		}
	}
	return !hasTarget || ObservationRecordMatchesUserRuntimeTarget(r, &RequestModel{RuntimeTargets: targets})
}
