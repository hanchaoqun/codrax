package types

import (
	"sort"
	"strings"
)

// ExternalObservationSufficiencyStatus is the typed verdict for a small,
// answer-grade external observation surface. Sufficient does not discharge a
// separate soft current-source obligation; Scope describes that boundary.
type ExternalObservationSufficiencyStatus string

const (
	ExternalObservationSufficiencyUnknown                ExternalObservationSufficiencyStatus = ""
	ExternalObservationSufficiencyInsufficient           ExternalObservationSufficiencyStatus = "insufficient"
	ExternalObservationSufficiencyBlockedByCurrentSource ExternalObservationSufficiencyStatus = "blocked_by_current_source"
	ExternalObservationSufficiencySufficientForAnswer    ExternalObservationSufficiencyStatus = "sufficient_for_answer"
)

func (s ExternalObservationSufficiencyStatus) Sufficient() bool {
	return s == ExternalObservationSufficiencySufficientForAnswer
}

type ExternalObservationSufficiencyScope string

const (
	ExternalObservationSufficiencyScopeUnknown              ExternalObservationSufficiencyScope = ""
	ExternalObservationSufficiencyScopeExternalLane         ExternalObservationSufficiencyScope = "external_observation_lane"
	ExternalObservationSufficiencyScopeSourceOptionalAnswer ExternalObservationSufficiencyScope = "source_optional_answer"
)

// ExternalObservationSufficiency summarizes a small, addressable external
// observation set. It is intentionally diagnostic: callers may use Status as
// an external-lane readiness signal, not a whole-task completion or waiver.
// CurrentSourceRequirement is compiled once from the shared typed request
// policy, including on insufficient/precise-blocked assessments.
type ExternalObservationSufficiency struct {
	Status                   ExternalObservationSufficiencyStatus `json:"status,omitempty"`
	Scope                    ExternalObservationSufficiencyScope  `json:"scope,omitempty"`
	CurrentSourceRequirement RuntimeSourceRequirementPrecision    `json:"current_source_requirement,omitempty"`
	Reason                   string                               `json:"reason,omitempty"`
	RecordCount              int                                  `json:"record_count,omitempty"`
	AddressableCount         int                                  `json:"addressable_count,omitempty"`
	Origins                  []AnswerEvidenceOrigin               `json:"origins,omitempty"`
	SourceKinds              []ObservationSourceKind              `json:"source_kinds,omitempty"`
}

const externalObservationSufficiencyMaxDirectRecords = 8

// AssessExternalObservationSufficiency reports when typed external
// observations are already an answer-grade surface, separately from whether
// current-source evidence is requested. It must not inspect raw user prose or
// model-authored free text, and does not grant a completion waiver.
func AssessExternalObservationSufficiency(records []ObservationRecord, rm *RequestModel, hint TurnRouteHint) ExternalObservationSufficiency {
	required := runtimeSourceAuthorityRequestCurrentSourceRequired(rm, hint)
	out := ExternalObservationSufficiency{
		CurrentSourceRequirement: runtimeSourceAuthorityRequirementPrecision(rm, hint, required),
	}
	if out.CurrentSourceRequirement == RuntimeSourceRequirementPrecise {
		out.Status = ExternalObservationSufficiencyBlockedByCurrentSource
		out.Reason = "typed precise current-source lane is required"
		return out
	}
	if !externalObservationSufficiencyRouteEligible(rm, hint) {
		out.Status = ExternalObservationSufficiencyInsufficient
		out.Reason = "turn is not typed as external-observation-first"
		return out
	}
	candidates := externalObservationSufficiencyCandidates(records)
	if len(candidates) == 0 {
		out.Status = ExternalObservationSufficiencyInsufficient
		out.Reason = "no small addressable external observations"
		return out
	}
	out.RecordCount = len(candidates)
	out.AddressableCount = len(candidates)
	out.Origins = externalObservationSufficiencyOrigins(candidates)
	out.SourceKinds = externalObservationSufficiencySourceKinds(candidates)
	if len(candidates) > externalObservationSufficiencyMaxDirectRecords {
		out.Status = ExternalObservationSufficiencyInsufficient
		out.Reason = "external observation set is broad; continue normal investigation"
		return out
	}
	out.Status = ExternalObservationSufficiencySufficientForAnswer
	if required {
		out.Scope = ExternalObservationSufficiencyScopeExternalLane
		out.Reason = "small typed external observation set is addressable; the soft current-source obligation remains separate"
	} else {
		out.Scope = ExternalObservationSufficiencyScopeSourceOptionalAnswer
		out.Reason = "small typed external observation set is addressable and current-source evidence is optional"
	}
	return out
}

// RouteBackedExternalObservationRequiresCurrentSource reports that the
// pre-pipeline route classifier explicitly marked current checkout evidence as
// required for an external-observation turn. Repository execution access is
// orthogonal: artifact-only investigations still enter route=repo without
// making checkout evidence load-bearing. This is a lane obligation signal, not
// evidence: it can block external-only sufficiency, but it never creates
// citations or answer facts by itself.
func RouteBackedExternalObservationRequiresCurrentSource(rm *RequestModel, hint TurnRouteHint) bool {
	if !hint.ExternalObservationParticipates() || !hint.RequiresCurrentSourceEvidence() {
		return false
	}
	if rm != nil && rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	return true
}

func externalObservationSufficiencyRouteEligible(rm *RequestModel, hint TurnRouteHint) bool {
	if hint.ExternalObservationFirst() {
		return true
	}
	if rm == nil {
		return false
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() || rm.HasExternalObservationArtifactReference() {
		return true
	}
	if rm.ExternalObservationPolicy != nil {
		return rm.ExternalObservationPolicy.ArtifactCitationsExternalOnly() ||
			rm.ExternalObservationPolicy.ExcludesCurrentSource()
	}
	return false
}

func externalObservationSufficiencyCandidates(records []ObservationRecord) []ObservationRecord {
	if len(records) == 0 {
		return nil
	}
	out := make([]ObservationRecord, 0, len(records))
	for _, record := range records {
		if !AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
			continue
		}
		if !ObservationRecordHasAddressableExternalObservation(record) {
			continue
		}
		if !observationRecordHasExternalObservationContent(record) {
			continue
		}
		out = append(out, record)
	}
	return out
}

// ObservationRecordHasAddressableExternalObservation reports whether an
// origin-specific observation points at a page/row/line/span/selector that can
// be cited as an external observation without converting it into a current
// source line.
func ObservationRecordHasAddressableExternalObservation(record ObservationRecord) bool {
	if !AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
		return false
	}
	span := record.Span
	if span.LineStart > 0 || span.LineEnd > 0 ||
		span.Row > 0 ||
		strings.TrimSpace(span.Selector) != "" ||
		strings.TrimSpace(span.JSONPointer) != "" ||
		span.StartTs > 0 || span.EndTs > 0 ||
		span.StartTsMs > 0 || span.EndTsMs > 0 {
		return true
	}
	for _, ref := range record.SupportRefs {
		if strings.TrimSpace(ref) != "" {
			return true
		}
	}
	ref := record.SourceRef
	return strings.TrimSpace(ref.RawRef) != "" ||
		strings.TrimSpace(ref.RowSetRef) != "" ||
		strings.TrimSpace(ref.PageRef) != ""
}

func observationRecordHasExternalObservationContent(record ObservationRecord) bool {
	for _, value := range []string{
		record.Summary,
		record.RawExcerpt,
		record.Subject,
		record.Predicate,
		record.Object,
		record.Value,
	} {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return record.ResultCount != nil
}

func externalObservationSufficiencyOrigins(records []ObservationRecord) []AnswerEvidenceOrigin {
	seen := map[AnswerEvidenceOrigin]bool{}
	var out []AnswerEvidenceOrigin
	for _, record := range records {
		if record.Origin == AnswerEvidenceOriginUnknown || seen[record.Origin] {
			continue
		}
		seen[record.Origin] = true
		out = append(out, record.Origin)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

func externalObservationSufficiencySourceKinds(records []ObservationRecord) []ObservationSourceKind {
	seen := map[ObservationSourceKind]bool{}
	var out []ObservationSourceKind
	for _, record := range records {
		kind := record.SourceRef.Kind
		if kind == ObservationSourceUnknown || seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}
