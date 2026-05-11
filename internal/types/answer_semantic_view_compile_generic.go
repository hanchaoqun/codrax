package types

// compileGeneric builds the AnswerSemanticView for QFGeneric — the
// fall-through family for questions that don't fit any of the six
// shape-specific templates. Question shapes here include: free-form
// "explain how X works" / "what does Y do" / mixed multi-topic
// explanations.
//
// Required blocks:
//
//	1× BlockSummary           — the answer's main body. For Generic
//	                            family the Summary is often longer
//	                            than other families because the
//	                            explanation lives there directly.
//
// Optional:
//
//	0..N× BlockSection        — sub-headed body sections (when
//	                            multiple topics).
//	0..N× BlockOrderedList    — when the explanation walks a sequence.
//	0..N× BlockBulletList     — when listing parallel items.
//	0..N× BlockDiagram        — only when control flow / dispatch /
//	                            architecture is part of the answer.
//	0..N× BlockCaveat         — out-of-scope / convention-only caveats.
//
// QFGeneric intentionally has no BlockScalar / BlockDecision / BlockTable
// in the optional set — those are reserved for the family-specific
// templates. If a Generic answer surfaces a scalar, the LLM should
// embed it in summary prose rather than pretending it's a scalar
// answer (otherwise the question would not have been classified as
// Generic).
func compileGeneric(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	view := &AnswerSemanticView{
		Family: QFGeneric,
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
			"Write a thorough explanation that fully addresses the user's question. Length matches " +
				"what the answer needs — a shallow question yields a short answer, a deep one yields " +
				"a deep answer. Open with the core conclusion as the first paragraph; structure " +
				"with sub-headed sections when covering multiple topics."),
	}
	view.OptionalBlocks = []BlockRequirement{
		{
			Kind:     BlockSection,
			MinCount: 0,
			MaxCount: 0,
			Required: false,
			Rationale: "When the answer covers multiple topics, structure with sub-headed sections " +
				"so the reader can navigate. One section per major topic.",
		},
		{
			Kind:     BlockOrderedList,
			MinCount: 0,
			MaxCount: 0,
			Required: false,
			Rationale: "When the explanation walks a sequence (steps in a process, hops in a flow), " +
				"an ordered list inside a section reads better than dense prose.",
		},
		{
			Kind:     BlockBulletList,
			MinCount: 0,
			MaxCount: 0,
			Required: false,
			Rationale: "When listing parallel items (features, options, alternatives), a bullet list " +
				"is clearer than a comma-separated paragraph.",
		},
		{
			Kind:     BlockDiagram,
			MinCount: 0,
			MaxCount: 1,
			Required: false,
			Rationale: "Add a diagram only when the current question asks for a visual / structural " +
				"walkthrough or when grounded evidence has a relationship shape that prose would obscure. " +
				"Do not add a diagram as a generic enrichment when it would distract from the user's requested answer.",
		},
		optionalCaveatBlock(
			"When the explanation is bounded to a sub-tree, or when external/log evidence "+
				"contributes, disclose the scope so the reader knows what was searched.",
			string(FacetUncertaintyBoundary),
		),
	}
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	return view
}
