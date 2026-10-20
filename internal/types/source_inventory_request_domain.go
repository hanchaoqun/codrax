package types

// SourceInventoryCurrentSourceApplicable keeps a result shape (enumeration,
// table, or repaired inventory profile) from creating a source obligation on
// an external-observation request. Ordinary source requests retain their
// existing inventory rules. This is applicability, not proof of completeness.
func SourceInventoryCurrentSourceApplicable(rm RequestModel) bool {
	return SourceInventoryCurrentSourceApplicableInContext(rm, TurnRouteHint{}, false)
}

func SourceInventoryCurrentSourceApplicableInContext(rm RequestModel, hint TurnRouteHint, attachedRuntime bool) bool {
	if attachedRuntime {
		rm.runtimeMaterialDomain = true
	}
	if !runtimeSourceRequestHasExternalObservationCarrier(&rm, hint) {
		return true
	}
	// A source-location attribute or a separately required source read does
	// not establish the principal source-member universe. An inactive declaration
	// (including absence) or a synthesized profile cannot gain one via a consumer.
	// Explicit model declarations and unknown legacy provenance retain their
	// existing independent-source precision checks below.
	if !rm.SourceInventoryProfile.Active() ||
		rm.SourceInventoryProfile.DeclarationOrigin == SourceInventoryDeclarationSynthesized {
		return false
	}
	// Source permission cannot certify its own inventory obligation through a
	// generic scope. Route-synthesized allow plus an unquoted all/production
	// scope otherwise turns a runtime record roster into source declarations.
	// Probe independent typed source authority without changing permissions,
	// route obligations, explicit inventory declarations, or accepted evidence.
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.CurrentSourceMode == ExternalObservationCurrentSourceAllow {
		policy := *rm.ExternalObservationPolicy
		policy.CurrentSourceMode = ExternalObservationCurrentSourceDefault
		rm.ExternalObservationPolicy = &policy
	}
	return RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, hint) == RuntimeSourceRequirementPrecise
}

// SourceInventoryCanSynthesizePrincipalProfile bounds the ordinary source-only
// enumeration repair. In a runtime/mixed request, the members may be events,
// captures, samples or measurements even when a required source location is an
// attribute. Only a separate model-provided SourceInventoryProfile can name a
// principal source-member universe there; neither a table nor source precision
// grants permission to invent function/method/type roles. This does not revoke
// any independent current-source obligation or disable ordinary source tools.
func SourceInventoryCanSynthesizePrincipalProfile(rm RequestModel, hint TurnRouteHint, attachedRuntime bool) bool {
	if attachedRuntime {
		rm.runtimeMaterialDomain = true
	}
	return !runtimeSourceRequestHasExternalObservationCarrier(&rm, hint)
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
		rm.runtimeMaterialDomain = true
	}
	return rm
}
