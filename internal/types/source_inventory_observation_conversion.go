package types

import "strings"

// SourceInventoryObservationFromAdvisory converts engine-built advisory rows
// into the durable typed observation consumed by downstream answer contracts.
func SourceInventoryObservationFromAdvisory(advisory SourceInventoryAdvisory) SourceInventoryObservation {
	if !advisory.IsActive() {
		return SourceInventoryObservation{}
	}
	out := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: advisory.AdvisoryOnly,
		Complete:     advisory.Complete,
		Scopes:       append([]string(nil), advisory.Scopes...),
		Provenance:   append([]string(nil), advisory.Provenance...),
		Lens:         []string{"members", "symbols", "attributes", "count"},
		Sets:         make([]SourceInventoryObservationSet, 0, len(advisory.Sets)),
	}
	for _, set := range advisory.Sets {
		if sourceInventoryObservationAppendZeroSetFromAdvisory(&out, set) {
			continue
		}
		role, ok := NormalizeAnswerCandidateRole(string(set.Role))
		if !ok || role == AnswerCandidateRoleUnknown {
			continue
		}
		obsSet := SourceInventoryObservationSet{
			Role:     role,
			Complete: set.Complete,
			Total:    set.Total,
			Members:  make([]SourceInventoryObservationMember, 0, len(set.Candidates)),
		}
		for _, candidate := range set.Candidates {
			member := sourceInventoryObservationMemberFromAdvisory(candidate)
			if strings.TrimSpace(member.Name) == "" {
				continue
			}
			obsSet.Members = append(obsSet.Members, member)
		}
		obsSet.Count = len(obsSet.Members)
		if obsSet.Total < obsSet.Count {
			obsSet.Total = obsSet.Count
		}
		if obsSet.Count == 0 {
			continue
		}
		out.Sets = append(out.Sets, obsSet)
	}
	return normalizeSourceInventoryObservation(out)
}

func sourceInventoryObservationMemberFromAdvisory(candidate SourceInventoryAdvisoryCandidate) SourceInventoryObservationMember {
	attrs := make([]SourceInventoryObservationAttribute, 0, len(candidate.Attributes))
	attributeAmbiguity := ""
	if len(candidate.Attributes) > 1 {
		attributeAmbiguity = "one_of_many_candidate_attributes"
	}
	for _, attr := range candidate.Attributes {
		item := SourceInventoryObservationAttribute{
			Name:          attr.Member,
			Key:           attr.Key,
			SupportRef:    attr.SupportRef,
			Note:          attr.Note,
			SurfaceTerms:  append([]string(nil), attr.SurfaceTerms...),
			Role:          attr.Role,
			SourceClass:   sourceInventoryObservationPathClass(attr.SourceClass, attr.File),
			Exported:      attr.Exported,
			File:          attr.File,
			Line:          attr.Line,
			Language:      attr.Language,
			CoverageState: sourceInventoryObservationCoverage(attr.SupportRef, attr.File),
			Ambiguity:     attributeAmbiguity,
		}
		if attributeAmbiguity != "" {
			item.Reason = "Multiple graph-backed callable attributes are present under this member; the model must choose or disclose ambiguity."
			item.CoverageState = SourceInventoryCoverageAmbiguous
		}
		if strings.TrimSpace(item.Name) != "" {
			attrs = append(attrs, item)
		}
	}
	return SourceInventoryObservationMember{
		Name:          candidate.Member,
		Key:           candidate.Key,
		SupportRef:    candidate.SupportRef,
		Note:          candidate.Note,
		SurfaceTerms:  append([]string(nil), candidate.SurfaceTerms...),
		Role:          candidate.Role,
		SourceClass:   sourceInventoryObservationPathClass(candidate.SourceClass, candidate.File),
		Exported:      candidate.Exported,
		File:          candidate.File,
		Line:          candidate.Line,
		Language:      candidate.Language,
		CoverageState: sourceInventoryObservationCoverage(candidate.SupportRef, candidate.File),
		Attributes:    attrs,
	}
}

func sourceInventoryObservationPathClass(class SourcePathRole, file string) SourcePathRole {
	if class != "" && class != SourcePathRoleUnknown {
		return class
	}
	return ClassifySourcePathRole(file)
}

func sourceInventoryObservationCoverage(supportRef, file string) SourceInventoryCoverageState {
	if strings.TrimSpace(supportRef) != "" || strings.TrimSpace(file) != "" {
		return SourceInventoryCoverageObserved
	}
	return SourceInventoryCoverageNeedsRead
}
