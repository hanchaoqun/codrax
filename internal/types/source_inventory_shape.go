package types

// IsTypedSourceEnumerationShape reports a typed source-inventory-like enumeration.
func IsTypedSourceEnumerationShape(rm RequestModel) bool {
	if rm.Intent != IntentEnumerate && NormalizeRequirementKind(rm.AnalyzerHints.Kind) != ReqEnumeration {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if rm.Predicates.IsCategoryEnumeration || rm.Predicates.HasPerMemberTable {
		return true
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return true
	}
	return sourceScopeProfileDeclaresInventorySurface(rm.SourceScopeProfile)
}

func sourceScopeProfileDeclaresInventorySurface(profile *SourceScopeProfile) bool {
	if profile == nil {
		return false
	}
	scope := profile.RequestedScope
	return scope != "" && scope != SourceScopeUnknown && scope.IsValid()
}
