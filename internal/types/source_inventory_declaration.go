package types

// SourceInventoryDeclarationOrigin distinguishes a normalized model-declared
// source inventory from the system's ordinary enumeration-shape repair. It is
// an internal IR receipt, not a new model parameter or evidence permission.
type SourceInventoryDeclarationOrigin string

const (
	SourceInventoryDeclarationModelProvided SourceInventoryDeclarationOrigin = "model_provided"
	SourceInventoryDeclarationSynthesized   SourceInventoryDeclarationOrigin = "synthesized"
)

// SourceInventoryHasDeclaredNavigationRequest is deliberately weaker than a
// source obligation. A normalized model declaration may navigate an optional
// source lane; it cannot require a read or claim a complete source universe.
func SourceInventoryHasDeclaredNavigationRequest(rm RequestModel) bool {
	profile := rm.SourceInventoryProfile
	return profile != nil && profile.Active() &&
		profile.DeclarationOrigin == SourceInventoryDeclarationModelProvided &&
		len(profile.SourceQuotes) > 0 &&
		!(rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource()) &&
		!sourceInventoryCompletionSupportBoundary(rm) &&
		!SourceInventoryLaneConflictsWithRelationFlow(rm)
}

// This request-only predicate must not call the applicability/authority
// compiler: that compiler consumes it. Declaration provenance is necessary,
// not sufficient. Precision comes from existing structural source facets,
// never a confidence score, rationale, or words inside the quote.
func sourceInventoryHasIndependentCurrentSourceRequirement(rm RequestModel) bool {
	if !SourceInventoryHasDeclaredNavigationRequest(rm) {
		return false
	}
	profile := rm.SourceInventoryProfile
	return sourceInventoryHasExplicitTypedScope(rm.SourceScopeProfile) ||
		profile.RequiresConstSet ||
		(profile.TypeUnderlying != "" && profile.TypeUnderlying != SourceInventoryTypeUnderlyingUnknown) ||
		(rm.CompletenessObligation.IsActive() && profile.MechanicalRowsOnly())
}

func CloneSourceInventoryProfile(in *SourceInventoryProfile) *SourceInventoryProfile {
	if in == nil {
		return nil
	}
	out := *in
	out.TargetRoles = append([]AnswerCandidateRole(nil), in.TargetRoles...)
	out.RequestedFields = append([]SourceInventoryRequestedField(nil), in.RequestedFields...)
	out.SourceQuotes = append([]string(nil), in.SourceQuotes...)
	return &out
}
