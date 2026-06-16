package types

// compileRoleLookup builds the AnswerSemanticView for QFRoleLookup.
// Question shape: "what is the X for Y" where X is a role-shaped
// answer (entry function / handler / route / config key / owner
// symbol) and Y is the named clue. The answer is a SINGLE literal
// playing a role relative to the clue.
//
// Required blocks:
//
//	1× BlockSummary           — name the subject + the role result
//	1× BlockScalar            — the resolved literal (function name /
//	                            handler / config key / type / etc.) with
//	                            authoritative file:line.
//
// Optional:
//
//	0..1× BlockSection        — supporting context (call-site, owner
//	                            type, neighboring symbols when the
//	                            resolved literal alone is ambiguous).
//	0..N× BlockCaveat         — drift / absence / scope caveats.
func compileRoleLookup(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	view := &AnswerSemanticView{
		Family: QFRoleLookup,
	}
	if plan != nil {
		view.FacetCoverage = plan.FacetCoverage
		view.SummaryMode = plan.SummarySurfaceMode
	}
	if ir != nil && ir.AnswerContract.ExactResolution != nil {
		view.ExactResolution = ir.AnswerContract.ExactResolution
	}
	view.RequiredBlocks = []BlockRequirement{
		requireSummaryBlock(
			"Name the subject (the clue from the question) and the resolved role result. " +
				"Keep it tight — the scalar block below carries the literal."),
		{
			Kind:     BlockScalar,
			MinCount: 1,
			MaxCount: 1,
			Required: true,
			FacetIDs: []string{string(FacetResolvedLiteralOrSymbol)},
			AcceptableClaimForms: []ClaimForm{
				ClaimDefinitionFact,
				ClaimLiteralValueFact,
			},
			Rationale: "The single literal that plays the asked-for role (entry function name / " +
				"handler symbol / config key / owner type / agent-like component) with its authoritative file:line.",
			SurfaceRoleHint: SurfacePrincipal,
		},
	}
	view.OptionalBlocks = []BlockRequirement{
		{
			Kind:     BlockSection,
			MinCount: 0,
			MaxCount: 1,
			Required: false,
			FacetIDs: []string{string(FacetNearestMechanism), string(FacetCurrentCodePath)},
			Rationale: "Add a supporting section when the resolved literal alone is ambiguous — " +
				"surrounding type, call-site, or sibling symbols that disambiguate which role this " +
				"specific literal plays.",
		},
		optionalCaveatBlock(
			"When the role lookup hit a renamed / drifted symbol or returned absence, name the "+
				"scope and the drift so the reader can audit.",
			string(FacetUncertaintyBoundary),
		),
	}
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	return view
}
