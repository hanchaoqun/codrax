package types

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
)

const RuntimeResourceMeasurementPredicate = "resource_stack_observation"

// This is a display-only seal over one native query and its fixed tables.
// JSON cannot recreate it. Source freshness is checked again by the contract
// against the current private prepared/read receipt, not the displayed path.
type runtimeResourcePublicationWitness struct {
	digest       string
	endInclusive bool
}

func PublishRuntimeResourceMeasurement(r ObservationRecord, tables []RuntimeMeasurementTable, endInclusive bool) (ObservationRecord, bool) {
	ref := r.SourceRef
	if r.Predicate != RuntimeResourceMeasurementPredicate || r.ClaimKey != r.Predicate || r.Subject != "native resource stacks" || r.Object != "available" || r.Negative ||
		!RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Origin != AnswerEvidenceOriginRuntimeArtifact || r.Role != AnswerAggregateRoleSupportingCoverage ||
		r.GroundingPolicy != ClaimGroundingHard || r.ProvenanceLane != ObservationProvenanceArtifactSpan ||
		ref.Kind != ObservationSourceRuntimeArtifact || !ref.QueryWindowKnown || !ref.QueryLineRangeKnown || ref.QueryLineStart != 0 || ref.QueryLineEnd != 0 ||
		!runtimeResourceExactSelector(ref) || len(tables) != 2 {
		return r, false
	}
	for i, t := range tables {
		view := RuntimeMeasurementSummary
		if i == 1 {
			view = RuntimeMeasurementMembers
		}
		// Only the event summary is the producer's default. Physical frames
		// remain separately selectable; neither view supplies population or
		// relationship authority. The current request still owns display scope.
		if !t.IsValid() || t.View != view || t.ObservationID != r.ID || t.MemberSet != nil || t.DisplayPopulation != nil || t.EventData != nil || t.DefaultPresentation != (i == 0) {
			return r, false
		}
	}
	p := RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: ref, Tables: tables}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 64<<10 {
		return r, false
	}
	r.RichNotes = append(append([]string(nil), r.RichNotes...), TraceNoteKeyRuntimeMeasurement+"="+string(raw))
	r.runtimeResourcePublication = &runtimeResourcePublicationWitness{digest: runtimeSamplingDigest(r), endInclusive: endInclusive}
	return r, true
}

func runtimeResourceExactSelector(ref ObservationSourceRef) bool {
	if ref.QueryTargetScope == "process" {
		return ref.QueryTargetPID > 0 && ref.QueryTargetPID <= RuntimeTargetMaxPID
	}
	if ref.QueryTargetScope != "thread" {
		return false
	}
	pid, name, valid := threadidentity.QuerySelector(ref.QueryTargetPID, ref.QueryTargetThread)
	return valid && (pid > 0 || name != "")
}

func runtimeResourcePublicationValid(r ObservationRecord) bool {
	return r.runtimeResourcePublication != nil && r.runtimeResourcePublication.digest != "" && r.runtimeResourcePublication.digest == runtimeSamplingDigest(r)
}

func (t RuntimeMeasurementTable) HasNativeResourceFacts() bool {
	return t.nativeResourceFactsDigest != "" && t.nativeResourceFactsDigest == runtimeSamplingDigest(t)
}

// HasNativeFixedFacts is only the common pure display surface. Each producer
// retains its own validation, identity and population rules.
func (t RuntimeMeasurementTable) HasNativeFixedFacts() bool {
	return t.HasNativeSamplingFacts() || t.HasNativeResourceFacts()
}

func runtimeResourceMeasurementMatchesRequest(r ObservationRecord, rm *RequestModel) bool {
	if r.Predicate != RuntimeResourceMeasurementPredicate || rm == nil {
		return true
	}
	hasTarget := false
	for _, target := range rm.RuntimeTargets {
		if RuntimeTargetIsExplorationCursorSource(target.Source) {
			continue
		}
		hasTarget = true
		if target.Kind == RuntimeTargetKindProcess {
			if r.SourceRef.QueryTargetScope == "process" && target.PID > 0 && target.PID == r.SourceRef.QueryTargetPID {
				return true
			}
			continue
		}
		if target.Kind != RuntimeTargetKindThread || r.SourceRef.QueryTargetScope != "thread" {
			continue
		}
		pid, name, valid := threadidentity.QuerySelector(target.PID, target.Thread)
		if valid && (pid > 0 && pid == r.SourceRef.QueryTargetPID || pid == 0 && r.SourceRef.QueryTargetPID == 0 && name != "" && name == r.SourceRef.QueryTargetThread) {
			return true
		}
	}
	return !hasTarget
}

func runtimeResourceMeasurementWithinWindows(r ObservationRecord, scope *RuntimeArtifactScopeProfile) bool {
	if r.Predicate != RuntimeResourceMeasurementPredicate || !scope.HasExplicitTimeWindows() {
		return true
	}
	if !runtimeResourcePublicationValid(r) {
		return false
	}
	for _, window := range scope.ExplicitTimeWindows() {
		if r.SourceRef.QueryWindowStartTs >= *window.TimeStart && r.SourceRef.QueryWindowEndTs <= *window.TimeEnd &&
			(!r.runtimeResourcePublication.endInclusive || r.SourceRef.QueryWindowEndTs < *window.TimeEnd) {
			return true
		}
	}
	return false
}
