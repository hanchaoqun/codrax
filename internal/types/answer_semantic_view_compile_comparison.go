package types

// compileComparison builds the AnswerSemanticView for QFComparison.
// Question shape: "compare A vs B", "X for A and Y for B",
// "differences between A and B" — answer is partitioned across the
// user-named buckets so each bucket gets its own principal section
// preserving the user's mental partition end-to-end.
//
// Default required blocks:
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
// Typed per-member-table shape:
//
// When RequestModel.Predicates.HasPerMemberTable is true, the analyzer has
// explicitly declared that every member's requested attributes must remain in
// a structured table. In that shape the required bucket sections are replaced
// by exactly one principal BlockTable. This keeps the closed typed declaration
// authoritative across family routing: QFComparison must not weaken the table
// to an optional presentation choice merely because the same request has two
// or more buckets.
//
// Default optional blocks:
//
//	0..1× BlockTable   — side-by-side rendering when the
//	                     comparison has multiple discrete axes.
//	0..1× BlockCaveat  — disclose any axis asymmetry between
//	                     buckets.
//
// Diagram is NOT required by the comparison family itself — comparisons are
// typically prose-led. A user-explicit DiagramContract is applied later by the
// family-independent presentation contract.
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
			},
			Rationale: "One section per user-named bucket; section Title is the verbatim " +
				"bucket label from the question. Body text answers that bucket on the user's " +
				"comparison axis; add citations or code-path details only when the axis itself " +
				"requires them, rather than forcing every comparison into current-code-path form. " +
				"When typed principal member rows belong to this bucket, carry them once in this " +
				"section's items[]; section items support label/text/cells and citation_ref, so do " +
				"not duplicate the same roster in a separate global list or table.",
			SurfaceRoleHint: SurfacePrincipal,
		},
	}
	perMemberTable := ir != nil && ir.RequestModel.Predicates.HasPerMemberTable
	if perMemberTable {
		view.RequiredBlocks = []BlockRequirement{
			requireSummaryBlock(
				"Open with what is being compared and along which axis(es). The summary MUST " +
					"name every bucket label verbatim from the question. Keep the comparison " +
					"axis explicit so the principal table below has a clear member × attribute contract."),
			{
				Kind:     BlockTable,
				MinCount: 1,
				MaxCount: 1,
				Required: true,
				FacetIDs: []string{
					string(FacetEnumerationItem),
					string(FacetBucketLabel),
					string(FacetComponentRelation),
				},
				Rationale: "The typed request requires one principal per-member table with no list or section escape. " +
					"Use one row per requested member and keep that member's bucket identity visible in the row. " +
					"Use columns for the requested comparison attributes or axes, and keep grounded member identity, " +
					"attributes, and citations together rather than duplicating the roster across bucket sections.",
				SurfaceRoleHint: SurfacePrincipal,
			},
		}
	}

	view.OptionalBlocks = []BlockRequirement{
		optionalCaveatBlock(
			"When the comparison axis is asymmetric (e.g. one bucket lacks a feature the "+
				"other has), disclose the asymmetry rather than synthesising a placeholder "+
				"value for the missing side.",
			string(FacetUncertaintyBoundary),
		),
	}
	if !perMemberTable {
		view.OptionalBlocks = append([]BlockRequirement{{
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
				"per bucket carries that bucket's value on each axis. Do not add this optional " +
				"table merely to repeat typed member rows already carried by the required bucket " +
				"sections; use it only when it adds a distinct cross-bucket axis grid.",
		}}, view.OptionalBlocks...)
	}

	// No family-native DiagramPlan — comparison is prose-led. A user-explicit
	// required diagram is applied by AnswerPresentationContract after family
	// compilation, keeping display preference separate from principal content.

	view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
	// v3 B2 — comparison answers benefit from component_relation
	// context when comparing structurally-related entities.
	markGlaringFacets(view, FacetComponentRelation)
	return view
}
