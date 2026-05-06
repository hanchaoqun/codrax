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
	mechanismStrength := rootCauseMechanismSupportStrength(plan)
	if ir != nil && ir.AnswerContract.ExactResolution != nil {
		view.ExactResolution = ir.AnswerContract.ExactResolution
	}
	view.RequiredBlocks = []BlockRequirement{
		requireSummaryBlock(rootCauseTraceSummaryRationale(plan)),
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
			Rationale:       rootCauseTraceOrderedListRationale(plan),
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
	// v3 B2 — root-cause-trace answers benefit from branch_guard +
	// nearest_mechanism context to keep the explanation reproducible.
	markGlaringFacets(view, FacetBranchGuard, FacetNearestMechanism)
	if mechanismStrength != rootCauseMechanismStrong {
		demoteFacetToOptional(view, FacetNearestMechanism)
	}
	return view
}

func rootCauseTraceSummaryRationale(plan *AnswerSurfacePlan) string {
	if plan != nil && plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause {
		return "Open with what the attached runtime artifact observed and the nearest grounded current-code path/mechanism available in the current checkout. " +
			"Keep the lead conclusion bounded: current cited code can prove what the code contains today, but it may not prove the exact internal runtime branch taken by the older build. " +
			"Name the strongest supported current-code mechanism without upgrading it into a stronger historical path claim. " +
			"If the current checkout only exposes a lone precondition guard (and no grounded companion statement deeper in the same path), describe that as the closest current boundary rather than as the crash mechanism itself."
	}
	return "Open with the core conclusion: state the failure mode and the load-bearing line " +
		"where the cause originates. Keep it tight — the ordered cause chain below carries the hop-by-hop detail."
}

func rootCauseTraceOrderedListRationale(plan *AnswerSurfacePlan) string {
	if plan != nil && plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause {
		return "Walk the grounded current-code path from the innermost cited frame outward. " +
			"Each list item is one current-code hop or mechanism anchor with file:line. " +
			"Do not convert a current guard / downstream dereference pair into a claimed runtime branch unless the observed artifact or a cited current line explicitly proves that path was taken. " +
			"When the closest grounded current fact is only an entry guard, keep it as a boundary note rather than promoting it into the principal crash step."
	}
	return "Walk the principal cause chain from the innermost failing frame outward. " +
		"Each list item is one hop with file:line; the order matches the call/causation sequence."
}
