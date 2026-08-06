package types

import "github.com/hanchaoqun/codrax/internal/logging"

// emitSemanticViewTrace writes a [trace/sv] debug line summarising
// the compiled view so operators can observe which family resolved
// + how many block / diagram / uncertainty / richness obligations
// the compiler produced.
//
// The "source" parameter ("agent" / "bus") tells the operator which
// builder entry-point produced the view.
func emitSemanticViewTrace(source string, view *AnswerSemanticView, ir *AnalysisIR, _ *AnswerSurfacePlan) {
	if view == nil {
		return
	}
	hasDiagram := view.DiagramPlan != nil && view.DiagramPlan.Required
	intent := Intent("")
	if ir != nil {
		intent = ir.RequestModel.Intent
	}
	logging.Debug("[trace/sv] source=%s family=%s intent=%s required_blocks=%d optional_blocks=%d presentation_allowed_blocks=%d requested_dimensions=%d has_diagram=%v uncertainty_rules=%d richness_candidates=%d required_candidate_roles=%d required_mechanism_anchors=%d error_granularity=%v facet_coverage_present=%v exact_resolution_present=%v exact_resolution_answer_surface=%v summary_mode=%q",
		source,
		view.Family,
		intent,
		len(view.RequiredBlocks),
		len(view.OptionalBlocks),
		len(view.Presentation.AllowedBlocks),
		len(view.Presentation.RequestedDimensions),
		hasDiagram,
		len(view.UncertaintyRules),
		len(view.RichnessCandidates),
		len(view.RequiredCandidateRoles),
		len(view.RequiredMechanismAnchors),
		view.ErrorGranularityProfile != nil && view.ErrorGranularityProfile.Active(),
		view.FacetCoverage != nil,
		view.ExactResolution != nil,
		view.ExactResolution != nil && !view.SuppressExactResolutionAnswerSurface,
		view.SummaryMode,
	)
}

// BuildAnswerSemanticView is the entry point that compiles an
// AnswerSemanticView from the analyzer's output. The signature is
// agent-context-agnostic so both BuildAnswerSemanticViewForAgentContext
// and BuildAnswerSemanticViewForBusContext can share the same
// implementation by adapting their inputs.
//
// B2 (this commit): family-aware dispatch — ResolveQuestionFamily
// picks one of the 7 compile_<family>.go entry points which fills
// the RequiredBlocks / OptionalBlocks / DiagramPlan / UncertaintyRules
// / RichnessCandidates fields. Each compile_<family>.go is small
// (~80-120 LOC) and reads only typed signals (R3: no keyword tables).
//
// Returns nil only when ir is nil — every other input shape produces
// a (possibly minimal) view so downstream consumers can rely on
// "if view != nil { ... }" without further nil-checks.
//
// AllQuestionFamilies() in facet_plan.go enumerates the 7 values
// the dispatch must handle; the structural test
// TestAllQuestionFamiliesHaveCompiler enforces no family slips
// through without a compile entry-point.
func BuildAnswerSemanticView(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	if ir == nil {
		return nil
	}
	family := ResolveQuestionFamily(ir.RequestModel)
	var view *AnswerSemanticView
	switch family {
	case QFRootCauseTrace:
		view = compileRootCauseTrace(ir, plan)
	case QFConfigPrecedence:
		view = compileConfigPrecedence(ir, plan)
	case QFRoleLookup:
		view = compileRoleLookup(ir, plan)
	case QFCallChain:
		view = compileCallChain(ir, plan)
	case QFEnumeration:
		view = compileEnumeration(ir, plan)
	case QFArchitecture:
		view = compileArchitecture(ir, plan)
	case QFComparison:
		view = compileComparison(ir, plan)
	case QFGeneric:
		view = compileGeneric(ir, plan)
	default:
		// Unknown family: fall back to generic. This SHOULD never fire
		// because AllQuestionFamilies + the structural test cover the
		// 7-value enum, but keep the fallback so a future-added family
		// without a compile entry-point still produces a non-nil view.
		view = compileGeneric(ir, plan)
	}
	applyExactAbsenceSummaryLead(view, plan)
	applyRequestedCandidateRoles(view, ir)
	applyRequiredMechanismAnchors(view, ir, plan)
	applySurfacePlanDecisionLaneOverrides(view, plan)
	applyErrorGranularityProfile(view, ir)
	applyPresentationContract(view, ir, plan)
	return view
}

func applySurfacePlanDecisionLaneOverrides(view *AnswerSemanticView, plan *AnswerSurfacePlan) {
	if view == nil || plan == nil {
		return
	}
	if plan.RuntimeGroundingDisposition != nil &&
		plan.RuntimeGroundingDisposition.IsActive() &&
		!plan.CurrentSourceEvidenceOrigin {
		demoteFacetToOptional(view, FacetCurrentCodePath)
	}
	if !plan.CurrentStatusDiagnosticRequired {
		view.CurrentStatusDiagnostic = nil
		if len(view.RequiredBlocks) == 0 {
			return
		}
		out := view.RequiredBlocks[:0]
		for _, req := range view.RequiredBlocks {
			if blockRequirementIsCurrentStatusDecision(req) {
				continue
			}
			out = append(out, req)
		}
		view.RequiredBlocks = out
	}
}

func blockRequirementIsCurrentStatusDecision(req BlockRequirement) bool {
	return req.Kind == BlockDecision &&
		req.Required &&
		blockRequirementHasFacetID(req, string(FacetCurrentCodePath)) &&
		blockRequirementHasFacetID(req, string(FacetUncertaintyBoundary))
}

func blockRequirementHasFacetID(req BlockRequirement, facetID string) bool {
	if facetID == "" {
		return false
	}
	for _, got := range req.FacetIDs {
		if got == facetID {
			return true
		}
	}
	return false
}

func applyPresentationContract(view *AnswerSemanticView, ir *AnalysisIR, plan *AnswerSurfacePlan) {
	if view == nil {
		return
	}
	view.Presentation = CompileAnswerPresentationContract(ir, plan)
	if !view.Presentation.DiagramRequired {
		return
	}
	if view.DiagramPlan == nil {
		kind := view.Presentation.DiagramKind
		view.DiagramPlan = diagramPlanFor(plan, kind, nil, nil, DefaultEdgeRelationsForKind(kind))
	}
	if view.DiagramPlan == nil {
		view.DiagramPlan = &DiagramFacetGraph{
			Required:      true,
			Kind:          view.Presentation.DiagramKind,
			EdgeRelations: DefaultEdgeRelationsForKind(view.Presentation.DiagramKind),
		}
	} else {
		view.DiagramPlan.Required = true
		if view.DiagramPlan.Kind == DiagramNone {
			view.DiagramPlan.Kind = view.Presentation.DiagramKind
		}
	}
	ensureRequiredPresentationDiagramBlock(view, plan)
}

func ensureRequiredPresentationDiagramBlock(view *AnswerSemanticView, plan *AnswerSurfacePlan) {
	if view == nil {
		return
	}
	rationale := diagramRequirementRationale(
		plan,
		view.Presentation.DiagramKind,
		"The user explicitly requested a diagram. Preserve that visual output as a diagram block when the answer is grounded.",
	)
	req := BlockRequirement{
		Kind:      BlockDiagram,
		MinCount:  1,
		MaxCount:  1,
		Required:  true,
		Rationale: rationale,
	}
	for i := range view.RequiredBlocks {
		if view.RequiredBlocks[i].Kind != BlockDiagram {
			continue
		}
		view.RequiredBlocks[i].Required = true
		if view.RequiredBlocks[i].MinCount < 1 {
			view.RequiredBlocks[i].MinCount = 1
		}
		if view.RequiredBlocks[i].MaxCount == 0 || view.RequiredBlocks[i].MaxCount > 1 {
			view.RequiredBlocks[i].MaxCount = 1
		}
		if view.RequiredBlocks[i].Rationale == "" {
			view.RequiredBlocks[i].Rationale = rationale
		}
		view.OptionalBlocks = withoutOptionalBlockKind(view.OptionalBlocks, BlockDiagram)
		return
	}
	view.RequiredBlocks = append(view.RequiredBlocks, req)
	view.OptionalBlocks = withoutOptionalBlockKind(view.OptionalBlocks, BlockDiagram)
}

func withoutOptionalBlockKind(blocks []BlockRequirement, kind AnswerBlockKind) []BlockRequirement {
	if len(blocks) == 0 || kind == "" {
		return blocks
	}
	out := blocks[:0]
	for _, block := range blocks {
		if block.Kind == kind {
			continue
		}
		out = append(out, block)
	}
	return out
}

func applyRequiredMechanismAnchors(view *AnswerSemanticView, ir *AnalysisIR, plan *AnswerSurfacePlan) {
	if view == nil || ir == nil {
		return
	}
	var subRepoNames []string
	if plan != nil {
		subRepoNames = plan.SubRepoNames
	}
	view.RequiredMechanismAnchors = CompileRequiredMechanismAnchors(ir.RequestModel, ir.AnswerContract, view.Family, subRepoNames)
}

func applyRequestedCandidateRoles(view *AnswerSemanticView, ir *AnalysisIR) {
	if view == nil || ir == nil || ir.RequestModel.AnswerRoleProfile == nil || !ir.RequestModel.AnswerRoleProfile.Active() {
		return
	}
	roles := append([]AnswerCandidateRole(nil), view.RequiredCandidateRoles...)
	roles = append(roles, ir.RequestModel.AnswerRoleProfile.RequiredCandidateRoles...)
	seen := make(map[AnswerCandidateRole]struct{}, len(roles))
	view.RequiredCandidateRoles = view.RequiredCandidateRoles[:0]
	for _, role := range roles {
		if role == AnswerCandidateRoleUnknown {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		view.RequiredCandidateRoles = append(view.RequiredCandidateRoles, role)
	}
}

func applyErrorGranularityProfile(view *AnswerSemanticView, ir *AnalysisIR) {
	if view == nil || ir == nil || ir.RequestModel.ErrorGranularityProfile == nil ||
		!ir.RequestModel.ErrorGranularityProfile.Active() {
		return
	}
	if !ShouldCarryErrorGranularityHardContract(ir.RequestModel) {
		return
	}
	profile := *ir.RequestModel.ErrorGranularityProfile
	view.ErrorGranularityProfile = &profile
	ensureErrorGranularityDecisionBlock(view)
}

func ensureErrorGranularityDecisionBlock(view *AnswerSemanticView) {
	if view == nil {
		return
	}
	for i := range view.RequiredBlocks {
		if view.RequiredBlocks[i].Kind != BlockDecision {
			continue
		}
		view.RequiredBlocks[i].Required = true
		if view.RequiredBlocks[i].MinCount <= 0 {
			view.RequiredBlocks[i].MinCount = 1
		}
		if view.RequiredBlocks[i].MaxCount == 0 || view.RequiredBlocks[i].MaxCount > 1 {
			view.RequiredBlocks[i].MaxCount = 1
		}
		view.RequiredBlocks[i].SurfaceRoleHint = SurfacePrincipal
		if view.RequiredBlocks[i].Rationale == "" {
			view.RequiredBlocks[i].Rationale = "A canonical decision verdict is required for the failure-scope question."
		}
		return
	}
	view.RequiredBlocks = append(view.RequiredBlocks, BlockRequirement{
		Kind:            BlockDecision,
		MinCount:        1,
		MaxCount:        1,
		Required:        true,
		SurfaceRoleHint: SurfacePrincipal,
		Rationale:       "A canonical decision verdict is required for the failure-scope question.",
	})
}

// BuildAnswerSemanticViewForAgentContext compiles a view from the
// per-agent narrowed view. Reads ac.AnalysisIR + the surface plan
// derived from ac. Returns nil when AnalysisIR is missing.
func BuildAnswerSemanticViewForAgentContext(ac *AgentContext) *AnswerSemanticView {
	if ac == nil || ac.AnalysisIR == nil {
		return nil
	}
	if cached := ac.cachedAnswerSemanticView(); cached != nil {
		applyCallChainEndpointBoundary(cached, ac.AnalysisIR, ac.Mutable, ac.EvidenceItems)
		applyTraceCausalClaimContractForAgent(cached, ac)
		return cached
	}
	plan := BuildAnswerSurfacePlanForAgentContext(ac)
	view := BuildAnswerSemanticView(ac.AnalysisIR, plan)
	ac.storeAnswerSemanticView(view)
	applyCallChainEndpointBoundary(view, ac.AnalysisIR, ac.Mutable, ac.EvidenceItems)
	applyTraceCausalClaimContractForAgent(view, ac)
	emitSemanticViewTrace("agent", view, ac.AnalysisIR, plan)
	return cloneAnswerSemanticView(view)
}

// BuildAnswerSemanticViewForBusContext compiles a view from the
// orchestrator's BusContext. Mirror of BuildAnswerSurfacePlanForBusContext.
// Reads bus.AnalysisIR + the surface plan derived from bus. Returns
// nil when AnalysisIR is missing.
func BuildAnswerSemanticViewForBusContext(bus *BusContext) *AnswerSemanticView {
	if bus == nil || bus.AnalysisIR == nil {
		return nil
	}
	if cached := bus.cachedAnswerSemanticView(); cached != nil {
		applyCallChainEndpointBoundary(cached, bus.AnalysisIR, bus.Mutable, bus.EvidenceItems)
		applyTraceCausalClaimContractForBus(cached, bus)
		return cached
	}
	plan := BuildAnswerSurfacePlanForBusContext(bus)
	view := BuildAnswerSemanticView(bus.AnalysisIR, plan)
	bus.storeAnswerSemanticView(view)
	applyCallChainEndpointBoundary(view, bus.AnalysisIR, bus.Mutable, bus.EvidenceItems)
	applyTraceCausalClaimContractForBus(view, bus)
	emitSemanticViewTrace("bus", view, bus.AnalysisIR, plan)
	return cloneAnswerSemanticView(view)
}

func applyTraceCausalClaimContractForAgent(view *AnswerSemanticView, ctx *AgentContext) {
	if view == nil || ctx == nil || ctx.Mutable == nil {
		return
	}
	view.TraceCausalClaimContract = BuildTraceCausalClaimContract(
		ObservationLedgerInputFromAgentContext(ctx, ObservationPromptRecordLimit),
	)
}

func applyTraceCausalClaimContractForBus(view *AnswerSemanticView, ctx *BusContext) {
	if view == nil || ctx == nil || ctx.Mutable == nil {
		return
	}
	view.TraceCausalClaimContract = BuildTraceCausalClaimContract(
		ObservationLedgerInputFromBusContext(ctx, ObservationPromptRecordLimit),
	)
}

func principalSpanWaiverFromMutable(mutable *MutableState) *PrincipalSpanWaiver {
	if mutable == nil {
		return nil
	}
	return mutable.PrincipalSpanWaiver()
}

// CompileCallChainEndpointBoundary turns the model-declared typed waiver into
// a semantic endpoint disposition. Endpoint direction comes only from the
// analyzer's dedicated typed source/sink profile; the rationale remains an
// audit trail and cannot become presentation authority.
func CompileCallChainEndpointBoundary(rm RequestModel, waiver *PrincipalSpanWaiver) *CallChainEndpointBoundary {
	if ResolveQuestionFamily(rm) != QFCallChain || waiver == nil || !waiver.IsActive() ||
		waiver.Reason != PrincipalSpanWaiverNoDirectedPath {
		return nil
	}
	source, sink, ok := CallChainOrderedEndpointHints(rm)
	if !ok {
		return nil
	}
	boundary := &CallChainEndpointBoundary{
		Disposition:    CallChainEndpointNoDirectedPath,
		SourceEndpoint: source,
		RequestedSink:  sink,
	}
	if !boundary.Active() {
		return nil
	}
	return boundary
}

// CompileCallChainEndpointBoundaryWithEvidence enriches an accepted endpoint
// boundary with bounded grounded graph facts. This is a soft context carrier;
// the accepted waiver remains the only trigger and the model owns synthesis.
func CompileCallChainEndpointBoundaryWithEvidence(rm RequestModel, waiver *PrincipalSpanWaiver, evidence []EvidenceItem) *CallChainEndpointBoundary {
	boundary := CompileCallChainEndpointBoundary(rm, waiver)
	if boundary == nil {
		return nil
	}
	analysis := AnalyzeCallChainEvidenceGraph(evidence, boundary.SourceEndpoint, boundary.RequestedSink)
	existence := AnalyzeCallChainEndpointExistence(evidence, boundary.SourceEndpoint, boundary.RequestedSink)
	capsule := &CallChainEndpointEvidenceCapsule{
		EdgeCount: analysis.EdgeCount, SharedFrontier: analysis.SharedFrontier,
		SourceProof: existence.StartProof, RequestedSinkProof: existence.EndProof,
		SourcePath: analysis.SourcePath, SinkPath: analysis.SinkPath,
		SourceFrontier: analysis.SourceFrontier, RequestedBoundary: analysis.RequestedBoundary,
	}
	switch {
	case analysis.EdgeCount == 0:
		capsule.Status = CallChainEndpointEvidenceNoEdges
	case len(analysis.DirectedPath) > 0:
		capsule.Status = CallChainEndpointEvidenceDirectedPathPresent
		capsule.SourcePath = analysis.DirectedPath
	case len(analysis.ReversePath) > 0:
		capsule.Status = CallChainEndpointEvidenceReversePath
		capsule.SinkPath = analysis.ReversePath
	case analysis.StartAmbiguous || analysis.EndAmbiguous:
		capsule.Status = CallChainEndpointEvidenceEndpointAmbiguous
	case !analysis.StartResolved || !analysis.EndResolved:
		capsule.Status = CallChainEndpointEvidenceEndpointUnresolved
	case analysis.SharedFrontier != "":
		capsule.Status = CallChainEndpointEvidenceParallelConvergence
	default:
		capsule.Status = CallChainEndpointEvidenceDisjointFrontiers
	}
	capsule.SourcePath, capsule.SourcePathOmitted = boundCallChainEndpointEvidencePath(capsule.SourcePath, 8)
	capsule.SinkPath, capsule.SinkPathOmitted = boundCallChainEndpointEvidencePath(capsule.SinkPath, 8)
	boundary.EvidenceCapsule = capsule
	return boundary
}

func boundCallChainEndpointEvidencePath(in []CallChainEvidenceEdge, limit int) ([]CallChainEvidenceEdge, int) {
	if len(in) == 0 || limit <= 0 {
		return nil, len(in)
	}
	if len(in) <= limit {
		return append([]CallChainEvidenceEdge(nil), in...), 0
	}
	left := limit / 2
	right := limit - left
	out := make([]CallChainEvidenceEdge, 0, limit)
	out = append(out, in[:left]...)
	out = append(out, in[len(in)-right:]...)
	return out, len(in) - len(out)
}

func applyCallChainEndpointBoundary(view *AnswerSemanticView, ir *AnalysisIR, mutable *MutableState, handoffEvidence []EvidenceItem) {
	if view == nil {
		return
	}
	view.CallChainEndpointBoundary = nil
	if ir != nil {
		waiver := principalSpanWaiverFromMutable(mutable)
		// Finalization receives the explorer's accepted evidence through the
		// stage handoff on AgentContext/BusContext. Mutable.EmittedEvidence is
		// an exploration-time buffer and may already have been compacted or
		// reset by ParseOutput; TurnA is an earlier snapshot. Use all three
		// typed carriers so completion admission and finalizer context evaluate
		// the same evidence authority. Graph/existence analyzers deduplicate
		// identities and direction-preserving edges themselves.
		evidence := append([]EvidenceItem(nil), handoffEvidence...)
		if mutable != nil {
			if turnA := mutable.TurnAArtifacts(); turnA != nil {
				evidence = append(evidence, turnA.EvidenceItems...)
			}
			evidence = append(evidence, mutable.EmittedEvidence()...)
		}
		view.CallChainEndpointBoundary = CompileCallChainEndpointBoundaryWithEvidence(ir.RequestModel, waiver, evidence)
	}
}
