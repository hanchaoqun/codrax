package types

// compileConfigPrecedence builds the AnswerSemanticView for
// QFConfigPrecedence. Question shape: "what value does config key K
// take / where is K configured / which layer overrides which".
//
// Required blocks:
//
//	1× BlockSummary
//	1× BlockScalar                         for a typed scalar lookup, OR
//	1× BlockTable | BlockOrderedList       for a typed config mapping
//
// The distinction comes only from RequestPredicates.IsScalarAnswer. A
// repository can prove that a runtime override exists without proving its
// current process value, so a non-scalar precedence question must not be
// coerced into publishing one "effective" literal.
//
// Optional:
//
//	0..N× BlockOrderedList    — when precedence has clear linear
//	                            ordering, render as ordered list of
//	                            layer-source pairs.
//	0..N× BlockCaveat         — absence-scope / external-tool caveats.
//
// Generalisation note: the precedence layers MAY come from any config
// system — yaml, JSON, TOML, INI, .env, shell rc, command-line flags,
// programmatic defaults. The compile rules do NOT assume the file
// extension. (R5: 不耦合 yaml.)
func compileConfigPrecedence(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	view := &AnswerSemanticView{
		Family: QFConfigPrecedence,
	}
	if plan != nil {
		view.FacetCoverage = plan.FacetCoverage
		view.SummaryMode = plan.SummarySurfaceMode
		// Missing requested roles describe a proved exact-absence surface,
		// not an unobserved live value on an otherwise present precedence
		// layer. Keeping this carrier absence-only prevents a runtime/env
		// source from being rendered as a missing user-requested layer merely
		// because the current process binding was not observed.
		if ir != nil && plan.PreferredExactResolution != nil &&
			plan.PreferredExactResolution.Status == AnswerExactResolutionAbsent {
			view.MissingRequestedRoles = ConfigTraceMissingRequestedRoleDisclosures(
				ir.RequestModel,
				plan.ExactResolution,
				plan.ExactContextRequiredFiles,
				plan.SurfaceEvidence,
			)
		}
	}
	// The analysis-level exact contract remains available to exploration for
	// target focus and absence proof. It becomes a user-visible finalizer
	// contract only when exact disposition is itself the answer: a scalar
	// lookup, or a proved absence. A positive non-scalar mapping already names
	// and explains its setting in model-authored blocks; forcing an additional
	// exact_match carrier would let the renderer prepend a redundant system
	// conclusion and can turn an alias-shaped search term into false authority.
	if ir != nil && ir.AnswerContract.ExactResolution != nil {
		view.ExactResolution = ir.AnswerContract.ExactResolution
		view.SuppressExactResolutionAnswerSurface = !configPrecedenceNeedsVisibleExactResolution(ir, plan)
	}
	view.RequiredBlocks = []BlockRequirement{}
	view.OptionalBlocks = []BlockRequirement{}
	if configPrecedenceIsScalarLookup(ir) {
		view.RequiredBlocks = append(view.RequiredBlocks,
			requireSummaryBlock(
				"Open with the resolved value (or absence finding) and name the configuration key. "+
					"Disambiguate which source and layer prove that value. A source-code runtime override proves the lookup mechanism, not the current process value; disclose that boundary instead of assuming the override is unset."),
			BlockRequirement{
				Kind:     BlockScalar,
				MinCount: 1,
				MaxCount: 1,
				Required: true,
				FacetIDs: []string{string(FacetResolvedLiteralOrSymbol)},
				AcceptableClaimForms: []ClaimForm{
					ClaimDefinitionFact,
					ClaimAssignmentFact,
					ClaimLiteralValueFact,
				},
				Rationale: "Emit the resolved scalar value (or boolean / literal) with its authoritative " +
					"file:line citation. When the value is genuinely absent, set status=absent on the " +
					"exact-resolution contract instead of fabricating a literal.",
				SurfaceRoleHint: SurfacePrincipal,
			})
		view.OptionalBlocks = append(view.OptionalBlocks, configPrecedenceLayerCarrier(false))
	} else {
		view.RequiredBlocks = append(view.RequiredBlocks,
			requireSummaryBlock(
				"Open with how the configuration is resolved and name the configuration key or setting. "+
					"Keep configured defaults separate from runtime state: a runtime override source proves precedence, but does not prove the current process value."),
			configPrecedenceLayerCarrier(true),
		)
	}
	view.OptionalBlocks = append(view.OptionalBlocks,
		optionalCaveatBlock(
			"When the resolved value comes from a fallback / default / absence-scope finding, "+
				"emit a caveat naming the search scope so the reader can audit what was searched.",
			string(FacetUncertaintyBoundary),
		),
	)
	// Config precedence is table/list-led by family default. A user-explicit
	// DiagramContract is handled later by the family-independent presentation
	// contract so this compiler does not need a diagram-specific branch.
	if configPrecedenceRequiresUncertaintyDisclosure(view, plan) {
		view.UncertaintyRules = []UncertaintyRule{{
			TriggerFacet:      "",
			ExpectedBlockKind: BlockCaveat,
			MissingMessage: "Your answer claims an absence or fallback resolution; emit at least one " +
				"caveat block naming the exact search scope (file glob / repo-wide / per-package) " +
				"so the reader can reproduce the absence finding.",
		}}
	}
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	// v3 B2 — config-precedence answers benefit from explicit
	// precedence-role context when typed evidence supports it.
	markGlaringFacets(view, FacetConfigPrecedenceRole)
	return view
}

func configPrecedenceRequiresUncertaintyDisclosure(view *AnswerSemanticView, plan *AnswerSurfacePlan) bool {
	if view != nil && len(view.MissingRequestedRoles) > 0 {
		return true
	}
	return plan != nil && plan.PreferredExactResolution != nil &&
		plan.PreferredExactResolution.Status == AnswerExactResolutionAbsent
}

func configPrecedenceIsScalarLookup(ir *AnalysisIR) bool {
	return ir != nil && ir.RequestModel.Predicates.IsScalarAnswer
}

func configPrecedenceNeedsVisibleExactResolution(ir *AnalysisIR, plan *AnswerSurfacePlan) bool {
	if ir == nil || ir.AnswerContract.ExactResolution == nil {
		return false
	}
	if configPrecedenceIsScalarLookup(ir) {
		return true
	}
	return plan != nil && plan.PreferredExactResolution != nil &&
		plan.PreferredExactResolution.Status == AnswerExactResolutionAbsent
}

func configPrecedenceLayerCarrier(required bool) BlockRequirement {
	minCount := 0
	if required {
		minCount = 1
	}
	return BlockRequirement{
		Kind:             BlockTable,
		AlternativeKinds: []AnswerBlockKind{BlockOrderedList},
		MinCount:         minCount,
		MaxCount:         1,
		Required:         required,
		FacetIDs:         []string{string(FacetConfigPrecedenceRole)},
		Rationale: "Render each grounded precedence layer once, from low to high, with its source and configured value when known. " +
			"Use table rows or ordered-list items with citation_ref values so every source remains auditable. " +
			"For a runtime/CLI/environment layer whose live value was not observed, report the source and state that the runtime value is unknown; do not treat repository absence as proof that the layer is unset.",
		SurfaceRoleHint: SurfacePrincipal,
	}
}
