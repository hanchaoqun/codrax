package types

// EffectiveRequestRouteHint projects pre-analysis routing through the accepted
// request contract. Callers must pass the accepted RequestModel (nil before
// emit_analysis succeeds). It returns a value copy: execution access, write
// policy, the original routing record, and evidence permissions never change.
//
// An explicit negative source-explanation declaration can refine only the soft
// source requirement derived from that routing outcome on an external turn.
// Omission retains legacy routing; independent source roles, bindings and
// verification requests win over a contradictory negative declaration. This is
// not exclude_current_source, an evidence waiver, or proof of answer readiness.
func EffectiveRequestRouteHint(rm *RequestModel, hint TurnRouteHint) TurnRouteHint {
	if rm == nil || rm.CurrentSourceExplanationProfile == nil ||
		rm.CurrentSourceExplanationProfile.IsCurrentSourceExplanationRequested ||
		!hint.ExternalObservationParticipates() ||
		!hint.RequiredOutcomes.Has(TurnOutcomeSourceExplanation) ||
		hint.RequiredOutcomes.Has(TurnOutcomeSourceChange) || hint.WriteIntent != "" ||
		requestHasIndependentCurrentSourceObligation(rm) {
		return hint
	}
	hint.RequiredOutcomes &^= TurnOutcomeSourceExplanation
	hint.CurrentSourceEvidenceMode = TurnRouteCurrentSourceEvidenceOptional
	return hint
}

// This is a refinement veto, not a new hard evidence gate. In particular a
// bare current_key_code role retains its existing soft status; only the shared
// authority compiler may decide that a source requirement is precise.
func requestHasIndependentCurrentSourceObligation(rm *RequestModel) bool {
	// A route-synthesized allow policy cannot vouch for its own independence
	// through a default source scope. Inspect the same request without that
	// permission hint; retain the original policy untouched for all consumers.
	probe := *rm
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.CurrentSourceMode == ExternalObservationCurrentSourceAllow {
		policy := *rm.ExternalObservationPolicy
		policy.CurrentSourceMode = ExternalObservationCurrentSourceDefault
		probe.ExternalObservationPolicy = &policy
	}
	rm = &probe
	if rm.CurrentSourceExplanationProfile.Active() ||
		runtimeSourceAuthorityPreciseCurrentSourceRequirement(rm) ||
		rm.HasRuntimeArtifactCurrentVerificationAnchor() ||
		rm.HasCurrentSourceObligationSignal() || rm.ChangeImpactProfile.Active() ||
		rm.FieldValueProfile.Active() || rm.CallChainEndpointProfile.Active() || rm.PredicateAxis == AxisImplement {
		return true
	}
	for _, target := range rm.AnalyzerHints.ExactTargets {
		if textHasPreciseCurrentSourceAnchor(target) {
			return true
		}
	}
	for _, path := range rm.UserPinnedFiles {
		if !LooksLikeRuntimeArtifactPath(path) && CanonicalRequestedDimensionSource(path) != "" {
			return true
		}
	}
	if rm.RequestedAnswerDimensions == nil || !rm.RequestedAnswerDimensions.Active() {
		return false
	}
	for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
		if !dim.Required {
			continue
		}
		switch dim.Role {
		case RequestedAnswerDimensionCurrentKeyCode, RequestedAnswerDimensionSourceLocation, RequestedAnswerDimensionSourceAttribute:
			return true
		}
		for _, file := range rm.AnalyzerHints.RequiredFileHints {
			if CanonicalRequestedDimensionSource(file.Path) == "" {
				continue
			}
			for _, index := range file.RequestedDimensionIndices {
				if index == dim.Index {
					return true
				}
			}
		}
	}
	return false
}
