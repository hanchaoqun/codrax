package types

// compileEnumeration builds the AnswerSemanticView for QFEnumeration.
// Question shape: "list all X of Y" / "which X have Y" / "every
// implementer / subclass / handler / config-source of Z".
//
// Required blocks:
//
//	1× BlockSummary           — describe what the list enumerates +
//	                            the terminal criterion used to pick.
//	≥1× BlockOrderedList OR BlockTable OR BlockBulletList OR BlockSection
//	                            with items — the actual enumeration.
//	                            The OR is enforced at
//	                            B4 validator level via "at least one
//	                            principal-list block" rule.
//	                            MaxCount=0 (no bucket-count assumption).
//
// Optional:
//
//	0..N× BlockSection        — when the user partitioned the answer
//	                            into named buckets (e.g. "X for A,
//	                            Y for B"), each bucket becomes its
//	                            own section.
//	0..N× BlockCaveat         — completeness / scope / typed-graph-vs-
//	                            narrative divergence caveats.
//
// The user's question MAY name buckets via QuestionStructure.Buckets;
// the count is NOT capped here (R5: 不假设最大 bucket 数).
func compileEnumeration(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	view := &AnswerSemanticView{
		Family: QFEnumeration,
	}
	acceptableClaimForms := []ClaimForm{
		ClaimDefinitionFact,
		ClaimAssignmentFact,
		ClaimReturnFact,
		ClaimImportEdge,
		ClaimRegistrationEdge,
		ClaimLiteralValueFact,
		ClaimCallEdge,
		// INODE (§28.6 ⑫, 2026-07-09): enumeration answers grounded in
		// runtime-artifact rows (trace/log statistics such as per-inode IO
		// frequency tables) legitimately carry external_observation claims.
		// AcceptableClaimForms feeds HINT surfaces only (the block-contract
		// prompt's "claim_form must be one of" list and the
		// claim-use-missing violation's repair text) — the validator accepts
		// any present claim_use regardless of form, so omitting the form
		// here misled retries toward source-shaped claims without ever
		// hard-blocking; this line fixes the hint, not the gate.
		ClaimExternalObservation,
	}
	if ir != nil && ir.RequestModel.ChangeImpactProfile != nil && ir.RequestModel.ChangeImpactProfile.Active() {
		acceptableClaimForms = append(acceptableClaimForms, ClaimGuardCondition)
		if ir.RequestModel.ChangeImpactProfile.AllowsTextReferencePrincipal() {
			acceptableClaimForms = append(acceptableClaimForms, ClaimTextReferenceFact)
		}
	}
	if plan != nil {
		view.FacetCoverage = plan.FacetCoverage
		view.SummaryMode = plan.SummarySurfaceMode
	}
	if ir != nil && ir.AnswerContract.ExactResolution != nil {
		view.ExactResolution = ir.AnswerContract.ExactResolution
	}
	principalCarrier := BlockRequirement{
		Kind:                 BlockOrderedList,
		AlternativeKinds:     []AnswerBlockKind{BlockTable, BlockBulletList, BlockSection},
		MinCount:             1,
		MaxCount:             0, // no bucket / member count assumption
		Required:             true,
		FacetIDs:             []string{string(FacetEnumerationItem)},
		AcceptableClaimForms: acceptableClaimForms,
		Rationale: "The enumeration itself. Each item names the member with its authoritative " +
			"file:line. Use an ordered_list, table, bullet_list, or section with items depending on which is clearest; " +
			"a table is preferred when members have multiple attributes. Order is alphabetic OR " +
			"meaningful (e.g. precedence) — describe which in the summary block.",
		SurfaceRoleHint: SurfacePrincipal,
	}
	// HasPerMemberTable is the analyzer's closed typed declaration that the
	// requested member payload needs a table. Keeping list/section alternatives
	// here let a rejected table be deleted while the final answer still passed
	// the structural contract. Do not infer this requirement from request or
	// answer prose: only the schema boolean narrows the carrier kind.
	if ir != nil && ir.RequestModel.Predicates.HasPerMemberTable {
		principalCarrier.Kind = BlockTable
		principalCarrier.AlternativeKinds = nil
		principalCarrier.Rationale = "The requested per-member comparison/attribute payload must remain a table with structured items. " +
			"Keep principal member identities and their grounded attributes visible; do not replace or delete the table on retry."
	}
	view.RequiredBlocks = []BlockRequirement{
		requireSummaryBlock(
			"Describe what the list enumerates and the terminal criterion used to pick the items, " +
				"so the reader understands what kind of item each row is and why these belong (and none of the others)."),
		principalCarrier,
	}
	view.OptionalBlocks = []BlockRequirement{
		{
			Kind:     BlockSection,
			MinCount: 0,
			MaxCount: 0, // no bucket-count assumption
			Required: false,
			FacetIDs: []string{string(FacetBucketLabel)},
			Rationale: "When the user partitioned the question into named buckets, each bucket is " +
				"its own section under a header matching the user's verbatim label.",
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
