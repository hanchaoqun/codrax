package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimePeerErrorBoundedFactSet is the shared typed carrier for a finite
// per-occurrence runtime-log question. It deliberately reads neither the user
// request nor any model/final prose: the analyzer-owned runtime scope says the
// requested answer is a bounded fact set, while the validated LogBundle says
// the artifact contains multiple peer error occurrences.
func runtimePeerErrorBoundedFactSet(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil ||
		ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile == nil ||
		!ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.BoundedFactSet() {
		return false
	}
	bundle := ctx.LogTriage
	if bundle == nil && ctx.Mutable != nil {
		bundle = ctx.Mutable.LogTriage()
	}
	return bundle != nil && len(bundle.Errors) > 1
}

// renderExplorerPeerErrorFactScope keeps a finite peer-error investigation
// from widening its model-authored closure into a causal diagnosis. This is
// soft workflow guidance: it does not reject a tool call, inspect completion
// prose, or write any user-facing conclusion.
func renderExplorerPeerErrorFactScope(ctx *types.AgentContext) string {
	if !runtimePeerErrorBoundedFactSet(ctx) {
		return ""
	}
	return strings.Join([]string{
		"### Typed Peer-Error Fact Scope",
		"",
		"- runtime_question_scope=`bounded_fact_set`: complete the requested finite facts independently for each top-level error occurrence (for example its own message, first observed frame, and within-stack callers).",
		"- This scope carries no causal-attribution request. Do not widen `emit_investigation_complete.reason` into a cross-error root-cause, capture, propagation, caller/callee, or wrapper chain when `cross_error_relation=unproven`; preserve that relationship only as unproven if it must be mentioned.",
		"- Literal error-message wording remains an observation of that occurrence, not a relation edge to a peer occurrence.",
		"",
	}, "\n")
}
