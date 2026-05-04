package types

// compileRootCauseTrace builds the AnswerSemanticView for QFRootCauseTrace.
// Question shape: "why does X happen / panic / fail" — typically with
// an attached log or perf trace observing the failure event.
//
// Required blocks:
//   1× BlockSummary           — core conclusion (cause + observed site)
//   ≥1× BlockOrderedList      — principal cause chain (innermost frame
//                               outward; each hop is a step). MaxCount=0
//                               (no upper-bound assumption — chains can
//                               be 2 hops or 12 hops; the user's
//                               question dictates the depth).
//   0..N× BlockDiagram        — sequence diagram for the cause chain
//                               when DiagramHint resolves to one.
//   0..N× BlockCaveat         — drift / log-source / external-source
//                               disclosures.
//
// Diagram plan: sequence diagram with FacetCurrentCodePath as nodes
// and FacetPrincipalPathEdge as edges (the cause chain is a sequence
// of file:line transitions).
//
// Uncertainty rules: when log-source observed (FacetObservedArtifactFact),
// require a caveat block disclosing the drift between observed runtime
// and current code.
func compileRootCauseTrace(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	view := &AnswerSemanticView{
		Family: QFRootCauseTrace,
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
			"Open with the core conclusion: state the failure mode and the load-bearing line " +
				"where the cause originates. Keep it tight — the ordered cause chain below carries the hop-by-hop detail."),
		{
			Kind:     BlockOrderedList,
			MinCount: 1,
			MaxCount: 0, // no upper-bound; chain depth varies per question
			Required: true,
			FacetIDs: []string{
				string(FacetCurrentCodePath),
				string(FacetPrincipalPathEdge),
			},
			AcceptableClaimForms: []ClaimForm{
				ClaimDefinitionFact,
				ClaimCallEdge,
			},
			Rationale: "Walk the principal cause chain from the innermost failing frame outward. " +
				"Each list item is one hop with file:line; the order matches the call/causation sequence.",
			SurfaceRoleHint: SurfacePrincipal,
		},
	}
	view.OptionalBlocks = []BlockRequirement{
		{
			Kind:     BlockDiagram,
			MinCount: 0,
			MaxCount: 1,
			Required: false,
			FacetIDs: []string{
				string(FacetDiagramSpine),
				string(FacetCurrentCodePath),
			},
			Rationale: "When the cause chain crosses ≥3 hops, a sequence diagram makes the order " +
				"visually obvious. Each node is a function/site; each edge is a call/causation step.",
			SurfaceRoleHint: SurfaceDiagramOnly,
		},
		optionalCaveatBlock(
			"When the observed event came from an attached log/perf trace, name which file:line "+
				"are runtime observations vs current source you read; this is the drift caveat.",
			string(FacetObservedArtifactFact),
			string(FacetUncertaintyBoundary),
		),
	}
	view.DiagramPlan = diagramPlanFor(plan, DiagramSequence,
		[]string{string(FacetCurrentCodePath)},
		[]string{string(FacetPrincipalPathEdge)},
		append(DefaultEdgeRelationsForKind(DiagramSequence),
			DiagramEdgeRelationContract{
				Kind: DiagramRelObserve, Min: 0, ClaimForm: ClaimExternalObservation,
			},
		),
	)
	view.UncertaintyRules = []UncertaintyRule{uncertaintyRuleForObservedArtifact()}
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	return view
}
