package types

// compileComparison builds the AnswerSemanticView for QFComparison.
// Question shape: "compare A vs B", "X for A and Y for B",
// "differences between A and B" — answer is partitioned across the
// user-named buckets so each bucket gets its own principal section
// preserving the user's mental partition end-to-end.
//
// Required blocks:
//
//	1× BlockSummary  — opens with "what's being compared" + the
//	                   axis(es) of comparison; carries the
//	                   FacetBucketLabel facet so the renderer can
//	                   verify each bucket label appears verbatim
//	                   somewhere in the prose surface.
//	N× BlockSection  — exactly one per user-named bucket. Section
//	                   Title is the verbatim bucket Label;
//	                   MinCount=N=len(view.Buckets);
//	                   MaxCount=N (extra sections rejected so the
//	                   user's mental partition stays exact).
//
// Optional:
//
//	0..1× BlockTable   — side-by-side rendering when the
//	                     comparison has multiple discrete axes.
//	0..1× BlockCaveat  — disclose any axis asymmetry between
//	                     buckets.
//
// Diagram is NOT required — comparisons are typically prose-led.
//
// FacetBucketLabel is HARD per bucket per the family template
// (compile_facet_coverage's QFComparison branch); the renderer +
// G3 alignment gate independently verify each Label appears
// verbatim in summary OR a section heading.
func compileComparison(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	view := &AnswerSemanticView{
		Family: QFComparison,
	}
	if plan != nil {
		view.FacetCoverage = plan.FacetCoverage
		view.SummaryMode = plan.SummarySurfaceMode
	}
	if ir != nil && ir.AnswerContract.ExactResolution != nil {
		view.ExactResolution = ir.AnswerContract.ExactResolution
	}

	// Bucket count drives the per-section MinCount / MaxCount
	// pinning; falls back to 2 when buckets aren't yet resolved
	// (defensive — ResolveQuestionFamily's own gate requires
	// >=2 buckets to land in QFComparison, so this is theoretical).
	bucketCount := 2
	if ir != nil {
		if buckets := ir.RequestModel.QuestionStructure().Buckets; len(buckets) >= 2 {
			bucketCount = len(buckets)
		}
	}

	view.RequiredBlocks = []BlockRequirement{
		requireSummaryBlock(
			"Open with what is being compared and along which axis(es). The summary MUST " +
				"name every bucket label verbatim from the question. Keep the comparison " +
				"axis explicit (\"on metric M\", \"by component\", etc.) so each section " +
				"below has a known dimension to address."),
		{
			Kind:     BlockSection,
			MinCount: bucketCount,
			MaxCount: bucketCount,
			Required: true,
			FacetIDs: []string{
				string(FacetBucketLabel),
				string(FacetCurrentCodePath),
			},
			AcceptableClaimForms: []ClaimForm{
				ClaimDefinitionFact, ClaimCallEdge,
			},
			Rationale: "One section per user-named bucket; section Title is the verbatim " +
				"bucket label from the question. Body carries grounded code references " +
				"that establish each side's claim on the comparison axis.",
			SurfaceRoleHint: SurfacePrincipal,
		},
	}

	view.OptionalBlocks = []BlockRequirement{
		{
			Kind:     BlockTable,
			MinCount: 0,
			MaxCount: 1,
			Required: false,
			FacetIDs: []string{
				string(FacetBucketLabel),
				string(FacetComponentRelation),
			},
			Rationale: "When the comparison has multiple discrete axes, a side-by-side table " +
				"makes the axis × bucket grid scannable. Row 1 names the axes; one column " +
				"per bucket carries that bucket's value on each axis.",
			SurfaceRoleHint: SurfaceSupport,
		},
		optionalCaveatBlock(
			"When the comparison axis is asymmetric (e.g. one bucket lacks a feature the "+
				"other has), disclose the asymmetry rather than synthesising a placeholder "+
				"value for the missing side.",
			string(FacetUncertaintyBoundary),
		),
	}

	// No DiagramPlan — comparison is prose-led; LLM may still emit
	// a diagram via OptionalBlocks future-extension if it adds
	// signal, but the family does not require it.

	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	return view
}
