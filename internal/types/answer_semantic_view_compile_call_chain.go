package types

// compileCallChain builds the AnswerSemanticView for QFCallChain.
// Question shape: "trace from X to Y" / "how does control flow from
// A to B" — answer is an ordered sequence of call/dispatch hops.
//
// Required blocks:
//
//	1× BlockSummary           — what initiates the chain, what terminal
//	                            state is reached, main guard.
//	≥1× BlockOrderedList      — the hops themselves (each item is one
//	                            function / dispatch / branch).
//	0..1× BlockDiagram        — optional unless the current request
//	                            explicitly asked for a visual diagram.
//
// Optional:
//
//	0..N× BlockCaveat         — drift / log-source caveats when an
//	                            attached log was the chain's seed.
func compileCallChain(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	view := &AnswerSemanticView{
		Family: QFCallChain,
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
			"State what initiates the chain, what terminal state it reaches, and the main guard or " +
				"branch point if there is one. Keep it short — the ordered list carries the hop detail."),
		{
			Kind:     BlockOrderedList,
			MinCount: 1,
			MaxCount: 0,
			Required: true,
			FacetIDs: []string{
				string(FacetCurrentCodePath),
				string(FacetPrincipalPathEdge),
			},
			AcceptableClaimForms: []ClaimForm{
				ClaimDefinitionFact,
				ClaimCallEdge,
			},
			Rationale: "Walk the call chain hop by hop. Each item is one function/dispatch site " +
				"with file:line; the order matches the actual control flow.",
			SurfaceRoleHint: SurfacePrincipal,
		},
	}
	if diagramRequiredByUserIntent(plan) {
		view.RequiredBlocks = append(view.RequiredBlocks, BlockRequirement{
			Kind:     BlockDiagram,
			MinCount: 1,
			MaxCount: 1,
			Required: true,
			FacetIDs: []string{
				string(FacetDiagramSpine),
				string(FacetCurrentCodePath),
			},
			Rationale: "A sequence diagram showing the chain visually — actor-to-actor edges " +
				"matching the ordered list. Use Mermaid sequenceDiagram form.",
		})
	}
	view.OptionalBlocks = []BlockRequirement{
		optionalCaveatBlock(
			"When an attached log seeded the chain, disclose drift between observed runtime "+
				"frames and current code lines.",
			string(FacetObservedArtifactFact),
		),
	}
	if !diagramRequiredByUserIntent(plan) && diagramPreferredByEvidence(plan) {
		view.OptionalBlocks = append(view.OptionalBlocks, optionalDiagramBlock(
			"Add a small sequence diagram only when it directly clarifies the call chain the user asked for "+
				"and every node/edge is grounded. Do not add a diagram as a substitute for the ordered hops.",
			string(FacetDiagramSpine),
			string(FacetCurrentCodePath),
		))
	}
	view.DiagramPlan = diagramPlanFor(plan, DiagramSequence,
		[]string{string(FacetCurrentCodePath)},
		[]string{string(FacetPrincipalPathEdge)},
		DefaultEdgeRelationsForKind(DiagramSequence),
	)
	view.UncertaintyRules = []UncertaintyRule{uncertaintyRuleForObservedArtifact()}
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	// v3 B2 — call-chain answers benefit materially from branch_guard +
	// principal_path_edge typed evidence when present.
	markGlaringFacets(view, FacetBranchGuard, FacetPrincipalPathEdge)
	return view
}
