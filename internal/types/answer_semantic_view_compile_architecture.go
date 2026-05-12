package types

// compileArchitecture builds the AnswerSemanticView for QFArchitecture.
// Question shape: "describe the architecture of X" / "how is X
// organised / layered" — answer is a layered description of
// components and their relations.
//
// Required blocks:
//
//	1× BlockSummary           — high-level architecture conclusion.
//	≥1× BlockSection          — one section per layer / component
//	                            group (MaxCount=0; layer count varies).
//	0..1× BlockDiagram        — optional unless the current request
//	                            explicitly asked for a visual diagram.
//
// Optional:
//
//	0..N× BlockBulletList     — within a section, an unordered list
//	                            of sub-components.
//	0..N× BlockOrderedList    — when the architecture answer walks an
//	                            ordered pipeline, dispatch, or handoff
//	                            among grounded components.
//	0..N× BlockCaveat         — out-of-scope / convention-only caveats.
func compileArchitecture(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	view := &AnswerSemanticView{
		Family: QFArchitecture,
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
			"Open with the high-level architecture conclusion: what kind of system this is, the " +
				"top-level decomposition (layers / domains / modules), and how the layers connect. " +
				"Keep it tight — the per-layer sections below carry the detail."),
		{
			Kind:     BlockSection,
			MinCount: 1,
			MaxCount: 0, // no layer-count assumption
			Required: true,
			FacetIDs: []string{
				string(FacetComponentRelation),
			},
			AcceptableClaimForms: []ClaimForm{
				ClaimDefinitionFact,
				ClaimImportEdge,
				ClaimCallEdge,
			},
			Rationale:       "One section per architectural layer / component group. The body MUST combine TWO axes: (a) WHAT the component does — its role / purpose / responsibility, expressed as an active sentence whose subject is the component and whose predicate is a behavioural verb (an action the component performs for the system, not just an internal data structure it owns); (b) WHERE the component lives — grounded file:line references supporting (a). Listing only file:line locations or only internal type names without an active behaviour sentence is incomplete; describing the role without grounded references is unverified. Each section needs both axes.",
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
				string(FacetComponentRelation),
			},
			Rationale: "A flowchart showing the components and their relations. Top-down direction " +
				"for layered architectures; left-right for pipelines.",
		})
	}
	view.OptionalBlocks = []BlockRequirement{
		{
			Kind:     BlockBulletList,
			MinCount: 0,
			MaxCount: 0,
			Required: false,
			FacetIDs: []string{string(FacetComponentRelation)},
			Rationale: "Within a layer section, a bullet list enumerating the sub-components is " +
				"often clearer than dense prose.",
		},
		{
			Kind:     BlockOrderedList,
			MinCount: 0,
			MaxCount: 0,
			Required: false,
			FacetIDs: []string{string(FacetComponentRelation), string(FacetCurrentCodePath)},
			Rationale: "When the architecture question is really about an ordered pipeline, " +
				"dispatch path, or handoff between components, preserve the grounded sequence " +
				"as an ordered list instead of compressing it into prose.",
		},
		optionalCaveatBlock(
			"When some components are mentioned by convention only (no on-disk file), or when "+
				"the architecture description is bounded to a sub-tree, disclose the scope.",
			string(FacetUncertaintyBoundary),
		),
	}
	if !diagramRequiredByUserIntent(plan) && diagramPreferredByEvidence(plan) {
		view.OptionalBlocks = append(view.OptionalBlocks, optionalDiagramBlock(
			"Add a component diagram only when it directly serves the architecture question and every "+
				"node/relation is grounded. Do not emit a diagram when prose sections answer the user's "+
				"requested comparison or explanation more directly.",
			string(FacetDiagramSpine),
			string(FacetComponentRelation),
		))
	}
	view.DiagramPlan = diagramPlanFor(plan, DiagramArchitecture,
		[]string{string(FacetComponentRelation)},
		[]string{string(FacetPrincipalPathEdge)},
		DefaultEdgeRelationsForKind(DiagramArchitecture),
	)
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	// v3 B2 (2026-05-04) — architecture answers gain materially from
	// component_relation + nearest_mechanism context when typed
	// evidence is available. Mark them glaring so uncovered state
	// promotes to ViolRichnessGlaringGap.
	markGlaringFacets(view, FacetComponentRelation, FacetNearestMechanism)
	return view
}
