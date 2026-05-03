package types

// compileArchitecture builds the AnswerSemanticView for QFArchitecture.
// Question shape: "describe the architecture of X" / "how is X
// organised / layered" — answer is a layered description of
// components and their relations.
//
// Required blocks:
//   1× BlockSummary           — high-level architecture conclusion.
//   ≥1× BlockSection          — one section per layer / component
//                               group (MaxCount=0; layer count varies).
//   ≥1× BlockDiagram          — flowchart visualising the architecture
//                               (Required=true).
//
// Optional:
//   0..N× BlockBulletList     — within a section, an unordered list
//                               of sub-components.
//   0..N× BlockCaveat         — out-of-scope / convention-only caveats.
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
				string(FacetNearestMechanism),
			},
			AcceptableClaimForms: []ClaimForm{
				ClaimDefinitionFact,
			},
			Rationale: "One section per architectural layer / component group. Each section's " +
				"title names the layer; the body describes its responsibilities + key files.",
			SurfaceRoleHint: SurfacePrincipal,
		},
		{
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
			SurfaceRoleHint: SurfaceDiagramOnly,
		},
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
			SurfaceRoleHint: SurfaceSupport,
		},
		optionalCaveatBlock(
			"When some components are mentioned by convention only (no on-disk file), or when "+
				"the architecture description is bounded to a sub-tree, disclose the scope.",
			string(FacetUncertaintyBoundary),
		),
	}
	view.DiagramPlan = diagramPlanFor(plan, DiagramArchitecture,
		[]string{string(FacetComponentRelation)},
		[]string{string(FacetPrincipalPathEdge)},
	)
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	return view
}
