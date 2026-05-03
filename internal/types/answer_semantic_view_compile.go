package types

import "github.com/hanchaoqun/codrax/internal/logging"

// emitSemanticViewTrace writes a [trace/sv] debug line summarising
// the compiled view so operators can observe which family resolved
// + how many block / diagram / uncertainty / richness obligations
// the compiler produced. Per the docs/migration plan B1-T5 task
// description: required_blocks_count / optional_blocks_count /
// has_diagram / uncertainty_rules_count / richness_candidates_count.
//
// The "source" parameter ("agent" / "bus") tells the operator which
// builder entry-point produced the view.
func emitSemanticViewTrace(source string, view *AnswerSemanticView, ir *AnalysisIR, plan *AnswerSurfacePlan) {
	if view == nil {
		return
	}
	hasDiagram := view.DiagramPlan != nil && view.DiagramPlan.Required
	planShape := AnswerShape("")
	if plan != nil {
		planShape = plan.RequiredShape
	}
	intent := Intent("")
	if ir != nil {
		intent = ir.RequestModel.Intent
	}
	logging.Debug("[trace/sv] source=%s family=%s intent=%s plan_shape=%s required_blocks=%d optional_blocks=%d has_diagram=%v uncertainty_rules=%d richness_candidates=%d facet_coverage_present=%v exact_resolution_present=%v summary_mode=%q",
		source,
		view.Family,
		intent,
		planShape,
		len(view.RequiredBlocks),
		len(view.OptionalBlocks),
		hasDiagram,
		len(view.UncertaintyRules),
		len(view.RichnessCandidates),
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
// B1 (this commit): the compiler is a SKELETON — it produces a
// non-nil view that mirrors AnswerSurfacePlan's existing fields
// without doing any family-aware rule expansion. This is intentional:
// B1 only proves the type wiring works end-to-end; B2 fills in the
// per-family BlockRequirement / DiagramFacetGraph / UncertaintyRule
// / RichnessCandidate tables.
//
// Returns nil only when ir is nil — every other input shape produces
// a (possibly minimal) view so downstream consumers can rely on
// "if view != nil { ... }" without further nil-checks.
func BuildAnswerSemanticView(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
	if ir == nil {
		return nil
	}
	view := &AnswerSemanticView{
		Family: ResolveQuestionFamily(ir.RequestModel),
	}
	if plan != nil {
		view.FacetCoverage = plan.FacetCoverage
		view.SummaryMode = plan.SummarySurfaceMode
	}
	if ir.AnswerContract.ExactResolution != nil {
		view.ExactResolution = ir.AnswerContract.ExactResolution
	}
	// B1 skeleton: no RequiredBlocks / OptionalBlocks / DiagramPlan /
	// UncertaintyRules / RichnessCandidates yet. B2 fills them via
	// per-family compile_<family>.go files dispatched on view.Family.
	return view
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
