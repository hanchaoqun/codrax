package types

func sourceInventoryObservationZeroSetFromAdvisory(set SourceInventoryAdvisorySet) (SourceInventoryObservationSet, bool) {
	role, ok := NormalizeAnswerCandidateRole(string(set.Role))
	if !ok || role == AnswerCandidateRoleUnknown {
		return SourceInventoryObservationSet{}, false
	}
	return SourceInventoryObservationSet{
		Role:     role,
		Complete: set.Complete,
		Total:    set.Total,
	}, true
}

func sourceInventoryObservationAppendZeroSetFromAdvisory(out *SourceInventoryObservation, set SourceInventoryAdvisorySet) bool {
	if len(set.Candidates) != 0 {
		return false
	}
	if zeroSet, ok := sourceInventoryObservationZeroSetFromAdvisory(set); ok && out != nil {
		out.Sets = append(out.Sets, zeroSet)
	}
	return true
}
