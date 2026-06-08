package types

import (
	"sort"
	"strings"
)

// ExternalObservationSufficiencyStatus is the typed verdict for whether
// non-current-source observations are enough to answer without source sidecar
// reads. It consumes only structured routing/request/ledger fields.
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

// ExternalObservationSufficiency summarizes a small, addressable external
// observation set. It is intentionally diagnostic: callers may use Status as
// the precise signal and render the rest as advisory context.
type ExternalObservationSufficiency struct {
	Status           ExternalObservationSufficiencyStatus `json:"status,omitempty"`
	Reason           string                               `json:"reason,omitempty"`
	RecordCount      int                                  `json:"record_count,omitempty"`
	AddressableCount int                                  `json:"addressable_count,omitempty"`
	Origins          []AnswerEvidenceOrigin               `json:"origins,omitempty"`
	SourceKinds      []ObservationSourceKind              `json:"source_kinds,omitempty"`
}

const externalObservationSufficiencyMaxDirectRecords = 8

// AssessExternalObservationSufficiency reports when typed external
// observations are already an answer-grade surface and current-source evidence
// is optional. It must not inspect raw user prose or model-authored free text.
func AssessExternalObservationSufficiency(records []ObservationRecord, rm *RequestModel, hint TurnRouteHint) ExternalObservationSufficiency {
	if externalObservationSufficiencyCurrentSourceRequired(rm, hint) {
		return ExternalObservationSufficiency{
			Status: ExternalObservationSufficiencyBlockedByCurrentSource,
			Reason: "typed current-source lane is required",
		}
	}
	if !externalObservationSufficiencyRouteEligible(rm, hint) {
		return ExternalObservationSufficiency{
			Status: ExternalObservationSufficiencyInsufficient,
			Reason: "turn is not typed as external-observation-first",
		}
	}
	candidates := externalObservationSufficiencyCandidates(records)
	if len(candidates) == 0 {
		return ExternalObservationSufficiency{
			Status: ExternalObservationSufficiencyInsufficient,
			Reason: "no small addressable external observations",
		}
	}
	if len(candidates) > externalObservationSufficiencyMaxDirectRecords {
		return ExternalObservationSufficiency{
			Status:           ExternalObservationSufficiencyInsufficient,
			Reason:           "external observation set is broad; continue normal investigation",
			RecordCount:      len(candidates),
			AddressableCount: len(candidates),
			Origins:          externalObservationSufficiencyOrigins(candidates),
			SourceKinds:      externalObservationSufficiencySourceKinds(candidates),
		}
	}
	return ExternalObservationSufficiency{
		Status:           ExternalObservationSufficiencySufficientForAnswer,
		Reason:           "small typed external observation set is addressable and current-source evidence is optional",
		RecordCount:      len(candidates),
		AddressableCount: len(candidates),
		Origins:          externalObservationSufficiencyOrigins(candidates),
		SourceKinds:      externalObservationSufficiencySourceKinds(candidates),
	}
}

func externalObservationSufficiencyCurrentSourceRequired(rm *RequestModel, hint TurnRouteHint) bool {
	if rm == nil {
		return false
	}
	if rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		return true
	}
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	if rm.HasTypedCurrentSourceScopeRequest() {
		return true
	}
	return rm.CurrentSourceLaneDecision().RequiresCurrentSource() && !hint.ExternalObservationFirst()
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
