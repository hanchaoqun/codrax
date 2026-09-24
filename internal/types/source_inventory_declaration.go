package types

// SourceInventoryDeclarationOrigin distinguishes a normalized model-declared
// source inventory from the system's ordinary enumeration-shape repair. It is
// an internal IR receipt, not a new model parameter or evidence permission.
type SourceInventoryDeclarationOrigin string

const (
	SourceInventoryDeclarationModelProvided SourceInventoryDeclarationOrigin = "model_provided"
	SourceInventoryDeclarationSynthesized   SourceInventoryDeclarationOrigin = "synthesized"
)

// SourceInventoryCurrentSourceApplicable keeps a result shape (enumeration,
// table, or repaired inventory profile) from creating a source obligation on
// an external-observation request. Ordinary source requests retain their
// existing inventory rules. This is applicability, not proof of completeness.
func SourceInventoryCurrentSourceApplicable(rm RequestModel) bool {
	return SourceInventoryCurrentSourceApplicableInContext(rm, TurnRouteHint{}, false)
}

func SourceInventoryCurrentSourceApplicableInContext(rm RequestModel, hint TurnRouteHint, attachedRuntime bool) bool {
	if attachedRuntime && !runtimeSourceRequestHasExternalObservationCarrier(&rm, hint) {
		rm = rm.withAttachedRuntimeArtifact()
	}
	if !runtimeSourceRequestHasExternalObservationCarrier(&rm, hint) {
		return true
	}
	return RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, hint) == RuntimeSourceRequirementPrecise
}

// SourceInventoryCurrentSourceApplicableFromBus uses the same current-run
// artifact receipts for dispatch and completion, including large attachments
// without a materialized PerfBundle and bundles restored beside the IR.
func SourceInventoryCurrentSourceApplicableFromBus(ctx *BusContext) bool {
	if ctx == nil {
		return true
	}
	rm := RuntimeSourceAuthorityRequestModelFromBusContext(ctx)
	if rm == nil {
		return !RuntimeArtifactContextActiveFromBus(ctx)
	}
	return SourceInventoryCurrentSourceApplicableInContext(*rm, ctx.TurnRouteHint, RuntimeArtifactContextActiveFromBus(ctx))
}

// SourceInventoryRequestModelFromBusContext is a detached policy view for
// source-inventory consumers whose persisted IR omits runtime bundles. The
// context-only carrier is not published to the IR, observations, or evidence.
func SourceInventoryRequestModelFromBusContext(ctx *BusContext) *RequestModel {
	rm := RuntimeSourceAuthorityRequestModelFromBusContext(ctx)
	if rm != nil && (RuntimeArtifactContextActiveFromBus(ctx) || ctx.TurnRouteHint.ExternalObservationParticipates()) {
		view := rm.withAttachedRuntimeArtifact()
		return &view
	}
	return rm
}

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
