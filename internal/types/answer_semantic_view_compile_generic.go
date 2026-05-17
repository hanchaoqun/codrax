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
//	0..N× BlockTable          — when a matrix / comparison / compact
//	                            evidence view is clearer than prose.
//	0..N× BlockScalar         — when a generic explanation still needs
//	                            to preserve a single resolved value.
//	0..N× BlockDecision       — when a concise verdict helps, without
//	                            replacing the explanatory body.
//	0..N× BlockDiagram        — only when control flow / dispatch /
//	                            architecture is part of the answer.
//	0..N× BlockCaveat         — out-of-scope / convention-only caveats.
//
// QFGeneric keeps only Summary required. Presentation blocks stay
// optional so the finalizer can preserve a model-authored table,
// literal, or concise verdict without having to misclassify the
// user's broader question as a specialized family.
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
			Kind:     BlockTable,
			MinCount: 0,
			MaxCount: 0,
			Required: false,
			Rationale: "When the model has a comparison matrix, option grid, evidence ledger, or other " +
				"multi-column surface, preserve it as a table. Raw markdown table text and structured rows are both acceptable.",
		},
		{
			Kind:     BlockScalar,
			MinCount: 0,
			MaxCount: 0,
			Required: false,
			Rationale: "When the broad answer includes one resolved literal, path, count, or symbol, it may be surfaced " +
				"as a scalar block instead of burying the value in prose.",
		},
		{
			Kind:     BlockDecision,
			MinCount: 0,
			MaxCount: 0,
			Required: false,
			Rationale: "When a concise verdict improves readability, include a decision block as a presentation aid; " +
				"do not replace the required explanatory summary.",
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
	if diagramRequiredByUserIntent(plan) {
		view.RequiredBlocks = append(view.RequiredBlocks, BlockRequirement{
			Kind:     BlockDiagram,
			MinCount: 1,
			MaxCount: 1,
			Required: true,
			Rationale: "The user explicitly asked for a diagram. Include one diagram block when you can ground the labels; " +
				"if strict grounding is incomplete, keep the prose answer honest and let the renderer preserve any recovered raw diagram.",
		})
	}
	if plan != nil && plan.Diagram != nil {
		view.DiagramPlan = diagramPlanFor(plan, genericDiagramKind(plan), nil, nil,
			DefaultEdgeRelationsForKind(genericDiagramKind(plan)))
	}
	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	return view
}

func genericDiagramKind(plan *AnswerSurfacePlan) DiagramKind {
	if plan == nil {
		return DiagramNone
	}
	if plan.CompiledDiagramKind != DiagramNone {
		return plan.CompiledDiagramKind
	}
	if plan.Diagram != nil && len(plan.Diagram.PreferredKinds) > 0 {
		return plan.Diagram.PreferredKinds[0]
	}
	return DiagramNone
}
