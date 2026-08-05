package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocCallChainFinalEvidenceBoundary keeps the last prompt surface
// aligned with the typed call-chain contract. It is language-agnostic and
// prompt-only: names and request wording never become behavior authority.
func renderAnswerDocCallChainFinalEvidenceBoundary(view *types.AnswerSemanticView) string {
	if view == nil || view.Family != types.QFCallChain {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Final Call-Chain Evidence Boundary\n\n")
	b.WriteString("- You own the explanation. Preserve only directed hops carried by grounded caller-to-callee evidence. A call-site proves that edge, not the callee's body, side effect, storage medium, synchronization mode, or completion semantics.\n")
	b.WriteString("- Describe a terminal endpoint's internal behavior only from a separate grounded definition/mechanism row for that endpoint, and cite that implementation line when the behavior matters. Class names, method names, comments, layer labels, and the wording of the request do not mint implementation authority. If no terminal-body proof is available, say only that the chain reaches or invokes the endpoint.\n")
	b.WriteString("- Keep the model-authored summary useful and concise; this boundary supplies evidence caliber only and does not author a conclusion.\n\n")
	return b.String()
}

// renderAnswerDocTraceFinalDecisionBoundary replays the small set of typed
// authority limits that must govern the model's synthesis after every other
// dynamic section has been rendered. It deliberately does not copy candidate
// labels or choose a cause/recommendation. The deterministic system continues
// to own facts while the model owns the conclusion.
func renderAnswerDocTraceFinalDecisionBoundary(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	authority := answerDocRuntimeTraceGuidanceView(ctx)
	if !authority.RuntimeTrace {
		return ""
	}
	set := types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))
	var requestModel *types.RequestModel
	if ctx.AnalysisIR != nil {
		requestModel = &ctx.AnalysisIR.RequestModel
	}
	if len(set.Projections) == 0 || !types.RuntimeTraceReportMaterializationAllowed(requestModel, set) {
		return ""
	}

	hasActual, hasEliminable := traceDecisionAxesPresent(set)
	var b strings.Builder
	b.WriteString("## Final Trace Decision Boundary (Typed Facts; Model-Owned Conclusion)\n\n")
	b.WriteString("- You own the diagnosis, prioritization, optimization direction, and wording. The system supplies measurements and authority ceilings only; do not merely restate the projection rows.\n")
	if authority.CausalUnproven {
		b.WriteString("- causal_conclusion=`unproven`: the strongest supported synthesis is a bounded candidate or first validation direction, not a proven dropped-frame/frame-deadline cause.\n")
	}
	if authority.FrameEvidenceStatus != "" {
		fmt.Fprintf(&b, "- frame_evidence_status=`%s`: do not infer a stronger frame/deadline attribution.\n", authority.FrameEvidenceStatus)
	}
	switch {
	case hasActual && hasEliminable:
		b.WriteString("- available_axes=`actual_occupancy,existing_rule_eliminable`: compare both and explain their different decision use. Actual occupancy, existing-rule eliminable impact, and proven frame causality are distinct calibers; none substitutes for another. Their coexistence does not prove physical independence.\n")
	case hasActual:
		b.WriteString("- available_axes=`actual_occupancy`: identify the measured time concentration and a validation/optimization direction without inventing an existing-rule eliminable amount.\n")
	case hasEliminable:
		b.WriteString("- available_axes=`existing_rule_eliminable`: prioritize the typed repair candidates without inventing a separate actual-occupancy ranking.\n")
	default:
		b.WriteString("- available_axes=`none`: stay within the target-state, path, and evidence-boundary facts; do not invent a ranked cause.\n")
	}
	b.WriteString("- cross_row_addition=`not_authorized_without_exact_typed_relation`: a row-local state breakdown applies only to that row. Do not merge, decompose, compare as one subtotal, or add values from different rows/threads/fix directions unless one exact typed relation/fold carrier names those members and authorizes that operation.\n")
	b.WriteString("- Preserve directed wakeup/path semantics and typed holder/waiter or overlap relations exactly. Temporal order, adjacency, a candidate flag, or a kernel caller symbol alone does not prove synchronous blocking, lock ownership, post-wakeup preemption, or physical coupling.\n\n")
	return b.String()
}
