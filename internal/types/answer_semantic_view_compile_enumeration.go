package types

// compileEnumeration builds the AnswerSemanticView for QFEnumeration.
// Question shape: "list all X of Y" / "which X have Y" / "every
// implementer / subclass / handler / config-source of Z".
//
// Required blocks:
//   1× BlockSummary           — describe what the list enumerates +
//                               the terminal criterion used to pick.
//   ≥1× BlockOrderedList OR BlockTable OR BlockBulletList — the actual
//                               enumeration. The OR is enforced at
//                               B4 validator level via "at least one
//                               principal-list block" rule.
//                               MaxCount=0 (no bucket-count assumption).
//
// Optional:
//   0..N× BlockSection        — when the user partitioned the answer
//                               into named buckets (e.g. "X for A,
//                               Y for B"), each bucket becomes its
//                               own section.
//   0..N× BlockCaveat         — completeness / scope / typed-graph-vs-
//                               narrative divergence caveats.
//
// The user's question MAY name buckets via QuestionStructure.Buckets;
// the count is NOT capped here (R5: 不假设最大 bucket 数).
func compileEnumeration(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	view := &AnswerSemanticView{
		Family: QFEnumeration,
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
			"Describe what the list enumerates and the terminal criterion used to pick the items, " +
				"so the reader understands what kind of item each row is and why these belong (and none of the others)."),
		{
			Kind:     BlockOrderedList,
			MinCount: 1,
			MaxCount: 0, // no bucket / member count assumption
			Required: true,
			FacetIDs: []string{string(FacetEnumerationItem)},
			AcceptableClaimForms: []ClaimForm{
				ClaimDefinitionFact,
			},
			Rationale: "The enumeration itself. Each item names the member with its authoritative " +
				"file:line. Order is alphabetic OR meaningful (e.g. precedence) — describe which in " +
				"the summary block.",
			SurfaceRoleHint: SurfacePrincipal,
		},
	}
	view.OptionalBlocks = []BlockRequirement{
		{
			Kind:     BlockTable,
			MinCount: 0,
			MaxCount: 1,
			Required: false,
			FacetIDs: []string{string(FacetEnumerationItem)},
			Rationale: "When each enumerated member has multiple attributes worth surfacing " +
				"(e.g. role + file + role-rationale), a table reads better than an ordered list.",
			SurfaceRoleHint: SurfacePrincipal,
		},
		{
			Kind:     BlockSection,
			MinCount: 0,
			MaxCount: 0, // no bucket-count assumption
			Required: false,
			FacetIDs: []string{string(FacetBucketLabel)},
			Rationale: "When the user partitioned the question into named buckets, each bucket is " +
				"its own section under a header matching the user's verbatim label.",
			SurfaceRoleHint: SurfaceSupport,
		},
		optionalCaveatBlock(
			"When the typed graph reports more members than your enumeration includes, OR when "+
				"a comment / docstring contradicts the structural membership, disclose the divergence.",
			string(FacetUncertaintyBoundary),
		),
	}
	view.UncertaintyRules = []UncertaintyRule{
		{
			TriggerFacet:      string(FacetUncertaintyBoundary),
			ExpectedBlockKind: BlockCaveat,
			MissingMessage: "Your enumeration's completeness is bounded (lower_bound or unknown), " +
				"OR the typed graph and source-author intent diverge for some members; emit a caveat " +
				"block disclosing the bound and any structural-vs-narrative divergence.",
		},
	}
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	return view
}
