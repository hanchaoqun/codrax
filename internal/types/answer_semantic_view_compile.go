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
	logging.Debug("[trace/sv] source=%s family=%s intent=%s required_blocks=%d optional_blocks=%d has_diagram=%v uncertainty_rules=%d richness_candidates=%d required_candidate_roles=%d required_mechanism_anchors=%d error_granularity=%v facet_coverage_present=%v exact_resolution_present=%v summary_mode=%q",
		source,
		view.Family,
		intent,
		len(view.RequiredBlocks),
		len(view.OptionalBlocks),
		hasDiagram,
		len(view.UncertaintyRules),
		len(view.RichnessCandidates),
		len(view.RequiredCandidateRoles),
		len(view.RequiredMechanismAnchors),
		view.ErrorGranularityProfile != nil && view.ErrorGranularityProfile.Active(),
		view.FacetCoverage != nil,
		view.ExactResolution != nil,
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
	applyRequiredMechanismAnchors(view, ir)
	applyErrorGranularityProfile(view, ir)
	return view
}

func applyRequiredMechanismAnchors(view *AnswerSemanticView, ir *AnalysisIR) {
	if view == nil || ir == nil {
		return
	}
	view.RequiredMechanismAnchors = CompileRequiredMechanismAnchors(ir.RequestModel, ir.AnswerContract, view.Family)
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
	plan := BuildAnswerSurfacePlanForAgentContext(ac)
	view := BuildAnswerSemanticView(ac.AnalysisIR, plan)
	emitSemanticViewTrace("agent", view, ac.AnalysisIR, plan)
	return view
}

// BuildAnswerSemanticViewForBusContext compiles a view from the
// orchestrator's BusContext. Mirror of BuildAnswerSurfacePlanForBusContext.
// Reads bus.AnalysisIR + the surface plan derived from bus. Returns
// nil when AnalysisIR is missing.
func BuildAnswerSemanticViewForBusContext(bus *BusContext) *AnswerSemanticView {
	if bus == nil || bus.AnalysisIR == nil {
		return nil
	}
	plan := BuildAnswerSurfacePlanForBusContext(bus)
	view := BuildAnswerSemanticView(bus.AnalysisIR, plan)
	emitSemanticViewTrace("bus", view, bus.AnalysisIR, plan)
	return view
}
