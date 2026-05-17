package types

// compileConfigPrecedence builds the AnswerSemanticView for
// QFConfigPrecedence. Question shape: "what value does config key K
// take / where is K configured / which layer overrides which".
//
// Required blocks:
//
//	1× BlockSummary           — what value, where it lands
//	1× BlockScalar OR BlockTable — final resolved value (Scalar) OR
//	                            layer-by-key precedence (Table). The
//	                            OR is expressed as MinCount=1 on each
//	                            + Required=true on at least one;
//	                            B4 validator enforces "at least one
//	                            of {scalar, table}" in
//	                            validatePrincipalClaimUse.
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
		view.MissingRequestedRoles = ConfigTraceMissingRequestedRoleDisclosures(
			ir.RequestModel,
			plan.ExactResolution,
			plan.ExactContextRequiredFiles,
			plan.SurfaceEvidence,
		)
	}
	if ir != nil && ir.AnswerContract.ExactResolution != nil {
		view.ExactResolution = ir.AnswerContract.ExactResolution
	}
	view.RequiredBlocks = []BlockRequirement{
		requireSummaryBlock(
			"Open with the resolved value (or absence finding) and name the configuration key. " +
				"Disambiguate: which file:line is the source of the resolved value when multiple layers exist."),
		{
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
		},
	}
	view.OptionalBlocks = []BlockRequirement{
		{
			Kind:     BlockTable,
			MinCount: 0,
			MaxCount: 1,
			Required: false,
			FacetIDs: []string{string(FacetConfigPrecedenceRole)},
			Rationale: "When the value is determined by precedence across multiple layers, render " +
				"a table whose columns are layer / source-file:line / value / overrides? — one row " +
				"per layer in precedence order.",
			SurfaceRoleHint: SurfacePrincipal,
		},
		{
			Kind:     BlockOrderedList,
			MinCount: 0,
			MaxCount: 0,
			Required: false,
			FacetIDs: []string{string(FacetConfigPrecedenceRole)},
			Rationale: "Alternatively, when the precedence forms a clear linear chain (no per-key " +
				"split), an ordered list of layer-source pairs reads more naturally than a table.",
		},
		optionalCaveatBlock(
			"When the resolved value comes from a fallback / default / absence-scope finding, "+
				"emit a caveat naming the search scope so the reader can audit what was searched.",
			string(FacetUncertaintyBoundary),
		),
	}
	// Config precedence is table/list-led by family default. A user-explicit
	// DiagramContract is handled later by the family-independent presentation
	// contract so this compiler does not need a diagram-specific branch.
	view.UncertaintyRules = []UncertaintyRule{
		{
			TriggerFacet:      string(FacetUncertaintyBoundary),
			ExpectedBlockKind: BlockCaveat,
			MissingMessage: "Your answer claims an absence or fallback resolution; emit at least one " +
				"caveat block naming the exact search scope (file glob / repo-wide / per-package) " +
				"so the reader can reproduce the absence finding.",
		},
	}
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	// v3 B2 — config-precedence answers benefit from explicit
	// precedence-role context when typed evidence supports it.
	markGlaringFacets(view, FacetConfigPrecedenceRole)
	return view
}
