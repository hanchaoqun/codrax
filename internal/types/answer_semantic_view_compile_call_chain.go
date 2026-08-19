package types

// CallChainPrincipalClaimForms is the single claim-form roster for the
// principal structured call-chain carrier. Keep the compiler and emit-time
// relation-presence gate on this one typed source so a newly supported
// language or relation shape cannot update one side while leaving the other
// stale.
func CallChainPrincipalClaimForms() []ClaimForm {
	return []ClaimForm{
		ClaimDefinitionFact,
		ClaimCallEdge,
		ClaimCallbackHandoff,
		ClaimRegistrationEdge,
	}
}

// IsCallChainPrincipalRelationClaimForm reports whether a principal
// call-chain claim form asserts a directed relation and therefore needs a
// model-authored endpoint anchor. Definition facts remain descriptive and do
// not enter this relation contract.
func IsCallChainPrincipalRelationClaimForm(form ClaimForm) bool {
	for _, candidate := range CallChainPrincipalClaimForms() {
		if candidate == form {
			return RelationForClaimForm(form).IsValid()
		}
	}
	return false
}

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
	principalListRationale := "List only grounded directed hops. Within each connected typed segment, order items by the proved control flow and include file:line in the list citation. " +
		"When evidence forms disconnected segments, keep them visibly separate and do not imply an execution order, value handoff, or bridge between them."
	if callChainHasRequiredMemberRoster(ir) {
		principalListRationale += " The user also requested a visible member roster. That descriptive member_set is a separate block responsibility: do not put member names/responsibilities in this principal_path_edge carrier unless each row is itself one exact grounded endpoint edge. Emit a sibling member_set block without principal_path_edge or directed claim ownership for ordinary key-function/member rows."
	}
	view.RequiredBlocks = []BlockRequirement{
		requireSummaryBlock(
			"Summarize the grounded directed segment or segments and their verified entry/terminal points. " +
				"If typed evidence does not bridge two segments, state that boundary instead of presenting one end-to-end chain. " +
				"Keep it short — the ordered list carries the relation detail."),
		{
			Kind:     BlockOrderedList,
			MinCount: 1,
			MaxCount: 0,
			Required: true,
			FacetIDs: []string{
				string(FacetCurrentCodePath),
				string(FacetPrincipalPathEdge),
			},
			AcceptableClaimForms: CallChainPrincipalClaimForms(),
			Rationale:            principalListRationale,
			SurfaceRoleHint:      SurfacePrincipal,
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
			Rationale: diagramRequirementRationale(plan, DiagramSequence,
				"A sequence diagram showing the chain visually — actor-to-actor edges "+
					"matching the ordered list. Use Mermaid sequenceDiagram form. Keep source file:line citations in the ordered list; diagram messages should describe the relation without duplicating source positions."),
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
		defaultEdgeRelationsForPlan(plan, DiagramSequence),
	)
	view.UncertaintyRules = []UncertaintyRule{uncertaintyRuleForObservedArtifact()}
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	// v3 B2 — call-chain answers benefit materially from branch_guard +
	// principal_path_edge typed evidence when present.
	markGlaringFacets(view, FacetBranchGuard, FacetPrincipalPathEdge)
	return view
}

func callChainHasRequiredMemberRoster(ir *AnalysisIR) bool {
	if ir == nil || ir.RequestModel.RequestedAnswerDimensions == nil {
		return false
	}
	for _, dimension := range ir.RequestModel.RequestedAnswerDimensions.Dimensions {
		if dimension.Required && dimension.Role == RequestedAnswerDimensionMemberSet {
			return true
		}
	}
	return false
}
