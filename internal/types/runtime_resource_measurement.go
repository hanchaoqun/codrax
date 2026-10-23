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
	digest                  string
	endInclusive            bool
	statisticsBudgetOmitted bool
	eventDisplay            *runtimeResourceEventDisplayIndex
}

func PublishRuntimeResourceMeasurement(r ObservationRecord, tables []RuntimeMeasurementTable, endInclusive bool) (ObservationRecord, bool) {
	ref := r.SourceRef
	if r.Predicate != RuntimeResourceMeasurementPredicate || r.ClaimKey != r.Predicate || r.Subject != "native resource stacks" || r.Object != "available" || r.Negative ||
		!RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Origin != AnswerEvidenceOriginRuntimeArtifact || r.Role != AnswerAggregateRoleSupportingCoverage ||
		r.GroundingPolicy != ClaimGroundingHard || r.ProvenanceLane != ObservationProvenanceArtifactSpan ||
		ref.Kind != ObservationSourceRuntimeArtifact || !ref.QueryWindowKnown || !ref.QueryLineRangeKnown || ref.QueryLineStart != 0 || ref.QueryLineEnd != 0 ||
		(len(tables) != 2 && len(tables) != 3) ||
		(!runtimeResourceExactSelector(ref) && !(runtimeResourceBroadSelector(ref) && len(tables) == 3)) {
		return r, false
	}
	for i, t := range tables {
		view := RuntimeMeasurementSummary
		if i == 1 {
			view = RuntimeMeasurementMembers
		} else if i == 2 {
			view = RuntimeMeasurementDistribution
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

// Broad native facts remain a display-only query population, never an exact
// target or member-completeness credential. Legacy two-table producers do not
// gain this path; the event-statistics producer supplies the third view.
func runtimeResourceBroadSelector(ref ObservationSourceRef) bool {
	return ref.QueryTargetScope == "thread" && ref.QueryTargetPID == 0 && ref.QueryTargetThread == ""
}

// WithRuntimeResourceStatisticsDisplayOmission records a bounded publication
// disposition on an already native-sealed legacy pair. It grants no new facts.
func WithRuntimeResourceStatisticsDisplayOmission(r ObservationRecord) ObservationRecord {
	if !runtimeResourcePublicationValid(r) {
		return r
	}
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok || len(p.Tables) != 2 {
		return r
	}
	witness := *r.runtimeResourcePublication
	witness.statisticsBudgetOmitted = true
	r.runtimeResourcePublication = &witness
	return r
}

func RuntimeResourceStatisticsDisplayOmitted(r ObservationRecord) bool {
	return runtimeResourcePublicationValid(r) && r.runtimeResourcePublication.statisticsBudgetOmitted
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
	return t.HasNativeSamplingFacts() || t.HasNativeResourceFacts() || t.HasNativeRawFacts() || t.HasNativeMeasurementFacts()
}

func runtimeResourceMeasurementMatchesRequest(r ObservationRecord, rm *RequestModel) bool {
	if r.Predicate != RuntimeResourceMeasurementPredicate {
		return true
	}
	if runtimeResourceBroadSelector(r.SourceRef) {
		if rm == nil {
			return false
		}
		for _, target := range rm.RuntimeTargets {
			if !RuntimeTargetIsExplorationCursorSource(target.Source) {
				return false
			}
		}
		return true
	}
	if rm == nil {
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

// RuntimeResourceQueryIsSupplementary separates an executed factual selector
// from the current answer target roster. It does not restore dropped targets,
// prove user intent, or admit causal evidence. Callers still need the current
// physical-source receipt and the original request-window checks.
func RuntimeResourceQueryIsSupplementary(r ObservationRecord, rm *RequestModel) bool {
	if rm == nil || rm.RuntimeQuestionProfile == nil || rm.RuntimeQuestionProfile.Scope != RuntimeQuestionScopeBoundedFactSet ||
		!rm.RuntimeQuestionProfile.SuppressesRootCauseRankingPrompt() || !rm.RuntimeQuestionProfile.CarriesBoundedFactFamilies() ||
		!runtimeResourcePublicationValid(r) || !runtimeResourceExactSelector(r.SourceRef) || runtimeResourceMeasurementMatchesRequest(r, rm) {
		return false
	}
	for _, target := range rm.RuntimeTargets {
		if !RuntimeTargetIsExplorationCursorSource(target.Source) && (target.Kind == RuntimeTargetKindThread || target.Kind == RuntimeTargetKindProcess) {
			return true
		}
	}
	return false
}

const RuntimeResourceSupplementaryScopeNote = "补充查询：这是实际执行的独立精确查询，未匹配当前分析目标名册；名册缺项不等于没有查询或没有事件，也不证明用户要求或排除此主体。按原问题需要引用其同源事实，不替代已选目标、不合并总体、不授根因或请求成员全集资格。"

func supplementaryRuntimeResourceTable(table RuntimeMeasurementTable) RuntimeMeasurementTable {
	table = table.Clone()
	table.DefaultPresentation = false
	table.coverageScope = nil
	table.nativeTraceDisplayPopulation = false
	table.Label = "补充查询 / Supplementary query: " + table.Label
	table.Notes = append(table.Notes, RuntimeResourceSupplementaryScopeNote)
	// This is a deterministic display projection of already validated native
	// rows. Changing its scope metadata must reseal the unchanged fact payload.
	table.nativeResourceFactsDigest = runtimeSamplingDigest(table)
	return table
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
