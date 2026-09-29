package types

// RequestsAggregateWithoutMemberRoster distinguishes a complete input census
// from a requirement to display every input member. Counts may coexist with
// explanations and source citations without making those inputs answer rows.
// Explicit member/table/category obligations win; intent alone does not.
func RequestsAggregateWithoutMemberRoster(rm RequestModel) bool {
	if rm.Predicates.IsCategoryEnumeration || rm.Predicates.HasPerMemberTable {
		return false
	}
	view := rm.QuestionStructure()
	if (view.EnumerationBoundary != nil && view.EnumerationBoundary.DeclaredCount > 0) || len(view.Buckets) >= 2 {
		return false
	}
	count := rm.Predicates.IsCountQuestion
	if profile := rm.RequestedAnswerDimensions; profile != nil && profile.Active() {
		for _, dimension := range profile.Dimensions {
			if !dimension.Required {
				continue
			}
			switch dimension.Role {
			case RequestedAnswerDimensionMemberSet:
				return false
			case RequestedAnswerDimensionCount:
				count = true
			}
		}
	}
	return count
}

// SourceInventoryIsPathDiscovery prevents a pure filesystem census from
// entering a semantic-declaration lens that cannot enumerate file members.
// Mixed file+declaration requests retain the semantic lane.
func SourceInventoryIsPathDiscovery(profile *SourceInventoryProfile) bool {
	if profile == nil || !profile.Active() || profile.RequiresConstSet ||
		(profile.TypeUnderlying != "" && profile.TypeUnderlying != SourceInventoryTypeUnderlyingUnknown) {
		return false
	}
	roles := profile.PrincipalTargetRoles()
	if len(roles) == 0 {
		return false
	}
	for _, role := range roles {
		if role != AnswerCandidateRoleFile {
			return false
		}
	}
	return true
}
