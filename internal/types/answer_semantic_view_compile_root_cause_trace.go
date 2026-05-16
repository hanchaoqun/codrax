package types

// compileRootCauseTrace builds the AnswerSemanticView for QFRootCauseTrace.
// Question shape: "why does X happen / panic / fail" — typically with
// an attached log or perf trace observing the failure event.
//
// Required blocks:
//
//	1× BlockSummary           — core conclusion (cause + observed site)
//	≥1× BlockOrderedList      — principal diagnostic sequence. For
//	                            current-code-backed failures this is
//	                            the cause chain; for external-only
//	                            artifacts this is the observed frames /
//	                            events the user asked about.
//	0..N× BlockDiagram        — sequence diagram for the cause chain
//	                            when DiagramHint resolves to one.
//	0..N× BlockCaveat         — drift / log-source / external-source
//	                            disclosures.
//
// Diagram plan: optional sequence diagram. Current-code-backed runs use
// FacetCurrentCodePath / FacetPrincipalPathEdge; external-only runtime
// artifacts use FacetObservedArtifactFact so observed frames are not
// forced through current-repo symbol grounding.
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
	if ir != nil && ir.AnswerContract.CurrentStatusDiagnostic != nil && ir.AnswerContract.CurrentStatusDiagnostic.Required {
		view.CurrentStatusDiagnostic = ir.AnswerContract.CurrentStatusDiagnostic
	}
	view.RequiredBlocks = []BlockRequirement{
		requireSummaryBlock(rootCauseTraceSummaryRationale(plan)),
		rootCauseTracePrincipalListRequirement(plan),
	}
	if view.CurrentStatusDiagnostic != nil && view.CurrentStatusDiagnostic.Required {
		view.RequiredBlocks = append(view.RequiredBlocks, BlockRequirement{
			Kind:     BlockDecision,
			MinCount: 1,
			MaxCount: 1,
			Required: true,
			FacetIDs: []string{
				string(FacetCurrentCodePath),
				string(FacetUncertaintyBoundary),
			},
			AcceptableClaimForms: []ClaimForm{
				ClaimDefinitionFact,
				ClaimCallEdge,
				ClaimGuardCondition,
				ClaimAbsenceFact,
			},
			Rationale: "Set current_status_verdict to still_present when current cited code still exposes the comparable risk, fixed when current cited code blocks it, or not_enough_evidence only when current evidence cannot decide. " +
				"Then explain the boundary in prose using current-code citations and the historical observation lane.",
			SurfaceRoleHint: SurfacePrincipal,
		})
	}
	view.OptionalBlocks = []BlockRequirement{
		optionalCaveatBlock(
			"When the observed event came from an attached log/perf trace, name which file:line "+
				"are runtime observations vs current source you read; this is the drift caveat.",
			string(FacetObservedArtifactFact),
			string(FacetUncertaintyBoundary),
		),
	}
	if diagramPreferredByEvidence(plan) || diagramRequiredByUserIntent(plan) {
		facetIDs := []string{string(FacetDiagramSpine), string(FacetCurrentCodePath)}
		if runtimeObservationOnly(plan) {
			facetIDs = []string{string(FacetDiagramSpine), string(FacetObservedArtifactFact)}
		}
		diagramBlock := optionalDiagramBlock(
			"When the user's requested diagnostic answer is easier to inspect visually and every node "+
				"can be copied from runtime frames or current citations, a small diagram may show the "+
				"observed order. Do not add a diagram just to satisfy a template preference.",
			facetIDs...,
		)
		if diagramRequiredByUserIntent(plan) {
			diagramBlock.Required = true
			diagramBlock.MinCount = 1
			diagramBlock.MaxCount = 1
			diagramBlock.Rationale = diagramRequirementRationale(plan, DiagramSequence,
				"A sequence diagram showing the grounded diagnostic sequence visually — actor-to-actor edges matching the ordered list. Use Mermaid sequenceDiagram form.")
			view.RequiredBlocks = append(view.RequiredBlocks, diagramBlock)
		} else {
			view.OptionalBlocks = append(view.OptionalBlocks, diagramBlock)
		}
	}
	diagramNodeFacets := []string{string(FacetCurrentCodePath)}
	diagramEdgeFacets := []string{string(FacetPrincipalPathEdge)}
	diagramRelations := append(defaultEdgeRelationsForPlan(plan, DiagramSequence),
		DiagramEdgeRelationContract{
			Kind: DiagramRelObserve, Min: 0, ClaimForm: ClaimExternalObservation,
		},
	)
	if runtimeObservationOnly(plan) {
		diagramNodeFacets = []string{string(FacetObservedArtifactFact)}
		diagramEdgeFacets = []string{string(FacetObservedArtifactFact)}
		diagramRelations = []DiagramEdgeRelationContract{
			{Kind: DiagramRelObserve, Min: 0, ClaimForm: ClaimExternalObservation},
		}
	}
	view.DiagramPlan = diagramPlanFor(plan, DiagramSequence,
		diagramNodeFacets,
		diagramEdgeFacets,
		diagramRelations,
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

func rootCauseTracePrincipalListRequirement(plan *AnswerSurfacePlan) BlockRequirement {
	if runtimeObservationOnly(plan) {
		return BlockRequirement{
			Kind:     BlockOrderedList,
			MinCount: 1,
			MaxCount: 0,
			Required: true,
			FacetIDs: []string{
				string(FacetObservedArtifactFact),
			},
			AcceptableClaimForms: []ClaimForm{
				ClaimExternalObservation,
			},
			Rationale: "List the principal runtime observations that answer the user's diagnostic question " +
				"(for example the failing frames, error events, or trace spans) in the user's requested order. " +
				"These items come from the attached artifact and should use citation_ref=-1 unless a current " +
				"repo citation literally proves the same fact. Do not replace them with helper functions or " +
				"resolver internals from the current repository.",
			SurfaceRoleHint: SurfacePrincipal,
		}
	}
	return BlockRequirement{
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
	}
}

func rootCauseTraceSummaryRationale(plan *AnswerSurfacePlan) string {
	if runtimeObservationOnly(plan) {
		return "Open with the attached runtime artifact's answer to the user's diagnostic question. " +
			"Keep artifact facts separate from current-repo facts: name observed frames / error messages " +
			"as observations, and state when the current checkout cannot corroborate them because the " +
			"artifact has no repo intersection."
	}
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
	if runtimeObservationOnly(plan) {
		return "Walk the observed artifact frames / events in the order that answers the user. " +
			"Use citation_ref=-1 for artifact-only facts and do not substitute current-repo helper code."
	}
	if plan != nil && plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause {
		return "Walk the grounded current-code path from the innermost cited frame outward. " +
			"Each list item is one current-code hop or mechanism anchor with file:line. " +
			"Do not convert a current guard / downstream dereference pair into a claimed runtime branch unless the observed artifact or a cited current line explicitly proves that path was taken. " +
			"When the closest grounded current fact is only an entry guard, keep it as a boundary note rather than promoting it into the principal crash step."
	}
	return "Walk the principal cause chain from the innermost failing frame outward. " +
		"Each list item is one hop with file:line; the order matches the call/causation sequence."
}
